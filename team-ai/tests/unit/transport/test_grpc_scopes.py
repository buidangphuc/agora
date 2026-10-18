from app.transport.grpc._pb.platform.ai.v1 import ai_pb2
from app.transport.grpc.scopes import (
    ADMIN_SCOPE,
    AI_CLASSIFY_SCOPE,
    AI_SERVICE_SCOPES,
    AI_USE_SCOPE,
    LISTING_READ_SCOPE,
    LISTING_WRITE_SCOPE,
    ai_use_scopes,
)


def test_scope_constants_are_frozen() -> None:
    assert AI_USE_SCOPE == "ai:use"
    assert LISTING_WRITE_SCOPE == "listing.write"
    assert LISTING_READ_SCOPE == "listing.read"
    assert ADMIN_SCOPE == "admin"
    assert AI_CLASSIFY_SCOPE == "ai.classify"
    assert AI_SERVICE_SCOPES == {
        "MagicListing": ("listing.write",),
        "ChatCopilot": ("listing.write",),
        "SummarizeReviews": ("listing.read",),
        "ClassifyTags": ("ai.classify",),
        "ScreenListing": ("ai.classify",),
    }


def test_ai_use_scopes_follow_the_flag() -> None:
    assert ai_use_scopes(True) == ("ai:use",)
    assert ai_use_scopes(False) == ()


def test_every_ai_rpc_is_gated_or_explicitly_open() -> None:
    service = ai_pb2.DESCRIPTOR.services_by_name["AIService"]
    rpcs = {m.name for m in service.methods}
    assert rpcs - {"ShoppingAssistant"} == set(AI_SERVICE_SCOPES)
    assert "ShoppingAssistant" not in AI_SERVICE_SCOPES


def test_screen_listing_scope_is_ai_classify() -> None:
    """Scenario: Every AIService RPC except ShoppingAssistant has a scope entry."""
    assert AI_SERVICE_SCOPES["ScreenListing"] == ("ai.classify",)
