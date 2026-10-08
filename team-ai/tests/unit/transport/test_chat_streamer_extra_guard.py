"""CHAT_BACKEND=llm_router refuses to build without the `ai` extra."""

from __future__ import annotations

import importlib.util

import pytest

from app.core.config import Settings
from app.transport.grpc import chat_stream


def test_llm_router_without_the_ai_extra_fails_at_build(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    real = importlib.util.find_spec
    monkeypatch.setattr(
        chat_stream.importlib.util,
        "find_spec",
        lambda name, *a: None if name == "langchain_core" else real(name, *a),
    )
    settings = Settings(CHAT_BACKEND="llm_router", CHAT_MODEL="openai:primary")
    with pytest.raises(RuntimeError, match="'ai' extra"):
        chat_stream.build_chat_streamer(settings)
