from app.modules.business.ai_assistant.schemas import ProductCard
from app.modules.business.ai_assistant.shopping import MockAssistantReplier


async def test_mock_replier_names_first_card_title_and_is_stable():
    """Scenario: Mock reply is deterministic over real cards."""
    cards = [
        ProductCard(listing_id="l-1", title="Giày chạy bộ A", price=450000),
        ProductCard(listing_id="l-2", title="Giày chạy bộ B", price=520000),
    ]
    replier = MockAssistantReplier()

    first = await replier.compose("giày chạy bộ", cards)
    second = await replier.compose("giày chạy bộ", cards)

    assert first.text == second.text
    assert first.backend == "mock"
    assert "Giày chạy bộ A" in first.text
    assert any(s in first.text for s in ("450.000", "450,000", "450000"))
