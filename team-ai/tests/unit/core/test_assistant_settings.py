from pathlib import Path

import pytest
import yaml
from pydantic import ValidationError

from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

_COMPOSE = Path(__file__).parents[4] / "docker-compose.services.yaml"

ASSISTANT_KEYS = (
    "ASSISTANT_BACKEND",
    "ASSISTANT_TIMEOUT_SECONDS",
    "ASSISTANT_SEARCH_TIMEOUT_SECONDS",
    "ASSISTANT_LISTING_TIMEOUT_SECONDS",
    "ASSISTANT_LISTING_ADDR",
    "ASSISTANT_LISTING_PRINCIPAL_ID",
    "ASSISTANT_LISTING_PRINCIPAL_SCOPES",
)


def test_assistant_defaults():
    """Scenario: Invalid backend value is rejected at boot (defaults)."""
    settings = build_test_settings()

    assert settings.ASSISTANT_BACKEND == "mock"
    assert settings.ASSISTANT_TIMEOUT_SECONDS == 8.0
    assert settings.ASSISTANT_SEARCH_TIMEOUT_SECONDS == 0.8
    assert settings.ASSISTANT_LISTING_TIMEOUT_SECONDS == 0.5
    assert settings.ASSISTANT_LISTING_PRINCIPAL_ID == "team-ai"
    assert settings.ASSISTANT_LISTING_PRINCIPAL_SCOPES == "listing.read"


def test_llm_router_backend_is_accepted():
    """Scenario: Invalid backend value is rejected at boot (valid value)."""
    assert (
        build_test_settings(ASSISTANT_BACKEND="llm_router").ASSISTANT_BACKEND
        == "llm_router"
    )


def test_invalid_backend_value_is_rejected_at_boot():
    """Scenario: Invalid backend value is rejected at boot."""
    with pytest.raises(ValidationError, match="ASSISTANT_BACKEND"):
        build_test_settings(ASSISTANT_BACKEND="other")


@pytest.mark.parametrize(
    "field",
    [
        "ASSISTANT_TIMEOUT_SECONDS",
        "ASSISTANT_SEARCH_TIMEOUT_SECONDS",
        "ASSISTANT_LISTING_TIMEOUT_SECONDS",
    ],
)
def test_timeouts_must_be_positive(field):
    """Scenario: Invalid backend value is rejected at boot (bounds)."""
    with pytest.raises(ValidationError, match=field):
        build_test_settings(**{field: 0})


def test_assistant_rag_min_score_is_removed():
    """Scenario: Invalid backend value is rejected at boot (RAG min score retired)."""
    assert "ASSISTANT_RAG_MIN_SCORE" not in type(build_test_settings()).model_fields


def test_env_example_lists_assistant_keys_and_drops_rag_min_score():
    """Scenario: Invalid backend value is rejected at boot (env example)."""
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))

    assert set(ASSISTANT_KEYS) <= keys
    assert "ASSISTANT_RAG_MIN_SCORE" not in keys


def _team_ai_service() -> dict:
    return yaml.safe_load(_COMPOSE.read_text())["services"]["team-ai"]


def test_compose_points_the_assistant_at_domain_with_the_mock_backend():
    """Scenario: Published in-stock listing becomes a card (compose wiring)."""
    env = _team_ai_service()["environment"]
    pairs = dict(item.split("=", 1) for item in env if isinstance(item, str))

    assert pairs["ASSISTANT_LISTING_ADDR"] == "team-domain-svc:50051"
    assert pairs["ASSISTANT_BACKEND"] == "mock"


def test_compose_team_ai_waits_for_search_and_domain():
    """Scenario: Search is down (compose depends_on)."""
    depends = _team_ai_service()["depends_on"]

    assert {"team-search", "team-domain"} <= set(depends)


COPILOT_KEYS = ("COPILOT_BACKEND", "COPILOT_TIMEOUT_SECONDS")


def test_copilot_backend_accepts_only_known_values():
    """Scenario: Unknown backend value is rejected at boot (settings)"""
    assert build_test_settings(COPILOT_BACKEND="llm_router").COPILOT_BACKEND == (
        "llm_router"
    )
    with pytest.raises(ValidationError, match="COPILOT_BACKEND"):
        build_test_settings(COPILOT_BACKEND="gpt")


def test_copilot_timeout_must_be_positive():
    """Scenario: Unknown backend value is rejected at boot (timeout bound)"""
    with pytest.raises(ValidationError, match="COPILOT_TIMEOUT_SECONDS"):
        build_test_settings(COPILOT_TIMEOUT_SECONDS=0)


def test_env_example_lists_copilot_keys():
    """Scenario: Mock is the default and is deterministic (env example)"""
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))

    assert set(COPILOT_KEYS) <= keys
