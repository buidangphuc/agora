"""Token-usage capture for streamed chat replies.

Providers report usage on (usually the last) stream chunk as
``AIMessageChunk.usage_metadata``. ``UsageAccumulator`` sums what arrives;
``ChatUsage`` is the per-call result, with a character-based estimate (flagged
``estimated=True``) when the provider reported nothing.
"""

from __future__ import annotations

from dataclasses import dataclass
from math import ceil
from typing import Any

CHARS_PER_TOKEN = 4


def estimate_tokens(text: str) -> int:
    return ceil(len(text) / CHARS_PER_TOKEN) if text else 0


@dataclass(frozen=True)
class ChatUsage:
    target: str
    input_tokens: int
    output_tokens: int
    total_tokens: int
    estimated: bool = False


class UsageAccumulator:
    def __init__(self) -> None:
        self._usage: dict[str, Any] | None = None

    def add(self, usage_metadata: Any) -> None:
        if not usage_metadata:
            return
        from langchain_core.messages.ai import add_usage

        self._usage = add_usage(self._usage, usage_metadata)  # type: ignore[arg-type]

    def result(self, *, target: str, input_text: str, output_text: str) -> ChatUsage:
        usage = self._usage
        if usage:
            input_tokens = int(usage.get("input_tokens") or 0)
            output_tokens = int(usage.get("output_tokens") or 0)
            total = int(usage.get("total_tokens") or (input_tokens + output_tokens))
            return ChatUsage(target, input_tokens, output_tokens, total)
        input_tokens = estimate_tokens(input_text)
        output_tokens = estimate_tokens(output_text)
        return ChatUsage(
            target,
            input_tokens,
            output_tokens,
            input_tokens + output_tokens,
            estimated=True,
        )
