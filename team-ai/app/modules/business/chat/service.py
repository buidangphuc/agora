"""LLM-backed chat: redact, replay history, meter, stream, remember.

Failover between models lives in ``LLMInstance.astream``; this service decides
what a failure means for the caller, the quota and the history.
"""

import uuid
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import aclosing
from typing import Any, cast

from loguru import logger

from app.core.errors import RateLimitError
from app.core.redaction import RedactionPolicy
from app.core.resilience import error_status_code
from app.modules.ai.llm.langfuse import LLMTraceContext
from app.modules.ai.llm.prompt import PromptProvider
from app.modules.ai.llm.runtime import (
    LLMDeadlineExceeded,
    LLMInstance,
    LLMStreamInterrupted,
    chunk_text,
)
from app.modules.ai.llm.session import SessionStore, Turn
from app.modules.ai.llm.usage import UsageAccumulator
from app.modules.business.chat.errors import (
    ChatDeadlineExceeded,
    ChatError,
    ChatInterrupted,
    ChatModelRateLimited,
    ChatModelUnavailable,
    ChatQuotaExhausted,
)
from app.modules.platform.quota.models import QuotaPolicy, QuotaReservation
from app.modules.platform.quota.service import QuotaService


class LLMChatService:
    def __init__(
        self,
        llm: LLMInstance,
        *,
        first_token_timeout: float,
        quota: QuotaService | None,
        quota_policy: QuotaPolicy,
        sessions: SessionStore,
        prompts: PromptProvider,
    ) -> None:
        self._llm = llm
        self._first_token_timeout = first_token_timeout
        self._quota = quota
        self._quota_policy = quota_policy
        self._sessions = sessions
        self._prompts = prompts
        self._redaction = RedactionPolicy.for_llm_input()

    async def stream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str,
        request_id: str,
        deadline_seconds: float | None,
    ) -> AsyncIterator[str]:
        message = self._redaction.redact_text(message)
        stateful = bool(principal_id and session_id)
        history = await self._load_history(principal_id, session_id) if stateful else []
        messages = await self._build_messages(message, history)
        # Fresh key per call: a client-supplied request id repeated by a caller
        # would otherwise hand back the earlier reservation and never charge.
        reservation = await self._reserve(principal_id, uuid.uuid4().hex)

        context = LLMTraceContext(
            session_id=session_id or None,
            user_id=principal_id or None,
            request_id=request_id or None,
            tags=("grpc", "chat"),
        )
        reply: list[str] = []
        usage = UsageAccumulator()
        target = ""
        try:
            # ``astream`` is an async generator; the protocol-free return type
            # hides ``aclose``.
            upstream = cast(
                AsyncGenerator[Any, None],
                self._llm.astream(
                    messages,
                    context=context,
                    first_token_timeout=self._first_token_timeout,
                    deadline_seconds=deadline_seconds,
                ),
            )
            async with aclosing(upstream) as chunks:
                async for chunk in chunks:
                    usage.add(getattr(chunk, "usage_metadata", None))
                    target = target or str(
                        chunk.response_metadata.get("model_name", "")
                    )
                    text = chunk_text(chunk)
                    if text:
                        reply.append(text)
                        yield text
        except Exception as exc:
            await self._settle(reservation, refund=not reply)
            raise _translate(exc, delivered=bool(reply)) from exc
        except BaseException:
            # Cancelled or closed by the caller: a partial reply still counts.
            await self._settle(reservation, refund=not reply)
            raise

        await self._settle(reservation, refund=not reply)
        text = "".join(reply)
        self._report_usage(request_id, target, usage, messages, text)
        if stateful and text:
            await self._remember(principal_id, session_id, Turn(message, text))

    async def _build_messages(self, message: str, history: list[Turn]) -> list[Any]:
        from langchain_core.messages import AIMessage, HumanMessage, SystemMessage

        messages: list[Any] = [
            SystemMessage(content=await self._prompts.system_prompt())
        ]
        for turn in history:
            messages.append(HumanMessage(content=turn.user))
            messages.append(AIMessage(content=turn.assistant))
        messages.append(HumanMessage(content=message))
        return messages

    async def _load_history(self, principal_id: str, session_id: str) -> list[Turn]:
        try:
            return await self._sessions.load(principal_id, session_id)
        except Exception as exc:  # degrade to stateless rather than fail the chat
            logger.warning("chat.session.load_failed error={}", type(exc).__name__)
            return []

    async def _remember(self, principal_id: str, session_id: str, turn: Turn) -> None:
        try:
            await self._sessions.append(principal_id, session_id, turn)
        except Exception as exc:
            logger.warning("chat.session.append_failed error={}", type(exc).__name__)

    async def _reserve(
        self, principal_id: str, idempotency_key: str
    ) -> QuotaReservation | None:
        """One reply unit; None when unmetered or the quota backend failed."""
        if self._quota is None or not principal_id:
            return None
        try:
            return await self._quota.reserve(
                subject_id=principal_id,
                resource=self._quota_policy.resource,
                cost=1,
                policy=self._quota_policy,
                idempotency_key=idempotency_key,
            )
        except RateLimitError as exc:
            retry_after = (exc.headers or {}).get("Retry-After")
            raise ChatQuotaExhausted(
                int(retry_after) if retry_after and retry_after.isdigit() else None
            ) from exc
        except Exception as exc:  # metering must not take chat down
            logger.warning("chat.quota.reserve_failed error={}", type(exc).__name__)
            return None

    async def _settle(
        self, reservation: QuotaReservation | None, *, refund: bool
    ) -> None:
        if reservation is None or self._quota is None:
            return
        try:
            if refund:
                await self._quota.refund(reservation)
            else:
                await self._quota.finalize(reservation)
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
    ) -> None:
        result = usage.result(
            target=target,
            input_text="".join(chunk_text(m) for m in messages),
            output_text=reply,
        )
        logger.bind(
            request_id=request_id,
            target=result.target,
            input_tokens=result.input_tokens,
            output_tokens=result.output_tokens,
            total_tokens=result.total_tokens,
            estimated=result.estimated,
        ).info(
            "chat.llm.usage request_id={} target={} input_tokens={} "
            "output_tokens={} estimated={}",
            request_id,
            result.target,
            result.input_tokens,
            result.output_tokens,
            result.estimated,
        )


def _translate(exc: Exception, *, delivered: bool) -> ChatError:
    logger.warning(
        "chat.llm.failed delivered={} error={}", delivered, type(exc).__name__
    )
    if isinstance(exc, LLMDeadlineExceeded):
        return ChatDeadlineExceeded()
    if delivered:
        return ChatInterrupted()
    provider_error = exc.__cause__ if isinstance(exc, LLMStreamInterrupted) else exc
    if provider_error is not None and error_status_code(provider_error) == 429:
        return ChatModelRateLimited()
    return ChatModelUnavailable()
