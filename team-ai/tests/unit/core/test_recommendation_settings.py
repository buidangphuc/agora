import pytest
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

RECS_KEYS = (
    "RECS_QDRANT_URL",
    "RECS_QDRANT_COLLECTION",
    "RECS_VECTOR_DIM",
    "RECS_QDRANT_DISTANCE",
    "RECS_CANDIDATE_TOP_K",
    "RECS_RESULT_TOP_K",
    "RECS_CACHE_PREFIX",
    "RECS_CACHE_SCHEMA_VERSION",
    "RECS_RETRIEVE_TIMEOUT_MS",
    "RECS_MODEL_VERSION",
    "RECS_FEATURESTORE_REDIS_URL",
    "RECS_NEARLINE_REDIS_URL",
    "RECS_NEARLINE_PREFIX",
    "RECS_NEARLINE_MIN_IMPRESSIONS",
    "RECS_NEARLINE_TIMEOUT_MS",
)

# The RECS_* and REDIS_* values platform-gitops sets for team-ai in production,
# minus RECS_CACHE_TTL_SECONDS (dropped).
GITOPS_VALUES = {
    "ENVIRONMENT": "production",
    "AUTH_API_KEYS": "svc:admin:" + "0" * 64,  # pragma: allowlist secret
    "DOCS_ENABLED": False,
    "CORS_ALLOW_ORIGINS": "https://app.example.com",
    "TRUSTED_HOSTS": "app.example.com",
    "REDIS_HOST": "valkey",
    "REDIS_PORT": 6379,
    "RECS_QDRANT_URL": "http://qdrant:6333",
    "RECS_QDRANT_COLLECTION": "item_als_vectors",
    "RECS_CACHE_PREFIX": "recs",
    "RECS_CACHE_SCHEMA_VERSION": "v1",
    "RECS_FEATURESTORE_REDIS_URL": "redis://valkey:6379/2",
    "RECS_NEARLINE_REDIS_URL": "redis://valkey:6379/0",
    "RECS_RETRIEVE_TIMEOUT_MS": 15,
}


def test_defaults_are_team_ais():
    settings = build_test_settings()

    assert settings.RECS_QDRANT_URL == "http://localhost:6333"
    assert settings.RECS_QDRANT_COLLECTION == "item_als_vectors"
    assert settings.RECS_VECTOR_DIM == 64
    assert settings.RECS_QDRANT_DISTANCE == "Cosine"
    assert (settings.RECS_CANDIDATE_TOP_K, settings.RECS_RESULT_TOP_K) == (100, 10)
    assert (settings.RECS_CACHE_PREFIX, settings.RECS_CACHE_SCHEMA_VERSION) == (
        "recs",
        "v1",
    )
    assert settings.RECS_RETRIEVE_TIMEOUT_MS == 15
    assert settings.RECS_MODEL_VERSION == "serving-fallback"
    assert settings.RECS_FEATURESTORE_REDIS_URL == ""
    assert settings.RECS_NEARLINE_REDIS_URL == ""
    assert settings.RECS_NEARLINE_PREFIX == "recs:nearline"
    assert settings.RECS_NEARLINE_MIN_IMPRESSIONS == 1.0
    assert settings.RECS_NEARLINE_TIMEOUT_MS == 20


@pytest.mark.parametrize(
    "overrides",
    [{"RECS_QDRANT_DISTANCE": "Manhattan"}],
)
def test_enum_settings_reject_unknown_values(overrides: dict[str, str]):
    with pytest.raises(ValidationError):
        build_test_settings(**overrides)


def test_gitops_values_validate():
    settings = build_test_settings(**GITOPS_VALUES)

    assert settings.RECS_QDRANT_URL == "http://qdrant:6333"


def test_env_example_lists_the_recs_keys():
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))

    assert set(RECS_KEYS) <= keys
