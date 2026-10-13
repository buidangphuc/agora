import pytest
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

TAXONOMY_KEYS = (
    "TAXONOMY_REDIS_DATABASE",
    "TAXONOMY_REDIS_PREFIX",
)


def test_defaults():
    settings = build_test_settings()

    assert settings.TAXONOMY_REDIS_DATABASE == 5
    assert settings.TAXONOMY_REDIS_PREFIX == "tagtax"


def test_taxonomy_database_must_differ_from_shared_redis_database():
    with pytest.raises(ValidationError, match="TAXONOMY_REDIS_DATABASE"):
        build_test_settings(REDIS_DATABASE=5, TAXONOMY_REDIS_DATABASE=5)


def test_env_example_lists_the_taxonomy_keys():
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))

    assert set(TAXONOMY_KEYS) <= keys
