"""LLMRouterChatStreamer: attempt loop, first-chunk fallback rule, timeouts."""

from __future__ import annotations

import asyncio

import pytest

from app.core.resilience import CircuitBreakerPolicy, FailureKind
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import (
    Script,
    ScriptedProvider,
    StatusError,
)
from app.transport.grpc.chat_stream import (
    ChainExhausted,
    ChatDeadlineExceeded,
    LLMRouterChatStreamer,
    StreamInterrupted,
)
from tests.factories import build_test_settings


class RecordingRouter(ModelRouter):
    """Router that logs every outcome reported to it."""

    def __init__(self, *args, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self.outcomes: list[tuple[str, str]] = []

    def record_success(self, target, *, role="default"):
        self.outcomes.append((target, "success"))
        super().record_success(target, role=role)

    def record_error(self, target, **kwargs):
        kind = super().record_error(target, **kwargs)
        self.outcomes.append((target, kind.value))
        return kind


async def _no_sleep(_: float) -> None:
    return None


def _setup(
    provider: ScriptedProvider,
    *,
    chain: tuple[str, str] = ("a", "b, c"),
    threshold: int = 3,
    attempts: int = 3,
    first_token_timeout: float = 5.0,
):
    router = RecordingRouter(
        build_test_settings(CHAT_MODEL=chain[0], CHAT_FALLBACK_MODELS=chain[1]),
        model_builder=provider.builder,
        breaker_policy=CircuitBreakerPolicy(
            failure_threshold=threshold, cooldown_seconds=30
        ),
    )
    streamer = LLMRouterChatStreamer(
        router,
        first_token_timeout_seconds=first_token_timeout,
        max_attempts=attempts,
        sleep=_no_sleep,
    )
    return router, streamer


async def _collect(streamer, message="hi", **kwargs) -> list[str]:
    return [d async for d in streamer.astream(message, session_id="s", **kwargs)]


# --- 2.1 per-request routing + outcome feedback ------------------------------


async def test_healthy_primary_serves_and_records_one_success():
    provider = ScriptedProvider().queue("a", Script(chunks=("he", "llo")))
    router, streamer = _setup(provider)

    assert await _collect(streamer) == ["he", "llo"]

    assert router.outcomes == [("a", "success")]
    assert [c.target for c in provider.calls] == ["a"]


async def test_router_receives_one_outcome_per_attempt():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(503)))
    provider.queue("b", Script(fail_at=0, error=StatusError(500)))
    provider.queue("c", Script(chunks=("ok",)))
    router, streamer = _setup(provider)

    assert await _collect(streamer) == ["ok"]

    assert router.outcomes == [
        ("a", "server"),
        ("b", "server"),
        ("c", "success"),
    ]


async def test_models_are_built_once_per_target_and_reused_across_requests():
    provider = ScriptedProvider()
    _, streamer = _setup(provider)

    await _collect(streamer)
    await _collect(streamer)

    assert provider.built == ["a"]
    assert len(provider.calls) == 2


async def test_breaker_state_changes_next_request_target_without_restart():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(429)))
    provider.queue("b", Script(chunks=("from-b",)), Script(chunks=("from-b2",)))
    router, streamer = _setup(provider, threshold=1)

    assert await _collect(streamer) == ["from-b"]  # a fails, breaker opens
    assert router.breaker_state("a") == "open"

    assert await _collect(streamer) == ["from-b2"]
    assert [c.target for c in provider.calls] == ["a", "b", "b"]  # a skipped


# --- 2.2 fallback only before the first chunk --------------------------------


async def test_pre_chunk_failure_falls_back_and_caller_never_sees_it():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(429)))
    provider.queue("b", Script(chunks=("b1", "b2")))
    _, streamer = _setup(provider)

    assert await _collect(streamer) == ["b1", "b2"]


async def test_full_chain_is_tried_in_order():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(500)))
    provider.queue("b", Script(fail_at=0, error=TimeoutError()))
    provider.queue("c", Script(chunks=("c1",)))
    _, streamer = _setup(provider)

    assert await _collect(streamer) == ["c1"]
    assert [c.target for c in provider.calls] == ["a", "b", "c"]


async def test_failure_after_first_chunk_ends_stream_without_switching_models():
    provider = ScriptedProvider()
    provider.queue(
        "a",
        Script(chunks=("x", "y", "z"), fail_at=2, error=StatusError(503)),
    )
    provider.queue("b", Script(chunks=("NEVER",)))
    router, streamer = _setup(provider)
    received: list[str] = []

    with pytest.raises(StreamInterrupted) as info:
        async for delta in streamer.astream("hi", session_id="s"):
            received.append(delta)

    assert received == ["x", "y"]
    assert info.value.kind is FailureKind.SERVER
    assert [c.target for c in provider.calls] == ["a"]  # no retry, no fallback
    assert router.outcomes == [("a", "server")]


async def test_chain_exhausted_when_every_target_fails_before_first_chunk():
    provider = ScriptedProvider()
    for target in ("a", "b", "c"):
        provider.queue(target, Script(fail_at=0, error=StatusError(500)))
    router, streamer = _setup(provider)
    received: list[str] = []

    with pytest.raises(ChainExhausted) as info:
        async for delta in streamer.astream("hi", session_id="s"):
            received.append(delta)

    assert received == []
    assert info.value.attempts == 3
    assert info.value.last_kind is FailureKind.SERVER
    assert len(router.outcomes) == 3


async def test_chain_exhausted_reports_rate_limit_when_last_failure_was_429():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(500)))
    provider.queue("b", Script(fail_at=0, error=StatusError(429)))
    _, streamer = _setup(provider, chain=("a", "b"))

    with pytest.raises(ChainExhausted) as info:
        await _collect(streamer)

    assert info.value.last_kind is FailureKind.RATE_LIMITED


async def test_all_breakers_open_fails_fast_without_calling_the_provider():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(500)))
    router, streamer = _setup(provider, chain=("a", ""), threshold=1)
    with pytest.raises(ChainExhausted):
        await _collect(streamer)
    calls_before = len(provider.calls)

    with pytest.raises(ChainExhausted) as info:
        await _collect(streamer)

    assert len(provider.calls) == calls_before
    assert info.value.attempts == 0


async def test_empty_leading_chunks_do_not_count_as_first_chunk():
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("", "", "hi"), usage=None))
    _, streamer = _setup(provider)

    assert await _collect(streamer) == ["hi"]


async def test_client_error_is_not_retried_and_does_not_open_the_breaker():
    provider = ScriptedProvider()
    provider.queue("a", Script(fail_at=0, error=StatusError(400)))
    provider.queue("b", Script(chunks=("ok",)))
    router, streamer = _setup(provider, attempts=3, threshold=1)

    assert await _collect(streamer) == ["ok"]

    assert len(provider.calls_to("a")) == 1
    assert router.breaker_state("a") == "closed"


# --- 2.3 first-token timeout, bounded retry ----------------------------------


async def test_hung_model_times_out_is_recorded_as_timeout_and_falls_back():
    provider = ScriptedProvider()
    provider.queue("a", Script(hang=True))
    provider.queue("b", Script(chunks=("ok",)))
    router, streamer = _setup(provider, first_token_timeout=0.05)

    assert await _collect(streamer) == ["ok"]

    assert router.outcomes == [("a", "timeout"), ("b", "success")]
    assert provider.calls_to("a")[0].cancelled is True


async def test_attempts_across_the_chain_stop_at_the_configured_maximum():
    provider = ScriptedProvider()
    for target in ("a", "b", "c"):
        provider.queue(target, *[Script(fail_at=0, error=StatusError(502))] * 5)
    _, streamer = _setup(provider, attempts=2, threshold=10)

    with pytest.raises(ChainExhausted) as info:
        await _collect(streamer)

    assert info.value.attempts == 2
    assert len(provider.calls) == 2  # c is never reached
    assert len(provider.calls_to("a")) == 1  # chain targets are not retried


async def test_single_target_chain_retries_up_to_the_maximum():
    provider = ScriptedProvider()
    provider.queue("a", *[Script(fail_at=0, error=StatusError(502))] * 2)
    provider.queue("a", Script(chunks=("ok",)))
    _, streamer = _setup(provider, chain=("a", ""), attempts=3, threshold=10)

    assert await _collect(streamer) == ["ok"]

    assert len(provider.calls_to("a")) == 3


async def test_single_target_chain_is_bounded_by_the_maximum():
    provider = ScriptedProvider()
    provider.queue("a", *[Script(fail_at=0, error=StatusError(502))] * 9)
    _, streamer = _setup(provider, chain=("a", ""), attempts=3, threshold=10)

    with pytest.raises(ChainExhausted) as info:
        await _collect(streamer)

    assert info.value.attempts == 3
    assert len(provider.calls_to("a")) == 3


async def test_rate_limited_target_is_not_retried_but_skipped():
    provider = ScriptedProvider()
    provider.queue("a", *[Script(fail_at=0, error=StatusError(429))] * 3)
    provider.queue("b", Script(chunks=("ok",)))
    _, streamer = _setup(provider, attempts=3, threshold=10)

    assert await _collect(streamer) == ["ok"]

    assert len(provider.calls_to("a")) == 1


async def test_retry_backoff_uses_the_retry_policy_delays():
    delays: list[float] = []

    async def record_sleep(delay: float) -> None:
        delays.append(delay)

    provider = ScriptedProvider()
    provider.queue("a", *[Script(fail_at=0, error=StatusError(500))] * 3)
    router = ModelRouter(
        build_test_settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS=""),
        model_builder=provider.builder,
        breaker_policy=CircuitBreakerPolicy(failure_threshold=10),
    )
    streamer = LLMRouterChatStreamer(router, max_attempts=3, sleep=record_sleep)

    with pytest.raises(ChainExhausted):
        await _collect(streamer)

    assert delays == [
        0.25
    ]  # default backoff (0.0, 0.25, ...); a zero delay is not slept


async def test_no_retry_after_first_chunk_even_with_attempts_left():
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("x",), fail_at=1, error=StatusError(500)))
    provider.queue("a", Script(chunks=("again",)))
    _, streamer = _setup(provider, chain=("a", ""), attempts=5)

    with pytest.raises(StreamInterrupted):
        await _collect(streamer)

    assert len(provider.calls_to("a")) == 1


# --- deadline (streamer level; servicer mapping tested separately) -----------


async def test_deadline_shorter_than_first_token_timeout_cancels_upstream():
    provider = ScriptedProvider()
    provider.queue("a", Script(hang=True))
    provider.queue("b", Script(chunks=("NEVER",)))
    router, streamer = _setup(provider, first_token_timeout=30)

    started = asyncio.get_running_loop().time()
    with pytest.raises(ChatDeadlineExceeded):
        await _collect(streamer, deadline_seconds=0.1)
    elapsed = asyncio.get_running_loop().time() - started

    assert elapsed < 1.0
    assert provider.calls_to("a")[0].cancelled is True
    assert provider.calls_to("b") == []  # deadline ends the call, no fallback
    # The caller's budget is not the provider's fault: no failure recorded.
    assert router.breaker_state("a") == "closed"


async def test_deadline_mid_stream_raises_deadline_exceeded():
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("x", "y"), delay=0.2))
    _, streamer = _setup(provider)
    received: list[str] = []

    with pytest.raises(ChatDeadlineExceeded):
        async for delta in streamer.astream("hi", session_id="s", deadline_seconds=0.3):
            received.append(delta)

    assert received == ["x"]
    assert provider.calls_to("a")[0].cancelled is True


async def test_client_disconnect_frees_the_half_open_probe_without_judging():
    provider = ScriptedProvider()
    provider.queue("a", Script(chunks=("x", "y", "z"), delay=0.01))
    router, streamer = _setup(provider)

    gen = streamer.astream("hi", session_id="s")
    assert await gen.__anext__() == "x"
    await gen.aclose()

    assert router.breaker_state("a") == "closed"
    assert ("a", "cancelled") in router.outcomes
