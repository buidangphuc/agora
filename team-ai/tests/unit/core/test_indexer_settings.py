import pytest
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

INDEXER_KEYS = (
    "KAFKA_BROKERS",
    "LISTING_INDEXER_TOPIC",
    "LISTING_INDEXER_GROUP",
    "LISTING_INDEXER_MAX_ATTEMPTS",
    "LISTING_INDEXER_BASE_BACKOFF_SECONDS",
)


def test_defaults_have_team_ai_names() -> None:
    settings = build_test_settings()

    assert settings.LISTING_INDEXER_TOPIC == "listing.events"
    assert settings.LISTING_INDEXER_GROUP == "team-ai-indexer"


def test_rag_needs_kafka_brokers() -> None:
    with pytest.raises(ValidationError, match="RAG_ENABLED requires KAFKA_BROKERS"):
        build_test_settings(
            RAG_ENABLED=True, RAG_EMBED_SERVER_URL="http://embed", KAFKA_BROKERS=""
        )


@pytest.mark.parametrize(
    "overrides",
    [
        {"LISTING_INDEXER_MAX_ATTEMPTS": 0},
        {"LISTING_INDEXER_BASE_BACKOFF_SECONDS": -1.0},
    ],
)
def test_retry_bounds(overrides: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        build_test_settings(**overrides)


def test_env_example_lists_every_indexer_key() -> None:
    assert set(INDEXER_KEYS) <= parse_env_keys(ENV_EXAMPLE_PATH.read_text())
