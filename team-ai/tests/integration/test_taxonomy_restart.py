"""Promotion survives an app restart on the same Redis (fakeredis stands in for it)."""

import fakeredis
import pytest
from fakeredis import FakeServer

from app.bootstrap.addons import taxonomy
from app.bootstrap.application import create_app
from tests.factories import api_client_for, api_key_entry, build_test_settings

ADMIN = {"Authorization": "Bearer admin-token"}
KEYS = api_key_entry("admin", "admin-token", "admin", "ai.classify")
BASE = "/api/v1/ai"
_EXPLORE = {
    "batch_listings": [
        {"listing_id": "a", "title": "Sạc GaN công suất 77W"},
        {"listing_id": "b", "title": "Củ sạc laptop 77W"},
        {"listing_id": "c", "title": "Pin dự phòng 30000mAh"},
        {"listing_id": "d", "title": "Sạc dự phòng 30000mAh"},
    ],
    "min_frequency": 2,
    "min_confidence": 0.8,
}


@pytest.fixture()
def server(monkeypatch: pytest.MonkeyPatch) -> FakeServer:
    fake = FakeServer()
    # asdlc: allow-mock the swapped client is Redis, an external dependency (fakeredis); the taxonomy is real
    monkeypatch.setattr(
        taxonomy,
        "build_taxonomy_redis_client",
        lambda settings: fakeredis.FakeAsyncRedis(server=fake, decode_responses=True),
    )
    return fake


def _app():
    return create_app(
        settings=build_test_settings(AUTH_API_KEYS=KEYS),
        init_resources=False,
    )


async def _tags(client, status: str) -> dict[str, dict]:
    response = await client.get(
        f"{BASE}/tags", params={"status": status}, headers=ADMIN
    )
    return {t["slug"]: t for t in response.json()["tags"]}


async def test_promoted_tag_and_candidates_survive_restart(server):
    async with api_client_for(_app()) as client:
        await client.post(f"{BASE}/tags/explore", json=_EXPLORE, headers=ADMIN)
        promoted = await client.post(
            f"{BASE}/tags/promote",
            json={"tag_slugs": ["cong-suat-77w"], "add_synonyms": ["77 w"]},
            headers=ADMIN,
        )
        assert promoted.json()["total_promoted"] == 1
        exploring_before = set(await _tags(client, "exploring"))
    assert exploring_before

    async with api_client_for(_app()) as client:
        assert "77 w" in (await _tags(client, "promoted"))["cong-suat-77w"]["synonyms"]
        assert set(await _tags(client, "exploring")) == exploring_before
