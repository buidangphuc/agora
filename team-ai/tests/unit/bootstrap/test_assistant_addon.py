from contextlib import AsyncExitStack
from typing import Any

import grpc
import pytest

from app.bootstrap.addons.assistant import AssistantAddon
from app.bootstrap.addons.base import default_resource_addons
from app.bootstrap.resources import ApplicationResources
from tests.factories import build_test_settings


class _Channel:
    def __init__(self, addr: str) -> None:
        self.addr = addr
        self.closed = False

    def unary_unary(self, *args: Any, **kwargs: Any):
        raise AssertionError("no call expected")

    async def close(self) -> None:
        self.closed = True


@pytest.fixture
def channels(monkeypatch: pytest.MonkeyPatch) -> list[_Channel]:
    made: list[_Channel] = []

    def _make(addr: str, *args: Any, **kwargs: Any) -> _Channel:
        channel = _Channel(addr)
        made.append(channel)
        return channel

    monkeypatch.setattr(grpc.aio, "insecure_channel", _make)
    return made


async def test_addon_opens_search_and_listing_channels_and_closes_them_on_exit(
    channels: list[_Channel],
):
    """Scenario: Search is down (channels opened and closed in the bootstrap addon)."""
    resources = ApplicationResources()
    settings = build_test_settings(
        RECS_SEARCH_ADDR="search:50052", ASSISTANT_LISTING_ADDR="domain:50051"
    )

    async with AsyncExitStack() as stack:
        await AssistantAddon().open(resources, settings, stack)

        clients = resources.assistant_clients
        assert clients is not None
        assert clients.search is not None
        assert clients.listing is not None
        assert {c.addr for c in channels} == {"search:50052", "domain:50051"}
        assert not any(c.closed for c in channels)

    assert channels and all(c.closed for c in channels)


def test_addon_is_always_enabled_and_registered():
    """Scenario: Search is down (addon registered)."""
    assert AssistantAddon().is_enabled(build_test_settings()) is True
    assert any(a.name == AssistantAddon.name for a in default_resource_addons())


async def test_llm_backend_without_the_ai_extra_fails_at_boot(
    monkeypatch: pytest.MonkeyPatch, channels: list[_Channel]
):
    """Scenario: LLM backend without extra fails at boot."""
    monkeypatch.setattr("importlib.util.find_spec", lambda name, *a, **k: None)
    settings = build_test_settings(ASSISTANT_BACKEND="llm_router")

    with pytest.raises(RuntimeError, match="ai"):
        async with AsyncExitStack() as stack:
            await AssistantAddon().open(ApplicationResources(), settings, stack)
