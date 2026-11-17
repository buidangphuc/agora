"""No invented catalog remains in the shopping assistant."""

import importlib
import inspect
from pathlib import Path

import pytest

from app.modules.business.ai_assistant.factory import build_ai_assistant_services

_PKG = Path(__file__).parents[2] / "app" / "modules" / "business" / "ai_assistant"


@pytest.mark.parametrize("name", ["catalog", "retrieval"])
def test_demo_catalog_and_rag_retriever_modules_are_deleted(name: str):
    """Scenario: No invented catalog remains."""
    assert not (_PKG / f"{name}.py").exists()
    with pytest.raises(ModuleNotFoundError):
        importlib.import_module(f"app.modules.business.ai_assistant.{name}")


def test_build_ai_assistant_services_takes_no_rag_argument():
    """Scenario: No invented catalog remains."""
    params = inspect.signature(build_ai_assistant_services).parameters

    assert "rag" not in params
    assert "rag_min_score" not in params
    assert set(params) == {"grounding", "replier"}


def test_no_source_file_names_the_removed_symbols():
    """Scenario: No invented catalog remains."""
    root = _PKG.parents[2]
    hits = [
        str(path.relative_to(root))
        for path in root.rglob("*.py")
        if any(
            token in path.read_text(encoding="utf-8")
            for token in ("DemoCatalogRetriever", "RagProductRetriever")
        )
    ]

    assert hits == []


async def test_assistant_without_grounding_returns_no_demo_cards():
    """Scenario: No demo cards with RAG disabled."""
    from app.modules.business.ai_assistant.schemas import ShoppingAssistantRequest

    services = build_ai_assistant_services()

    answer = await services.shopping.answer(
        ShoppingAssistantRequest(message="tai nghe bluetooth")
    )

    assert answer.product_cards == []
