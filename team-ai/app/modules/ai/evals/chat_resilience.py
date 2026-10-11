"""Eval target: the REAL ``LLMRouterChatStreamer`` over scripted fake chat models.

A case describes a fallback chain, per-target scripts (chunks / 429 / 5xx / timeout /
hang / mid-stream failure) and one or more requests (optionally advancing a fake
breaker clock between them). The target returns a canonical string per request so a
plain ``exact`` evaluator can assert fallback order, the first-chunk rule and error
classes. Imports langchain (via ``testing``): eval/test use only.

Case ``input`` shape::

    {
        "chain": ["a", "b"],
        "threshold": 3,
        "cooldown": 30,
        "max_attempts": 1,
        "first_token_timeout": 0.05,
        "report_sent": false,
        "scripts": {"a": [{"fail_at": 0, "status": 429}], "b": [{"chunks": ["hi"]}]},
        "requests": [{"message": "hello", "advance": 0}],
    }

Output: ``"reply=<text> err=<none|Class:kind> calls=<targets>"`` per request, joined
with `` ; `` (plus `` sent=<model input>`` when ``report_sent``).
"""

from __future__ import annotations

from typing import Any

from app.core.config import Settings
from app.core.resilience import CircuitBreakerPolicy
from app.modules.ai.llm.router import ModelRouter
from app.modules.ai.llm.testing import Script, ScriptedProvider, StatusError
from app.transport.grpc.chat_stream import (
    ChainExhausted,
    ChatStreamError,
    LLMRouterChatStreamer,
    StreamInterrupted,
)


class _Clock:
    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now


async def _no_sleep(_: float) -> None:
    return None


def _script(spec: dict[str, Any]) -> Script:
    error: BaseException | None = None
    if "status" in spec:
        error = StatusError(int(spec["status"]))
    elif spec.get("error") == "timeout":
        error = TimeoutError()
    elif spec.get("error") == "connection":
        error = ConnectionError("scripted")
    return Script(
        chunks=tuple(spec.get("chunks", ("ok",))),
        fail_at=spec.get("fail_at"),
        error=error,
        hang=bool(spec.get("hang", False)),
    )


def _describe(exc: BaseException | None) -> str:
    if exc is None:
        return "none"
    if isinstance(exc, ChainExhausted):
        kind = exc.last_kind.value if exc.last_kind else "none"
        return f"ChainExhausted:{kind}"
    if isinstance(exc, StreamInterrupted):
        return f"StreamInterrupted:{exc.kind.value}"
    return type(exc).__name__


async def run_chat_case(spec: dict[str, Any], base_settings: Settings) -> str:
    chain: list[str] = list(spec["chain"])
    provider = ScriptedProvider()
    for target, scripts in spec.get("scripts", {}).items():
        provider.queue(target, *[_script(s) for s in scripts])
    settings = base_settings.model_copy(
        update={
            "CHAT_MODEL": chain[0],
            "CHAT_FALLBACK_MODELS": ",".join(chain[1:]),
        }
    )
    clock = _Clock()
    router = ModelRouter(
        settings,
        model_builder=provider.builder,
        breaker_policy=CircuitBreakerPolicy(
            failure_threshold=int(spec.get("threshold", 3)),
            cooldown_seconds=float(spec.get("cooldown", 30)),
        ),
        clock=clock,
    )
    streamer = LLMRouterChatStreamer(
        router,
        first_token_timeout_seconds=float(spec.get("first_token_timeout", 5)),
        max_attempts=int(spec.get("max_attempts", 3)),
        sleep=_no_sleep,
    )

    lines: list[str] = []
    for request in spec["requests"]:
        clock.now += float(request.get("advance", 0))
        seen = len(provider.calls)
        deltas: list[str] = []
        error: BaseException | None = None
        try:
            async for delta in streamer.astream(
                request["message"], session_id="", principal_id="eval"
            ):
                deltas.append(delta)
        except ChatStreamError as exc:
            error = exc
        calls = ",".join(call.target for call in provider.calls[seen:]) or "-"
        line = f"reply={''.join(deltas)} err={_describe(error)} calls={calls}"
        if spec.get("report_sent") and provider.calls[seen:]:
            sent = provider.calls[-1].messages[-1].content
            line += f" sent={sent}"
        lines.append(line)
    return " ; ".join(lines)
