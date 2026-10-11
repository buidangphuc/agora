"""scripts/run_grpc.py must serve Recommend the same way the main entrypoint does."""

from __future__ import annotations

from types import SimpleNamespace

from scripts import run_grpc
from tests.factories import build_test_settings


async def test_run_grpc_registers_the_recommendation_provider(monkeypatch):
    settings = build_test_settings(CHAT_BACKEND="mock")
    sentinel = object()
    captured: dict = {}

    async def fake_open(app, settings, *, init_resources, addons):
        app.state.resources.recommendation_service = sentinel
        app.state.resources.rag_service = None

    async def fake_close(app):
        return None

    async def fake_serve(**kwargs):
        captured.update(kwargs)

    async def fake_close_streamer(streamer):
        return None

    monkeypatch.setattr(run_grpc, "get_settings", lambda: settings)
    monkeypatch.setattr(run_grpc, "open_application_resources", fake_open)
    monkeypatch.setattr(run_grpc, "close_application_resources", fake_close)
    monkeypatch.setattr(run_grpc, "serve", fake_serve)
    monkeypatch.setattr(run_grpc, "close_chat_streamer", fake_close_streamer)
    monkeypatch.setattr(run_grpc, "configure_logging", lambda **kw: None)
    monkeypatch.setattr(
        run_grpc, "build_chat_streamer", lambda *a, **kw: SimpleNamespace()
    )

    await run_grpc._main()

    provider = captured["recommendation_provider"]
    assert provider is not None
    assert provider() is sentinel
