from typing import Any

import pytest

from app.modules.business.listing_screen.factory import build_screen_service
from app.modules.business.listing_screen.mock import MockScreenService
from tests.factories import build_test_settings


def test_mock_backend_builds_mock_service():
    """Scenario: Mock backend allows a listing without the marker."""
    svc = build_screen_service(build_test_settings())

    assert isinstance(svc, MockScreenService)


def test_llm_router_without_extra_fails_at_boot(monkeypatch: pytest.MonkeyPatch):
    """Scenario: llm_router without the ai extra fails at boot."""
    monkeypatch.setattr(
        "app.modules.business.listing_screen.factory.importlib.util.find_spec",
        lambda name, *a, **k: None,
    )

    with pytest.raises(RuntimeError, match="ai"):
        build_screen_service(build_test_settings(SCREEN_BACKEND="llm_router"))


def test_llm_router_with_extra_builds_the_llm_service(monkeypatch: pytest.MonkeyPatch):
    """Scenario: LLM review verdict is returned with its labels."""
    from app.modules.business.listing_screen.service import LLMScreenService

    built: dict[str, Any] = {}

    def _fake_build(settings: Any, **kwargs: Any) -> object:
        built.update(kwargs)
        return object()

    monkeypatch.setattr("app.modules.ai.llm.runtime.build_llm_instance", _fake_build)

    svc = build_screen_service(build_test_settings(SCREEN_BACKEND="llm_router"))

    assert isinstance(svc, LLMScreenService)
    assert built  # the LLM chain was built lazily, only for this backend
