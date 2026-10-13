"""Tag routes: scopes on every route, team-ai bodies."""

import pytest

from app.bootstrap.application import create_app
from tests.factories import api_client_for, api_key_entry, build_test_settings

ADMIN = {"Authorization": "Bearer admin-token"}
SERVICE = {"Authorization": "Bearer service-token"}
BASE = "/api/v1/ai"

_EXPLORE = {
    "batch_listings": [
        {"listing_id": "a", "title": "Sạc GaN công suất 77W"},
        {"listing_id": "b", "title": "Củ sạc laptop 77W"},
    ],
    "min_frequency": 2,
    "min_confidence": 0.8,
}
_CLASSIFY = {"title": "Tai nghe Bluetooth 5.3 chống ồn chủ động, sạc nhanh 65W"}
_SKU = {
    "spu_title": "Điện thoại Titan",
    "variants": [{"variant_id": "v1", "name": "Xanh Titan / 512GB"}],
}

# (method, path, json, required scope)
ROUTES = [
    ("POST", "/tags/classify", _CLASSIFY, "ai.classify"),
    ("POST", "/tags/classify-sku-hierarchy", _SKU, "ai.classify"),
    ("GET", "/tags", None, "ai.classify"),
    ("POST", "/tags/explore", _EXPLORE, "admin"),
    ("POST", "/tags/promote", {"tag_slugs": ["cong-suat-77w"]}, "admin"),
]


def _app():
    return create_app(
        settings=build_test_settings(
            AUTH_API_KEYS=", ".join(
                (
                    api_key_entry("admin", "admin-token", "admin", "ai.classify"),
                    api_key_entry("svc", "service-token", "ai.classify"),
                )
            )
        ),
        init_resources=False,
    )


@pytest.mark.parametrize(("method", "path", "body", "scope"), ROUTES)
async def test_missing_token_is_401(method, path, body, scope):
    async with api_client_for(_app()) as client:
        response = await client.request(method, BASE + path, json=body)

    assert response.status_code == 401


@pytest.mark.parametrize(("method", "path", "body", "scope"), ROUTES)
async def test_service_key_gets_only_the_read_routes(method, path, body, scope):
    async with api_client_for(_app()) as client:
        response = await client.request(method, BASE + path, json=body, headers=SERVICE)

    assert response.status_code == (403 if scope == "admin" else 200)


@pytest.mark.parametrize(("method", "path", "body", "scope"), ROUTES)
async def test_admin_key_gets_all_routes(method, path, body, scope):
    async with api_client_for(_app()) as client:
        response = await client.request(method, BASE + path, json=body, headers=ADMIN)

    assert response.status_code == 200


async def test_classify_returns_expected_slugs():
    async with api_client_for(_app()) as client:
        response = await client.post(
            f"{BASE}/tags/classify", json=_CLASSIFY, headers=SERVICE
        )

    slugs = {t["slug"] for t in response.json()["canonical_tags"]}
    assert {"bluetooth-5-3", "chong-on-chu-dong-anc", "sac-nhanh-65w-gan"} <= slugs


async def test_list_filters_by_query():
    async with api_client_for(_app()) as client:
        response = await client.get(
            f"{BASE}/tags", params={"facet_group": "connectivity"}, headers=SERVICE
        )

    tags = response.json()["tags"]
    assert tags
    assert {t["facet_group"] for t in tags} == {"connectivity"}


async def test_explore_then_promote_then_list():
    async with api_client_for(_app()) as client:
        explored = await client.post(
            f"{BASE}/tags/explore", json=_EXPLORE, headers=ADMIN
        )
        promoted = await client.post(
            f"{BASE}/tags/promote",
            json={"tag_slugs": ["cong-suat-77w"], "add_synonyms": ["77 w"]},
            headers=ADMIN,
        )
        listed = await client.get(
            f"{BASE}/tags", params={"status": "promoted"}, headers=SERVICE
        )

    assert explored.json()["new_candidate_count"] == 1
    assert promoted.json()["total_promoted"] == 1
    by_slug = {t["slug"]: t for t in listed.json()["tags"]}
    assert "77 w" in by_slug["cong-suat-77w"]["synonyms"]
