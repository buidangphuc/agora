from __future__ import annotations

import pytest
from httpx import ASGITransport, AsyncClient

from app.bootstrap.application import create_app
from tests.factories import build_test_settings


@pytest.fixture
def test_settings():
    return build_test_settings()


@pytest.mark.asyncio
async def test_tag_classifier_endpoints(test_settings):
    app = create_app(settings=test_settings, init_resources=False)
    transport = ASGITransport(app=app)

    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # 1. Classify endpoint
        classify_res = await client.post(
            "/api/v1/ai/tags/classify",
            json={
                "title": "Tai nghe Bluetooth 5.3 chống ồn ANC sạc nhanh 65W GaN",
                "description": "Chuẩn chống nước IPX7 pin khủng",
                "category_id": "cat-electronics",
            },
        )
        assert classify_res.status_code == 200
        data = classify_res.json()
        canonical_slugs = [t["slug"] for t in data["canonical_tags"]]
        assert "bluetooth-5-3" in canonical_slugs
        assert "chong-on-chu-dong-anc" in canonical_slugs
        assert "connectivity" in data["suggested_facet_filters"]

        # 2. List tags endpoint
        list_res = await client.get("/api/v1/ai/tags?category_id=cat-electronics")
        assert list_res.status_code == 200
        list_data = list_res.json()
        assert list_data["canonical_count"] > 0

        # 3. Explore endpoint
        explore_res = await client.post(
            "/api/v1/ai/tags/explore",
            json={
                "batch_listings": [
                    {
                        "listing_id": "item-1",
                        "title": "Cáp sạc 100W Anker bọc dù",
                        "description": "Hỗ trợ công suất 100W",
                        "category_id": "cat-electronics",
                    },
                    {
                        "listing_id": "item-2",
                        "title": "Củ sạc laptop 100W GaN nhỏ gọn",
                        "description": "Công suất 100W",
                        "category_id": "cat-electronics",
                    },
                ],
                "min_frequency": 2,
            },
        )
        assert explore_res.status_code == 200
        explore_data = explore_res.json()
        assert explore_data["total_processed"] == 2

        # 4. Promote endpoint
        promote_res = await client.post(
            "/api/v1/ai/tags/promote",
            json={
                "tag_slugs": ["cong-suat-100w"],
                "target_category_id": "cat-electronics",
                "add_synonyms": ["sac 100w"],
            },
        )
        assert promote_res.status_code == 200
        promote_data = promote_res.json()
        assert promote_data["total_promoted"] == 1
