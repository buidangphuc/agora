import socket
from typing import Any

import pytest

from app.modules.business.listing_screen.factory import build_screen_service
from app.modules.business.listing_screen.ports import ScreenVerdict
from tests.factories import build_test_settings

MARKER = "screen-review-marker"


def _mock(**overrides: Any):
    return build_screen_service(build_test_settings(**overrides))


async def test_default_settings_allow_clean_seed_text():
    """Scenario: Default mock settings allow ordinary seed listing text."""
    settings = build_test_settings()
    assert settings.SCREEN_MOCK_MARKER == MARKER

    result = await build_screen_service(settings).screen(
        title="iPhone 15 Pro Max 256GB",
        description="Like new, original box, battery health 98%.",
    )

    assert result.verdict == ScreenVerdict.ALLOW
    assert list(result.labels) == []
    assert result.backend == "mock"


async def test_marker_in_title_is_review_with_mock_marker_label():
    """Scenario: Mock backend flags a listing containing the marker."""
    result = await _mock().screen(title=f"Cheap phone {MARKER}", description="ok")

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["mock_marker"]
    assert result.backend == "mock"


async def test_marker_in_description_is_review():
    """Scenario: Mock backend flags a listing containing the marker."""
    result = await _mock().screen(title="Cheap phone", description=f"see {MARKER}.")

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["mock_marker"]


async def test_clean_text_is_allow_without_labels():
    """Scenario: Mock backend allows a listing without the marker."""
    result = await _mock().screen(title="Road bike", description="Barely used")

    assert result.verdict == ScreenVerdict.ALLOW
    assert list(result.labels) == []
    assert result.backend == "mock"


async def test_configured_marker_is_used():
    """Scenario: Mock backend flags a listing containing the marker."""
    svc = _mock(SCREEN_MOCK_MARKER="custom-flag")

    flagged = await svc.screen(title="x custom-flag y", description="")
    default_marker = await svc.screen(title=MARKER, description="")

    assert flagged.verdict == ScreenVerdict.REVIEW
    assert default_marker.verdict == ScreenVerdict.ALLOW


async def test_marker_matches_case_insensitively_past_8000_chars():
    """Scenario: Marker in the description is matched case-insensitively at any position."""
    description = "a" * (20000 - len(MARKER)) + MARKER.upper()
    assert len(description) == 20000

    result = await _mock().screen(title="Clean title", description=description)

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["mock_marker"]


async def test_marker_in_mixed_case_title_is_review():
    """Scenario: Marker in the description is matched case-insensitively at any position."""
    result = await _mock().screen(title="Screen-Review-MARKER here", description="")

    assert result.verdict == ScreenVerdict.REVIEW


async def test_mock_is_deterministic_and_offline(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Mock verdict is deterministic and makes no outbound call."""

    def _boom(*_a: Any, **_k: Any):
        raise AssertionError("mock screen must not use the LLM or the network")

    monkeypatch.setattr("app.modules.ai.llm.runtime.build_llm_instance", _boom)
    monkeypatch.setattr(socket.socket, "connect", _boom)
    svc = _mock()

    first = await svc.screen(title=f"a {MARKER}", description="d", listing_id="l1")
    second = await svc.screen(title=f"a {MARKER}", description="d", listing_id="l1")

    assert first == second
    assert first.verdict == ScreenVerdict.REVIEW
