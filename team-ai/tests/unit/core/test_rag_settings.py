import pytest
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

RAG_KEYS = (
    "RAG_QDRANT_URL",
    "RAG_QDRANT_COLLECTION",
    "RAG_EMBED_SERVER_URL",
    "RAG_EMBED_SERVER_PATH",
    "RAG_EMBED_DIM",
    "RAG_EMBED_TIMEOUT_SECONDS",
)


def test_defaults() -> None:
    settings = build_test_settings()

    assert settings.RAG_ENABLED is False
    assert settings.RAG_QDRANT_COLLECTION == "rag_documents"
    assert settings.RAG_EMBED_SERVER_PATH == "/embed"


def test_rag_on_requires_its_backends() -> None:
    for overrides in (
        {"RAG_EMBED_SERVER_URL": ""},
        {"RAG_EMBED_SERVER_URL": "http://embed:8080", "RAG_QDRANT_URL": ""},
    ):
        with pytest.raises(ValidationError, match="RAG_ENABLED requires"):
            build_test_settings(RAG_ENABLED=True, **overrides)

    settings = build_test_settings(
        RAG_ENABLED=True, RAG_EMBED_SERVER_URL="http://embed:8080"
    )
    assert settings.RAG_EMBED_SERVER_URL == "http://embed:8080"


def test_env_example_lists_the_rag_keys_and_not_the_removed_one() -> None:
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text())

    assert set(RAG_KEYS) <= set(keys)
    assert "RAG_EMBED_MODEL" not in keys
