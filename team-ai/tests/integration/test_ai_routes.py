"""HTTP assistant routes: team-ai's bodies, no principal required."""

from app.bootstrap.application import create_app
from tests.factories import api_client_for, build_test_settings


async def test_assistant_returns_cards_and_followups():
    app = create_app(settings=build_test_settings())

    async with api_client_for(app) as client:
        response = await client.post(
            "/api/v1/ai/assistant", json={"message": "tai nghe bluetooth", "top_k": 2}
        )

    assert response.status_code == 200
    body = response.json()
    assert 1 <= len(body["product_cards"]) <= 2
    assert body["reply_text"]
    assert body["suggested_followups"]


async def test_magic_listing_returns_generated_fields():
    app = create_app(settings=build_test_settings())

    async with api_client_for(app) as client:
        response = await client.post(
            "/api/v1/ai/magic-listing", json={"title_hint": "áo thun nam"}
        )

    assert response.status_code == 200
    assert response.json()["generated_title"]


async def test_chat_copilot_returns_quick_replies():
    app = create_app(settings=build_test_settings())

    async with api_client_for(app) as client:
        response = await client.post(
            "/api/v1/ai/chat-copilot", json={"buyer_message": "còn hàng không?"}
        )

    assert response.status_code == 200
    assert response.json()["quick_replies"]


async def test_assistant_rejects_top_k_zero():
    app = create_app(settings=build_test_settings())

    async with api_client_for(app) as client:
        response = await client.post(
            "/api/v1/ai/assistant", json={"message": "x", "top_k": 0}
        )

    assert response.status_code == 422


async def test_magic_listing_rejects_short_title():
    app = create_app(settings=build_test_settings())

    async with api_client_for(app) as client:
        response = await client.post(
            "/api/v1/ai/magic-listing", json={"title_hint": "a"}
        )

    assert response.status_code == 422
