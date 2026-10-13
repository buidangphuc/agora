import fakeredis.aioredis as fakeredis
import pytest
from pydantic import ValidationError

from app.modules.business.chat.echo import EchoChatService
from app.modules.business.chat.factory import build_chat_service
from scripts.check_env_example import ENV_EXAMPLE_PATH, parse_env_keys
from tests.factories import build_test_settings

CHAT_KEYS = (
    "CHAT_BACKEND",
    "CHAT_FIRST_TOKEN_TIMEOUT_SECONDS",
    "CHAT_SYSTEM_PROMPT",
    "CHAT_HISTORY_MAX_TURNS",
    "CHAT_HISTORY_MAX_TOKENS",
    "CHAT_HISTORY_TTL_SECONDS",
    "QUOTA_CHAT_REPLIES_PER_WINDOW",
    "QUOTA_CHAT_WINDOW_SECONDS",
)


def test_chat_defaults():
    settings = build_test_settings()

    assert settings.CHAT_BACKEND == "mock"
    assert settings.CHAT_FIRST_TOKEN_TIMEOUT_SECONDS == 8.0
    assert settings.CHAT_SYSTEM_PROMPT == ""
    assert settings.CHAT_HISTORY_MAX_TURNS == 10
    assert settings.CHAT_HISTORY_MAX_TOKENS == 1500
    assert settings.CHAT_HISTORY_TTL_SECONDS == 1800
    assert settings.QUOTA_CHAT_REPLIES_PER_WINDOW == 200
    assert settings.QUOTA_CHAT_WINDOW_SECONDS == 86400


def test_unknown_backend_fails_validation():
    with pytest.raises(ValidationError):
        build_test_settings(CHAT_BACKEND="other")


@pytest.mark.parametrize(
    "field",
    [
        "CHAT_FIRST_TOKEN_TIMEOUT_SECONDS",
        "CHAT_HISTORY_MAX_TOKENS",
        "CHAT_HISTORY_TTL_SECONDS",
        "QUOTA_CHAT_REPLIES_PER_WINDOW",
        "QUOTA_CHAT_WINDOW_SECONDS",
    ],
)
def test_positive_bounds(field):
    with pytest.raises(ValidationError):
        build_test_settings(**{field: 0})


def test_zero_history_turns_is_allowed():
    assert build_test_settings(CHAT_HISTORY_MAX_TURNS=0).CHAT_HISTORY_MAX_TURNS == 0


def test_env_example_lists_chat_keys():
    keys = parse_env_keys(ENV_EXAMPLE_PATH.read_text(encoding="utf-8"))

    assert set(CHAT_KEYS) <= keys


def test_mock_backend_builds_echo():
    chat = build_chat_service(
        build_test_settings(), quota=None, redis=fakeredis.FakeRedis()
    )

    assert isinstance(chat, EchoChatService)


def test_llm_router_needs_the_ai_extra(monkeypatch):
    monkeypatch.setattr(
        "app.modules.business.chat.factory.importlib.util.find_spec",
        lambda name: None,
    )

    with pytest.raises(RuntimeError, match="ai"):
        build_chat_service(
            build_test_settings(CHAT_BACKEND="llm_router"),
            quota=None,
            redis=fakeredis.FakeRedis(),
        )
