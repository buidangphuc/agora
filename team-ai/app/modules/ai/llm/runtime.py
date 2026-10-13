from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import aclosing, suppress
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, cast

from app.core.config import Settings
from app.core.errors import AppError, ServiceUnavailableError
from app.core.resilience import is_transient_error
from app.modules.ai.llm.langfuse import (
    LangfuseLLMTracker,
    LLMTraceContext,
    build_langfuse_tracker,
)
from app.modules.ai.llm.router import ModelBuilder, ModelRole, ModelRouter

if TYPE_CHECKING:
    from langchain_core.language_models.chat_models import BaseChatModel
    from langchain_core.messages import BaseMessage, BaseMessageChunk


class LLMDeadlineExceeded(AppError):
    """The fallback chain used up ``LLM_CHAIN_DEADLINE_SECONDS``.

    A 504 carries ``status_code`` >= 500, so ``is_transient_error`` classifies
    it as transient: callers may retry the whole request later.
    """

    def __init__(self, deadline_seconds: float) -> None:
        super().__init__(
            code="llm_deadline_exceeded",
            message=f"LLM call exceeded its {deadline_seconds:g}s overall deadline",
            status_code=504,
        )


class LLMStreamInterrupted(AppError):
    """A target failed after it had already streamed text.

    The reply cannot move to another model without mixing two voices in one
    answer, so the stream ends here. ``__cause__`` is the provider error.
    """

    def __init__(self) -> None:
        super().__init__(
            code="llm_stream_interrupted",
            message="LLM stream was interrupted after it started",
            status_code=503,
        )


class _AttemptExpired(Exception):
    """The budget given to one wait ran out (not a provider error)."""


@dataclass(frozen=True)
class LLMInstance:
    """The model dependency handlers receive: router + tracker.

    The model is resolved per call, because which target is healthy changes
    at runtime. ``ainvoke`` is the supported call path: it walks the role's
    chain in order, skipping targets whose breaker is open, attaches the trace
    config, reports each outcome to that target's breaker and moves on to the
    next target only after a transient failure. A deterministic error
    propagates at once; when every target fails the last error is raised, and
    when every breaker is open ``ServiceUnavailableError``.

    The whole walk shares one ``LLM_CHAIN_DEADLINE_SECONDS`` budget: each
    attempt runs under what is left of it, and when it runs out the walk stops
    with ``LLMDeadlineExceeded``. An attempt cut short by the deadline is our
    budget, not the provider's fault, so it is not reported to the breaker.
    The per-request ``LLM_TIMEOUT_SECONDS`` stays the SDK's job.
    """

    router: ModelRouter
    tracker: LangfuseLLMTracker

    def trace_config(self, context: LLMTraceContext | None = None) -> dict[str, object]:
        return self.tracker.trace_config(context)

    async def ainvoke(
        self,
        input: Any,
        *,
        role: ModelRole = "default",
        context: LLMTraceContext | None = None,
    ) -> BaseMessage:
        deadline_seconds = self.router.settings.LLM_CHAIN_DEADLINE_SECONDS
        loop = asyncio.get_running_loop()
        deadline = loop.time() + deadline_seconds
        last_error: Exception | None = None
        for target in self.router.chain(role):
            remaining = deadline - loop.time()
            if remaining <= 0:
                raise LLMDeadlineExceeded(deadline_seconds) from last_error
            if not self.router.try_acquire(target, role):
                continue
            try:
                async with asyncio.timeout(remaining) as budget:
                    message = await self._call(target, input, context)
            except Exception as exc:
                if budget.expired():
                    self.router.release(target, role=role)
                    raise LLMDeadlineExceeded(deadline_seconds) from exc
                self.router.record_error(target, error=exc, role=role)
                if not is_transient_error(exc):
                    raise
                last_error = exc
                continue
            self.router.record_success(target, role=role)
            return message
        if last_error is not None:
            raise last_error
        raise ServiceUnavailableError(
            message="Every configured LLM target is unavailable",
            code="llm_unavailable",
        )

    async def astream(
        self,
        input: Any,
        *,
        role: ModelRole = "default",
        context: LLMTraceContext | None = None,
        first_token_timeout: float,
        deadline_seconds: float | None = None,
    ) -> AsyncIterator[BaseMessageChunk]:
        """Stream one reply, failing over only before the first text chunk.

        Walks the role's chain like ``ainvoke``. A target is abandoned for the
        next one only while it has produced no text: after a transient error,
        or when no text arrives within ``first_token_timeout``. Chunks without
        text are held back until text arrives, so a failed target leaks none.
        Once a target has yielded text, its failure raises
        ``LLMStreamInterrupted`` and no other target is called.

        The pre-first-chunk walk shares ``LLM_CHAIN_DEADLINE_SECONDS``;
        ``deadline_seconds`` (the caller's own deadline) caps the whole stream.
        Either running out raises ``LLMDeadlineExceeded``, closes the upstream
        stream and releases the attempt without blaming the provider.
        """
        chain_seconds = self.router.settings.LLM_CHAIN_DEADLINE_SECONDS
        loop = asyncio.get_running_loop()
        chain_deadline = loop.time() + chain_seconds
        caller_deadline = (
            None if deadline_seconds is None else loop.time() + deadline_seconds
        )
        last_error: Exception | None = None
        for target in self.router.chain(role):
            now = loop.time()
            if now >= chain_deadline:
                raise LLMDeadlineExceeded(chain_seconds) from last_error
            if caller_deadline is not None and now >= caller_deadline:
                raise LLMDeadlineExceeded(cast(float, deadline_seconds))
            if not self.router.try_acquire(target, role):
                continue
            settled = False
            try:
                try:
                    upstream, buffered, ended = await self._open_stream(
                        target,
                        input,
                        context,
                        first_token_timeout=first_token_timeout,
                        chain_deadline=chain_deadline,
                        caller_deadline=caller_deadline,
                    )
                except _AttemptExpired as exc:
                    if chain_deadline <= (caller_deadline or chain_deadline):
                        raise LLMDeadlineExceeded(chain_seconds) from exc
                    raise LLMDeadlineExceeded(cast(float, deadline_seconds)) from exc
                except Exception as exc:
                    self.router.record_error(target, error=exc, role=role)
                    settled = True
                    if not is_transient_error(exc):
                        raise
                    last_error = exc
                    continue
                async with aclosing(upstream):
                    for chunk in buffered:
                        yield chunk
                    if not ended:
                        try:
                            while True:
                                chunk = await _next_chunk(
                                    upstream, _remaining(loop, caller_deadline)
                                )
                                if chunk is None:
                                    break
                                yield chunk
                        except _AttemptExpired as exc:
                            raise LLMDeadlineExceeded(
                                cast(float, deadline_seconds)
                            ) from exc
                        except Exception as exc:
                            self.router.record_error(target, error=exc, role=role)
                            settled = True
                            raise LLMStreamInterrupted() from exc
                self.router.record_success(target, role=role)
                settled = True
                return
            finally:
                if not settled:
                    self.router.release(target, role=role)
        if last_error is not None:
            raise last_error
        raise ServiceUnavailableError(
            message="Every configured LLM target is unavailable",
            code="llm_unavailable",
        )

    async def _open_stream(
        self,
        target: str,
        input: Any,
        context: LLMTraceContext | None,
        *,
        first_token_timeout: float,
        chain_deadline: float,
        caller_deadline: float | None,
    ) -> tuple[AsyncGenerator[BaseMessageChunk, None], list[BaseMessageChunk], bool]:
        """Start ``target``'s stream and read up to its first text chunk.

        Returns the stream, the chunks read so far (the last one carries the
        first text) and whether the stream already ended. On any failure the
        upstream is closed before the error leaves.
        """
        loop = asyncio.get_running_loop()
        upstream = cast(
            AsyncGenerator[Any, None],
            self.router.model_for(target).astream(
                input,
                config=self.tracker.trace_config(context),  # type: ignore[arg-type]
            ),
        )
        buffered: list[BaseMessageChunk] = []
        try:
            while True:
                deadline = min(chain_deadline, caller_deadline or chain_deadline)
                budget = deadline - loop.time()
                if budget <= first_token_timeout:
                    chunk = await _next_chunk(upstream, budget)
                else:
                    try:
                        chunk = await _next_chunk(upstream, first_token_timeout)
                    except _AttemptExpired as exc:
                        raise TimeoutError(
                            "no text within first_token_timeout"
                        ) from exc
                if chunk is None:
                    return upstream, buffered, True
                buffered.append(chunk)
                if chunk_text(chunk):
                    return upstream, buffered, False
        except BaseException:
            with suppress(Exception):
                await upstream.aclose()
            raise

    def chat_model(self, role: ModelRole = "default") -> BaseChatModel:
        """Raw model for the next call (streaming, tool binding). Bypasses
        failover and outcome reporting; the caller owns both."""
        return self.router.chat_model(role)

    async def _call(
        self,
        target: str,
        input: Any,
        context: LLMTraceContext | None,
    ) -> BaseMessage:
        return await self.router.model_for(target).ainvoke(
            input,
            config=self.tracker.trace_config(context),  # type: ignore[arg-type]
        )


def chunk_text(chunk: BaseMessageChunk) -> str:
    """The text of a stream chunk; some providers stream a list of parts."""
    content = chunk.content
    if isinstance(content, str):
        return content
    return "".join(part for part in content if isinstance(part, str))


def _remaining(loop: asyncio.AbstractEventLoop, deadline: float | None) -> float | None:
    return None if deadline is None else deadline - loop.time()


async def _next_chunk(
    upstream: AsyncGenerator[Any, None], timeout: float | None
) -> Any:
    """Next chunk of ``upstream`` (``None`` at its end), within ``timeout``.

    Only our own timeout becomes ``_AttemptExpired``; a ``TimeoutError`` raised
    by the provider passes through as the provider error it is.
    """
    try:
        async with asyncio.timeout(timeout) as scope:
            return await anext(upstream, None)
    except TimeoutError as exc:
        if scope.expired():
            raise _AttemptExpired from exc
        raise


def build_llm_instance(
    settings: Settings,
    *,
    instance_id: str,
    service_name: str,
    tags: tuple[str, ...] = (),
    model_builder: ModelBuilder | None = None,
) -> LLMInstance:
    return LLMInstance(
        router=ModelRouter(settings, model_builder=model_builder),
        tracker=build_langfuse_tracker(
            settings,
            instance_id=instance_id,
            service_name=service_name,
            tags=tags,
        ),
    )
