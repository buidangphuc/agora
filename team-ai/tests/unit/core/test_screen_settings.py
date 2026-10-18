"""Settings for ai-listing-screen task 1 (SCREEN_* switches)."""

import pytest
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

SCREEN_KEYS = ("SCREEN_BACKEND", "SCREEN_MOCK_MARKER", "SCREEN_TIMEOUT_SECONDS")


def test_default_backend_is_mock_and_defaults_hold_scenario_default_backend_is_mock():
    settings = build_test_settings()

    assert settings.SCREEN_BACKEND == "mock"
    assert settings.SCREEN_MOCK_MARKER == "screen-review-marker"
    assert settings.SCREEN_TIMEOUT_SECONDS == 1.5


def test_llm_router_backend_is_accepted_scenario_default_backend_is_mock():
    assert (
        build_test_settings(SCREEN_BACKEND="llm_router").SCREEN_BACKEND == "llm_router"
    )


@pytest.mark.parametrize("value", ["other", "", "MOCK"])
def test_unknown_backend_fails_startup_scenario_unknown_backend_fails(value):
    with pytest.raises(ValidationError) as exc:
        build_test_settings(SCREEN_BACKEND=value)
    assert exc.value.errors()[0]["type"] == "literal_error"


@pytest.mark.parametrize("value", [0, -1])
def test_timeout_must_be_positive_scenario_default_backend_is_mock(value):
    with pytest.raises(ValidationError) as exc:
        build_test_settings(SCREEN_TIMEOUT_SECONDS=value)
    assert exc.value.errors()[0]["type"] == "greater_than"


def test_env_example_lists_screen_keys_scenario_default_backend_is_mock():
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))
    for key in SCREEN_KEYS:
        assert key in keys
