import importlib
import platform

import pytest

SERVICES = {
    "ai.v1.ai_pb2": (
        "platform.ai.v1.AIService",
        {
            "ShoppingAssistant",
            "MagicListing",
            "ChatCopilot",
            "SummarizeReviews",
            "ClassifyTags",
        },
    ),
    "chat.v1.chat_pb2": ("platform.chat.v1.ChatService", None),
    "search.v1.search_pb2": ("platform.search.v1.SearchService", None),
    "recommendation.v1.recommendation_pb2": (
        "platform.recommendation.v1.RecommendationService",
        None,
    ),
}


@pytest.mark.parametrize("module", SERVICES)
def test_stub_exposes_contract_service(module: str) -> None:
    pb = importlib.import_module(f"app.transport.grpc._pb.platform.{module}")
    full_name, methods = SERVICES[module]
    service = pb.DESCRIPTOR.services_by_name[full_name.rsplit(".", 1)[1]]

    assert service.full_name == full_name
    if methods is not None:
        assert {m.name for m in service.methods} == methods


def test_stdlib_platform_is_not_shadowed() -> None:
    importlib.import_module("app.transport.grpc._pb.platform.ai.v1.ai_pb2")

    assert hasattr(platform, "system")
    assert platform.__file__ is not None
    assert "_pb" not in platform.__file__
