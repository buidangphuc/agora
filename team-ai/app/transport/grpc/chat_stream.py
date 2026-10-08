"""Chat streaming seam — the Chat/LLM decoupling for the gRPC ChatService.

team-ai embeds no LLM: token generation is delegated to the external LLM via the
existing router (``app.modules.ai.llm``), reached only through the ``ChatStreamer``
port. ``mock`` streams deterministic tokens offline; ``llm_router`` streams from
the router's fallback chain with per-request target selection, outcome feedback to
the router, a first-token timeout, a bounded number of pre-chunk attempts and the
caller's gRPC deadline as a hard cap. A model-server chat backend can be added as
another adapter without touching the servicer.

Fallback rule: a failure *before* the first text chunk moves on to the next target;
once a chunk has been emitted a failure ends the stream (``StreamInterrupted``) —
one reply never mixes output from two models.
"""

from __future__ import annotations

import asyncio
import time
import uuid
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import aclosing, suppress
from typing import TYPE_CHECKING, Any, Protocol, cast

from loguru import logger

from app.core.config import Settings
from app.core.errors import RateLimitError
from app.core.resilience import FailureKind, RetryPolicy, TimeoutPolicy
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.usage import ChatUsage, UsageAccumulator

if TYPE_CHECKING:
    from langchain_core.language_models.chat_models import BaseChatModel

    from app.modules.platform.quota.models import QuotaPolicy, QuotaReservation
    from app.modules.platform.quota.service import QuotaService

CHAT_QUOTA_RESOURCE = "chat.reply"


class ChatStreamError(Exception):
    """Base class for errors the chat streamer raises to the gRPC layer."""


class ChainExhausted(ChatStreamError):
    """Every target failed (or was skipped) before the first chunk was emitted."""

    def __init__(self, last_kind: FailureKind | None, attempts: int) -> None:
        super().__init__(
            f"llm chain exhausted after {attempts} attempt(s), "
            f"last_failure={last_kind.value if last_kind else 'none'}"
        )
        self.last_kind = last_kind
        self.attempts = attempts


class QuotaExhausted(ChatStreamError):
    """The principal's reply quota is used up; no model request was made."""

    def __init__(self, retry_after_seconds: int | None = None) -> None:
        super().__init__("chat quota exhausted")
        self.retry_after_seconds = retry_after_seconds


class StreamInterrupted(ChatStreamError):
    """The model failed after the first chunk; no fallback is attempted."""

    def __init__(self, kind: FailureKind) -> None:
        super().__init__(f"llm stream interrupted: {kind.value}")
        self.kind = kind


class ChatDeadlineExceeded(ChatStreamError):
    """The caller's gRPC deadline ran out; upstream work was cancelled."""


class ChatStreamer(Protocol):
    def astream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str = "",
        request_id: str = "",
        deadline_seconds: float | None = None,
    ) -> AsyncIterator[str]:
        """Yield token/text deltas for the reply. Implementations are async gens.

        May raise ``ChatStreamError`` subclasses; ``deadline_seconds`` (None = no
        deadline) is the caller's remaining time and is a hard cap. ``principal_id``
        is the forwarded caller ("" for anonymous): quota is keyed by it.
        """
        ...


class MockChatStreamer:
    """Offline, deterministic — echoes the prompt word by word."""

    async def astream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str = "",
        request_id: str = "",
        deadline_seconds: float | None = None,
    ) -> AsyncIterator[str]:
        for word in f"echo: {message}".split():
            yield word + " "


_EOS = object()  # the model ended without emitting any text


class LLMRouterChatStreamer:
    """Streams from the external LLM through the router's fallback chain."""

    def __init__(
        self,
        router: ModelRouter,
        *,
        first_token_timeout_seconds: float = 8.0,
        max_attempts: int = 3,
        quota_provider: Callable[[], QuotaService | None] | None = None,
        quota_policy: QuotaPolicy | None = None,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._router = router
        self._first_token_timeout = TimeoutPolicy(first_token_timeout_seconds)
        self._retry = RetryPolicy(max_attempts=max_attempts)
        self._quota_provider = quota_provider
        self._quota_policy = quota_policy
        self._sleep = sleep
        self._clock = clock
        self._models: dict[str, BaseChatModel] = {}

    async def astream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str = "",
        request_id: str = "",
        deadline_seconds: float | None = None,
    ) -> AsyncIterator[str]:
        deadline_at = (
            None if deadline_seconds is None else self._clock() + deadline_seconds
        )
        messages = await self._build_messages(message)

        # Quota is reserved before any LLM call; exhausted => no model request. The
        # reservation id is minted HERE, once per call: ``request_id`` is whatever
        # X-Request-Id the client sent, so using it would hand a repeated id the
        # earlier reservation back and never charge it again.
        reservation = await self._reserve_quota(principal_id, uuid.uuid4().hex)
        delivered = False
        try:
            async with aclosing(
                self._chain(
                    messages=messages,
                    request_id=request_id,
                    deadline_at=deadline_at,
                )
            ) as chain:
                async for text in chain:
                    delivered = True
                    yield text
        except BaseException:
            # Nothing delivered => refund; a partial reply counts as used.
            await self._settle_quota(reservation, refund=not delivered)
            raise
        await self._settle_quota(reservation, refund=not delivered)

    async def _chain(
        self,
        *,
        messages: list[Any],
        request_id: str,
        deadline_at: float | None,
    ) -> AsyncIterator[str]:
        router = self._router
        single_target = len(router.fallback_models()) <= 1
        last_kind: FailureKind | None = None
        attempts = 0

        for target in self._candidate_targets():
            if attempts >= self._retry.max_attempts:
                break
            if not router.try_acquire(target):
                continue  # another request holds this target's half-open probe
            model = self._model_for(target)
            while True:
                attempts += 1
                remaining = self._remaining(deadline_at)
                if remaining is not None and remaining <= 0:
                    router.record_error(target, kind=FailureKind.CANCELLED)
                    raise ChatDeadlineExceeded("deadline exceeded before LLM call")
                timeout = self._first_token_timeout.timeout_seconds
                if remaining is not None:
                    timeout = min(timeout, remaining)
                stream = model.astream(messages).__aiter__()  # type: ignore[arg-type]
                usage = UsageAccumulator()
                try:
                    first = await asyncio.wait_for(_next_text(stream, usage), timeout)
                except asyncio.CancelledError:
                    router.record_error(target, kind=FailureKind.CANCELLED)
                    await _aclose(stream)
                    raise
                except Exception as exc:
                    await _aclose(stream)
                    if self._deadline_passed(deadline_at) and isinstance(
                        exc, TimeoutError
                    ):
                        router.record_error(target, kind=FailureKind.CANCELLED)
                        raise ChatDeadlineExceeded("deadline exceeded") from exc
                    last_kind = router.record_error(target, error=exc)
                    logger.warning(
                        "chat.llm.attempt_failed target={} attempt={} kind={} error={}",
                        target,
                        attempts,
                        last_kind.value,
                        type(exc).__name__,
                    )
                    if single_target and await self._should_retry(
                        target, attempts, last_kind, exc
                    ):
                        continue
                    break  # next target

                if first is _EOS:
                    router.record_success(target)
                    self._report_usage(request_id, target, usage, messages, "")
                    return
                reply: list[str] = []
                async with aclosing(
                    self._relay(target, stream, cast(str, first), deadline_at, usage)
                ) as relay:
                    async for text in relay:
                        reply.append(text)
                        yield text
                self._report_usage(request_id, target, usage, messages, "".join(reply))
                return

        raise ChainExhausted(last_kind, attempts)

    async def _relay(
        self,
        target: str,
        stream: AsyncIterator[Any],
        first: str,
        deadline_at: float | None,
        usage: UsageAccumulator,
    ) -> AsyncIterator[str]:
        """Emit ``first`` then the rest; no fallback is possible from here on."""
        router = self._router
        try:
            yield first
            while True:
                remaining = self._remaining(deadline_at)
                if remaining is not None and remaining <= 0:
                    raise TimeoutError
                if remaining is None:
                    text = await _next_text(stream, usage)
                else:
                    text = await asyncio.wait_for(_next_text(stream, usage), remaining)
                if text is _EOS:
                    break
                yield cast(str, text)
        except (asyncio.CancelledError, GeneratorExit):
            router.record_error(target, kind=FailureKind.CANCELLED)
            await _aclose(stream)
            raise
        except Exception as exc:
            await _aclose(stream)
            if isinstance(exc, TimeoutError) and self._deadline_passed(deadline_at):
                router.record_error(target, kind=FailureKind.CANCELLED)
                raise ChatDeadlineExceeded("deadline exceeded mid-stream") from exc
            kind = router.record_error(target, error=exc)
            logger.warning(
                "chat.llm.stream_interrupted target={} kind={} error={}",
                target,
                kind.value,
                type(exc).__name__,
            )
            raise StreamInterrupted(kind) from exc
        router.record_success(target)

    async def _reserve_quota(
        self, principal_id: str, reservation_id: str
    ) -> QuotaReservation | None:
        """Reserve one reply for the principal, if quota is on.

        Returns None when quota is off, unwired or the caller is anonymous. A quota
        *backend* failure fails open (logged): metering must not take chat down.
        """
        quota = self._quota_provider() if self._quota_provider else None
        if quota is None or self._quota_policy is None or not principal_id:
            return None
        try:
            return await quota.reserve(
                subject_id=principal_id,
                resource=self._quota_policy.resource,
                cost=1,
                policy=self._quota_policy,
                idempotency_key=reservation_id,
            )
        except RateLimitError as exc:
            retry_after = (exc.headers or {}).get("Retry-After")
            raise QuotaExhausted(
                int(retry_after) if retry_after and retry_after.isdigit() else None
            ) from exc
        except Exception as exc:
            logger.warning("chat.quota.reserve_failed error={}", type(exc).__name__)
            return None

    async def _settle_quota(
        self, reservation: QuotaReservation | None, *, refund: bool
    ) -> None:
        quota = self._quota_provider() if self._quota_provider else None
        if reservation is None or quota is None:
            return
        try:
            if refund:
                await quota.refund(reservation)
            else:
                await quota.finalize(reservation)
        except Exception as exc:
            logger.warning(
                "chat.quota.settle_failed refund={} error={}",
                refund,
                type(exc).__name__,
            )

    def _report_usage(
        self,
        request_id: str,
        target: str,
        usage: UsageAccumulator,
        messages: list[Any],
        reply: str,
    ) -> ChatUsage:
        """Structured usage line for a completed reply.

        ``request_id``, ``target``, ``input_tokens`` and ``output_tokens`` are bound
        as log fields (they show up as ``record.extra`` in JSON logs) as well as in
        the message text. ``estimated`` marks a character-based estimate used when
        the provider reported no usage.
        """
        input_text = "".join(_chunk_text(m) for m in messages)
        result = usage.result(target=target, input_text=input_text, output_text=reply)
        logger.bind(
            request_id=request_id,
            target=target,
            input_tokens=result.input_tokens,
            output_tokens=result.output_tokens,
            total_tokens=result.total_tokens,
            estimated=result.estimated,
        ).info(
            "chat.llm.usage request_id={} target={} input_tokens={} "
            "output_tokens={} estimated={}",
            request_id,
            target,
            result.input_tokens,
            result.output_tokens,
            result.estimated,
        )
        return result

    def _candidate_targets(self) -> list[str]:
        if not self._router.fallback_models():
            return [""]  # no CHAT_MODEL configured: router serves its fake model
        return self._router.eligible_targets()

    def _model_for(self, target: str) -> BaseChatModel:
        if target not in self._models:
            self._models[target] = self._router.build_model(target)
        return self._models[target]

    async def _should_retry(
        self, target: str, attempt: int, kind: FailureKind, exc: BaseException
    ) -> bool:
        """Retry the same target (single-target chains only) on transient errors.

        Request-caused errors and rate limits are not helped by hammering the same
        target. ``attempt`` is the running pre-chunk attempt count, capped by
        ``LLM_MAX_ATTEMPTS``.
        """
        if not kind.counts_as_failure or kind is FailureKind.RATE_LIMITED:
            return False
        decision = self._retry.decision(attempt=attempt, error=exc)
        if not decision.should_retry:
            return False
        # The failure may just have opened this target's breaker.
        if not self._router.try_acquire(target):
            return False
        if decision.next_delay_seconds:
            await self._sleep(decision.next_delay_seconds)
        return True

    def _remaining(self, deadline_at: float | None) -> float | None:
        return None if deadline_at is None else deadline_at - self._clock()

    def _deadline_passed(self, deadline_at: float | None) -> bool:
        remaining = self._remaining(deadline_at)
        return remaining is not None and remaining <= 0.001

    async def _build_messages(self, message: str) -> list[Any]:
        from langchain_core.messages import HumanMessage

        return [HumanMessage(content=message)]


async def _next_text(stream: AsyncIterator[Any], usage: UsageAccumulator) -> Any:
    """Next non-empty text delta, or ``_EOS``.

    Empty chunks (role/usage-only) are skipped, but their usage is accumulated.
    """
    async for chunk in stream:
        usage.add(getattr(chunk, "usage_metadata", None))
        text = _chunk_text(chunk)
        if text:
            return text
    return _EOS


async def _aclose(stream: AsyncIterator[Any]) -> None:
    aclose = getattr(stream, "aclose", None)
    if aclose is None:
        return
    with suppress(Exception, asyncio.CancelledError):  # best-effort upstream cancel
        await aclose()


def _chunk_text(chunk: Any) -> str:
    content = getattr(chunk, "content", "")
    if isinstance(content, str):
        return content
    # Some providers stream content as a list of parts.
    if isinstance(content, list):
        return "".join(part for part in content if isinstance(part, str))
    return ""


def build_chat_streamer(
    settings: Settings,
    *,
    quota_provider: Callable[[], QuotaService | None] | None = None,
) -> ChatStreamer:
    if settings.CHAT_BACKEND == "mock":
        return MockChatStreamer()

    if settings.CHAT_BACKEND == "llm_router":
        return LLMRouterChatStreamer(
            ModelRouter(settings),
            first_token_timeout_seconds=settings.LLM_FIRST_TOKEN_TIMEOUT_SECONDS,
            max_attempts=settings.LLM_MAX_ATTEMPTS,
            quota_provider=quota_provider if settings.QUOTA_ENABLED else None,
            quota_policy=_chat_quota_policy(settings),
        )

    raise RuntimeError(
        f"CHAT_BACKEND={settings.CHAT_BACKEND!r} not supported "
        "(use 'mock' or 'llm_router')."
    )


def _chat_quota_policy(settings: Settings) -> QuotaPolicy | None:
    if not settings.QUOTA_ENABLED:
        return None
    from app.modules.platform.quota.models import QuotaPolicy

    return QuotaPolicy(
        resource=CHAT_QUOTA_RESOURCE,
        limit=settings.QUOTA_CHAT_REPLIES_PER_WINDOW,
        window_seconds=settings.QUOTA_CHAT_WINDOW_SECONDS,
    )
