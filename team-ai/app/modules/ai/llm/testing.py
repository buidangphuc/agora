"""Scripted chat models for tests and evals (no network, deterministic).

``ScriptedProvider`` hands out ``ScriptedChatModel`` instances per target; each
call to a model consumes the next ``Script`` queued for that target. A script can
stream chunks, raise an exception at a chosen position (before the first chunk =
``fail_at=0``), or hang forever (first-token timeout tests).

Imports langchain at module import — only import this from tests/evals.
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from contextlib import aclosing
from dataclasses import dataclass, field
from typing import Any

from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessageChunk, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatGenerationChunk, ChatResult


@dataclass
class Script:
    chunks: tuple[str, ...] = ("ok",)
    # Raise ``error`` after emitting this many chunks (0 = before the first one).
    fail_at: int | None = None
    error: BaseException | None = None
    # Never yield anything (simulates a hung provider).
    hang: bool = False
    usage: dict[str, int] | None = None
    # Seconds to sleep before each chunk (keep tiny in tests).
    delay: float = 0.0


@dataclass
class ScriptedCall:
    target: str
    messages: list[BaseMessage]
    # The ``config=`` handed to ``astream`` (trace tags/metadata/callbacks).
    config: dict[str, Any] | None = None
    cancelled: bool = False


class StatusError(Exception):
    """Provider-style HTTP error carrying ``status_code``."""

    def __init__(self, status_code: int, message: str = "") -> None:
        super().__init__(message or f"HTTP {status_code}")
        self.status_code = status_code


@dataclass
class ScriptedProvider:
    scripts: dict[str, list[Script]] = field(default_factory=dict)
    default: Script = field(default_factory=Script)
    calls: list[ScriptedCall] = field(default_factory=list)
    built: list[str] = field(default_factory=list)

    def queue(self, target: str, *scripts: Script) -> ScriptedProvider:
        self.scripts.setdefault(target, []).extend(scripts)
        return self

    def builder(self, target: str) -> ScriptedChatModel:
        self.built.append(target)
        return ScriptedChatModel(target=target, provider=self)

    def next_script(self, target: str) -> Script:
        queue = self.scripts.get(target)
        return queue.pop(0) if queue else self.default

    def calls_to(self, target: str) -> list[ScriptedCall]:
        return [call for call in self.calls if call.target == target]


class ScriptedChatModel(BaseChatModel):
    target: str
    provider: Any

    async def astream(  # type: ignore[override]
        self, input, config=None, *, stop=None, **kwargs
    ) -> AsyncIterator[Any]:
        messages = input if isinstance(input, list) else [input]
        call = ScriptedCall(self.target, list(messages), dict(config or {}) or None)
        self.provider.calls.append(call)
        try:
            async with aclosing(
                super().astream(input, config, stop=stop, **kwargs)
            ) as inner:
                async for chunk in inner:
                    yield chunk
        except (asyncio.CancelledError, GeneratorExit):
            call.cancelled = True
            raise

    @property
    def _llm_type(self) -> str:
        return "scripted"

    def _generate(  # pragma: no cover - streaming is the only path used
        self, messages, stop=None, run_manager=None, **kwargs
    ) -> ChatResult:
        raise NotImplementedError("ScriptedChatModel only supports astream")

    async def _agenerate(
        self, messages, stop=None, run_manager=None, **kwargs
    ) -> ChatResult:
        text = ""
        async for chunk in self._astream(messages, stop, run_manager, **kwargs):
            text += str(chunk.message.content)
        return ChatResult(generations=[ChatGeneration(message=AIMessageChunk(text))])

    async def _astream(
        self, messages, stop=None, run_manager=None, **kwargs
    ) -> AsyncIterator[ChatGenerationChunk]:
        script: Script = self.provider.next_script(self.target)
        if script.hang:
            await asyncio.Event().wait()
        for index, text in enumerate(script.chunks):
            if script.fail_at is not None and index == script.fail_at:
                raise script.error or StatusError(500)
            if script.delay:
                await asyncio.sleep(script.delay)
            yield ChatGenerationChunk(message=AIMessageChunk(content=text))
        if script.fail_at is not None and script.fail_at >= len(script.chunks):
            raise script.error or StatusError(500)
        if script.usage is not None:
            yield ChatGenerationChunk(
                message=AIMessageChunk(content="", usage_metadata=script.usage)  # type: ignore[arg-type]
            )
