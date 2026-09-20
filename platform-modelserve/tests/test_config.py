"""Unit tests for modelserve configuration."""

from __future__ import annotations

from modelserve.config import Settings


def test_default_settings() -> None:
    settings = Settings()
    assert settings.router_port == 8100
    assert settings.tei_embed_url == "http://tei-embed:8101"
    assert settings.tei_rerank_url == "http://tei-rerank:8102"
    assert settings.vllm_url == "http://vllm:8103"
    assert settings.embed_cache_enabled is True
    assert settings.max_queue_depth == 100


def test_custom_settings(monkeypatch) -> None:
    monkeypatch.setenv("ROUTER_PORT", "8999")
    monkeypatch.setenv("MODEL_VERSION", "custom-model-v2")
    settings = Settings()
    assert settings.router_port == 8999
    assert settings.model_version == "custom-model-v2"
