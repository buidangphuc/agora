"""Quota reserve -> finalize/refund around the chat LLM call (resource chat.reply)."""

from __future__ import annotations

import grpc
import pytest

from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider, StatusError
from app.modules.platform.identity.schemas import Principal
from app.modules.platform.quota.adapters.memory import MemoryQuotaStore
from app.modules.platform.quota.models import QuotaPolicy
from app.modules.platform.quota.service import QuotaService
from app.modules.platform.quota.store import StaticQuotaPolicyStore
from app.transport.grpc._pb.platform.chat.v1 import chat_pb2
from app.transport.grpc.chat_stream import (
    CHAT_QUOTA_RESOURCE,
    ChainExhausted,
    LLMRouterChatStreamer,
    QuotaExhausted,
    StreamInterrupted,
    build_chat_streamer,
)
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.servicers.chat import ChatServicer
from tests.factories import build_test_settings


class SpyQuota(QuotaService):
    def __init__(self) -> None:
        super().__init__(
            store=MemoryQuotaStore(), policy_store=StaticQuotaPolicyStore()
        )
        self.calls: list[tuple[str, dict]] = []

    async def reserve(self, **kwargs):
        self.calls.append(("reserve", kwargs))
        return await super().reserve(**kwargs)

    async def finalize(self, reservation, **kwargs):
        self.calls.append(("finalize", kwargs))
        return await super().finalize(reservation, **kwargs)

    async def refund(self, reservation):
        self.calls.append(("refund", {}))
        return await super().refund(reservation)


def _policy(limit: int = 5) -> QuotaPolicy:
    return QuotaPolicy(resource=CHAT_QUOTA_RESOURCE, limit=limit, window_seconds=3600)


def _streamer(
    provider: ScriptedProvider,
    quota: QuotaService | None,
    *,
    limit: int = 5,
    fallbacks: str = "",
) -> LLMRouterChatStreamer:
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=fallbacks),
        model_builder=provider.builder,
    )
    return LLMRouterChatStreamer(
        router,
        quota_provider=(lambda: quota) if quota is not None else None,
        quota_policy=_policy(limit),
        max_attempts=1,
    )


async def _chat(streamer, *, principal="alice", request_id="req-1") -> list[str]:
    return [
        d
        async for d in streamer.astream(
            "hello there", session_id="", principal_id=principal, request_id=request_id
        )
    ]


async def _used(quota: QuotaService, principal="alice", limit: int = 5) -> int:
    usage = await quota.get_usage(
        subject_id=principal, resource=CHAT_QUOTA_RESOURCE, policy=_policy(limit)
    )
    return usage.used


async def test_success_reserves_one_reply_then_finalizes_with_a_server_minted_key():
    quota = SpyQuota()
    provider = ScriptedProvider().queue("a", Script(chunks=("ok",)))

    assert await _chat(_streamer(provider, quota)) == ["ok"]

    assert [name for name, _ in quota.calls] == ["reserve", "finalize"]
    reserve = quota.calls[0][1]
    assert reserve["subject_id"] == "alice"
    assert reserve["resource"] == CHAT_QUOTA_RESOURCE
    assert reserve["cost"] == 1
    # A server-minted per-call key, never the client-influenced request id.
    assert reserve["idempotency_key"] not in (None, "", "req-1")
    assert len(reserve["idempotency_key"]) == 32  # uuid4().hex
    assert await _used(quota) == 1


async def test_repeating_a_client_request_id_is_charged_every_time():
    quota = SpyQuota()
    provider = ScriptedProvider()
    streamer = _streamer(provider, quota)

    await _chat(streamer, request_id="same")
    await _chat(streamer, request_id="same")

    assert await _used(quota) == 2


async def test_exhausted_quota_refuses_without_any_model_call():
    quota = SpyQuota()
    provider = ScriptedProvider()
    streamer = _streamer(provider, quota, limit=2)
    await _chat(streamer)
    await _chat(streamer)
    calls_before = len(provider.calls)

    with pytest.raises(QuotaExhausted) as info:
        await _chat(streamer)

    assert len(provider.calls) == calls_before  # the third call never reached a model
    assert info.value.retry_after_seconds is not None


async def test_pre_chunk_failure_refunds_so_a_failed_call_costs_nothing():
    quota = SpyQuota()
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(500)))
    streamer = _streamer(provider, quota, limit=1)

    with pytest.raises(ChainExhausted):
        await _chat(streamer)
    assert await _used(quota, limit=1) == 0
    assert [name for name, _ in quota.calls] == ["reserve", "refund"]

    # The single reply of quota is still available.
    assert await _chat(streamer) == ["ok"]


async def test_reply_that_breaks_after_a_chunk_is_finalized_not_refunded():
    quota = SpyQuota()
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("x", "y"), fail_at=1, error=StatusError(503)))
    streamer = _streamer(provider, quota)
    received: list[str] = []

    with pytest.raises(StreamInterrupted):
        async for delta in streamer.astream(
            "hi", session_id="", principal_id="alice", request_id="r"
        ):
            received.append(delta)

    assert received == ["x"]
    assert [name for name, _ in quota.calls] == ["reserve", "finalize"]
    assert await _used(quota) == 1  # the user received output


async def test_client_disconnect_after_a_chunk_still_counts_the_reply():
    quota = SpyQuota()
    provider = ScriptedProvider().queue("a", Script(chunks=("x", "y", "z")))
    streamer = _streamer(provider, quota)

    gen = streamer.astream("hi", session_id="", principal_id="alice", request_id="r")
    assert await gen.__anext__() == "x"
    await gen.aclose()

    assert [name for name, _ in quota.calls] == ["reserve", "finalize"]


async def test_disconnect_before_any_chunk_refunds():
    quota = SpyQuota()
    provider = ScriptedProvider().queue("a", Script(hang=True))
    streamer = _streamer(provider, quota)
    streamer._first_token_timeout = type(streamer._first_token_timeout)(0.05)

    with pytest.raises(ChainExhausted):
        await _chat(streamer)

    assert [name for name, _ in quota.calls] == ["reserve", "refund"]


async def test_quota_is_keyed_by_principal_so_users_do_not_share_a_budget():
    quota = SpyQuota()
    streamer = _streamer(ScriptedProvider(), quota, limit=1)

    await _chat(streamer, principal="alice")
    assert await _chat(streamer, principal="bob") == ["ok"]

    with pytest.raises(QuotaExhausted):
        await _chat(streamer, principal="alice")


async def test_quota_disabled_makes_no_quota_calls():
    quota = SpyQuota()
    provider = ScriptedProvider()
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a"), model_builder=provider.builder
    )
    # No provider/policy wired (QUOTA_ENABLED=false): nothing is reserved.
    streamer = LLMRouterChatStreamer(router)

    assert await _chat(streamer) == ["ok"]
    assert quota.calls == []


async def test_anonymous_callers_are_not_metered_per_principal():
    quota = SpyQuota()
    streamer = _streamer(ScriptedProvider(), quota)

    assert await _chat(streamer, principal="") == ["ok"]
    assert quota.calls == []


async def test_quota_backend_failure_fails_open():
    class BrokenQuota(SpyQuota):
        async def reserve(self, **kwargs):
            raise RuntimeError("store down")

    streamer = _streamer(ScriptedProvider(), BrokenQuota())

    assert await _chat(streamer) == ["ok"]


def test_build_chat_streamer_wires_the_chat_reply_policy_only_when_enabled():
    quota = SpyQuota()
    settings = build_test_settings(
        CHAT_BACKEND="llm_router",
        QUOTA_ENABLED=True,
        QUOTA_CHAT_REPLIES_PER_WINDOW=7,
        QUOTA_CHAT_WINDOW_SECONDS=120,
    )

    streamer = build_chat_streamer(settings, quota_provider=lambda: quota)

    assert isinstance(streamer, LLMRouterChatStreamer)
    policy = streamer._quota_policy
    assert policy is not None
    assert (policy.resource, policy.limit, policy.window_seconds) == (
        "chat.reply",
        7,
        120,
    )
    off = build_chat_streamer(
        build_test_settings(CHAT_BACKEND="llm_router"), quota_provider=lambda: quota
    )
    assert off._quota_policy is None  # type: ignore[attr-defined]


# --- servicer: RESOURCE_EXHAUSTED + the forwarded principal ------------------


class _Aborted(Exception):
    def __init__(self, code: grpc.StatusCode, details: str) -> None:
        super().__init__(details)
        self.code = code
        self.details = details


class _FakeContext:
    def __init__(self) -> None:
        self.trailing: tuple = ()

    def time_remaining(self) -> float | None:
        return None

    def set_trailing_metadata(self, metadata) -> None:
        self.trailing = tuple(metadata)

    async def abort(self, code: grpc.StatusCode, details: str = ""):
        raise _Aborted(code, details)


async def _stream(servicer: ChatServicer, principal: Principal, context: _FakeContext):
    token = bind_principal(principal)
    try:
        request = chat_pb2.StreamChatRequest(session_id="s", message="hello")
        return [r async for r in servicer.StreamChat(request, context)]  # type: ignore[arg-type]
    finally:
        reset_principal(token)


async def test_servicer_maps_exhausted_quota_and_meters_the_forwarded_principal():
    quota = SpyQuota()
    streamer = _streamer(ScriptedProvider(), quota, limit=1)
    servicer = ChatServicer(streamer)
    buyer = Principal(id="buyer-7", type="user", scopes=())

    await _stream(servicer, buyer, _FakeContext())
    context = _FakeContext()
    with pytest.raises(_Aborted) as info:
        await _stream(servicer, buyer, context)

    assert info.value.code is grpc.StatusCode.RESOURCE_EXHAUSTED
    assert info.value.details == "quota exceeded"
    assert dict(context.trailing).get("retry-after")
    assert quota.calls[0][1]["subject_id"] == "buyer-7"
