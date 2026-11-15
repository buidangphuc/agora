"""Copilot backend selection, LLM path and its fallbacks.

Interfaces these tests pin (implementer creates them):
  Settings.COPILOT_BACKEND: Literal["mock", "llm_router"] = "mock"
  Settings.COPILOT_TIMEOUT_SECONDS: float = 3.0 (> 0)
  ai_assistant.factory.build_copilot_service(settings, listing_lookup) -> ChatCopilotService
      llm_router builds its chain lazily through app.modules.ai.llm.runtime.build_llm_instance
  ai_assistant.copilot_llm.COPILOT_PROMPT_VERSION: str
  schemas.ChatCopilotResponse.model_version: str = "rules-v1"; the LLM path sets
      "llm_router:" + COPILOT_PROMPT_VERSION
  LLM reply format: a JSON array of draft strings (3 drafts)
"""

import asyncio
import importlib.util
import json
import time
from typing import Any

import pytest
from langchain_core.messages import AIMessage
from pydantic import ValidationError

from app.modules.business.ai_assistant.copilot_llm import COPILOT_PROMPT_VERSION
from app.modules.business.ai_assistant.factory import build_copilot_service
from app.modules.business.ai_assistant.schemas import ChatCopilotRequest
from tests.factories import build_test_settings
from tests.unit.modules.business.ai_assistant.test_copilot_grounding import (
    LISTING,
    OTHER_SELLER,
    PHONE,
    SELLER,
    STOCK_Q,
    FakeListing,
    _facts,
    claims_availability,
    promises_delivery,
)

LLM_DRAFTS = [
    "Dạ chào bạn, sản phẩm này shop còn hàng ạ, bạn cần size nào để shop giữ giúp nhé!",
    "Chào bạn, shop còn hàng ạ. Bạn đặt đơn để shop chuẩn bị hàng cho bạn nha.",
    "Dạ shop còn hàng bạn nhé, bạn cần tư vấn thêm gì cứ nhắn shop ạ!",
]


class FakeLLM:
    def __init__(self, reply: Any = "", *, delay: float = 0.0) -> None:
        self.reply = reply
        self.delay = delay
        self.inputs: list[Any] = []

    async def ainvoke(self, input: Any, **_kwargs: Any) -> AIMessage:
        self.inputs.append(input)
        if self.delay:
            await asyncio.sleep(self.delay)
        if isinstance(self.reply, Exception):
            raise self.reply
        return AIMessage(content=self.reply)

    def prompt_text(self) -> str:
        return json.dumps(self.inputs, ensure_ascii=False, default=str)


def _use_llm(monkeypatch: pytest.MonkeyPatch, llm: FakeLLM) -> None:
    monkeypatch.setattr(
        "app.modules.ai.llm.runtime.build_llm_instance",
        lambda settings, **kwargs: llm,
    )


def _llm_service(
    monkeypatch: pytest.MonkeyPatch,
    llm: FakeLLM,
    listing: FakeListing,
    **settings: Any,
):
    _use_llm(monkeypatch, llm)
    return build_copilot_service(
        build_test_settings(COPILOT_BACKEND="llm_router", **settings), listing
    )


async def _suggest(service, message: str = STOCK_Q, seller_id: str = SELLER):
    return await service.suggest(
        ChatCopilotRequest(
            buyer_message=message, seller_id=seller_id, listing_id=LISTING
        )
    )


async def test_mock_default_is_deterministic(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Mock is the default and is deterministic"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    _use_llm(monkeypatch, llm)
    service = build_copilot_service(build_test_settings(), FakeListing(_facts(stock=0)))

    first = await _suggest(service)
    second = await _suggest(service)

    assert first.quick_replies == second.quick_replies
    assert len(first.quick_replies) == 3
    assert first.model_version == "rules-v1"
    assert llm.inputs == []


def test_mock_backend_is_the_default_setting():
    """Scenario: Mock is the default and is deterministic (settings)"""
    settings = build_test_settings()

    assert settings.COPILOT_BACKEND == "mock"
    assert settings.COPILOT_TIMEOUT_SECONDS == 3.0


def test_unknown_backend_rejected():
    """Scenario: Unknown backend value is rejected at boot"""
    with pytest.raises(ValidationError, match="COPILOT_BACKEND"):
        build_test_settings(COPILOT_BACKEND="gpt")


def test_llm_router_without_extra_fails_at_boot(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Unknown backend value is rejected at boot (llm_router needs ai extra)"""
    real = importlib.util.find_spec
    monkeypatch.setattr(
        importlib.util,
        "find_spec",
        lambda name, *a, **k: None if name == "langchain_core" else real(name, *a, **k),
    )

    with pytest.raises(RuntimeError, match="ai"):
        build_copilot_service(
            build_test_settings(COPILOT_BACKEND="llm_router"),
            FakeListing(_facts(stock=1)),
        )


async def test_contradicting_llm_output_falls_back(monkeypatch: pytest.MonkeyPatch):
    """Scenario: LLM output that contradicts the facts is replaced"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    service = _llm_service(monkeypatch, llm, FakeListing(_facts(stock=0)))

    response = await _suggest(service)

    assert llm.inputs
    assert len(response.quick_replies) == 3
    assert not any(claims_availability(d) for d in response.quick_replies)
    assert response.quick_replies != LLM_DRAFTS
    assert response.model_version == "rules-v1"


async def test_llm_delivery_claim_falls_back(monkeypatch: pytest.MonkeyPatch):
    """Scenario: LLM output that contradicts the facts is replaced (delivery promise)"""
    drafts = [
        "Dạ shop còn hàng ạ, giao trong 2 ngày là bạn nhận được nhé!",
        LLM_DRAFTS[1],
        LLM_DRAFTS[2],
    ]
    llm = FakeLLM(json.dumps(drafts))
    service = _llm_service(monkeypatch, llm, FakeListing(_facts(stock=12)))

    response = await _suggest(service)

    assert llm.inputs
    assert not any(promises_delivery(d) for d in response.quick_replies)
    assert response.quick_replies != drafts
    assert response.model_version == "rules-v1"


@pytest.mark.parametrize("reply", ["not json at all", "[]", '{"a": 1}', '["one"]'])
async def test_unusable_llm_output_falls_back(
    monkeypatch: pytest.MonkeyPatch, reply: str
):
    """Scenario: LLM output that contradicts the facts is replaced (unusable output)"""
    service = _llm_service(monkeypatch, FakeLLM(reply), FakeListing(_facts(stock=12)))

    response = await _suggest(service)

    assert len(response.quick_replies) == 3
    assert response.model_version == "rules-v1"


async def test_llm_error_falls_back(monkeypatch: pytest.MonkeyPatch):
    """Scenario: LLM timeout falls back (provider error)"""
    service = _llm_service(
        monkeypatch, FakeLLM(RuntimeError("boom")), FakeListing(_facts(stock=12))
    )

    response = await _suggest(service)

    assert len(response.quick_replies) == 3
    assert response.model_version == "rules-v1"


async def test_llm_timeout_falls_back(monkeypatch: pytest.MonkeyPatch):
    """Scenario: LLM timeout falls back"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS), delay=30)
    service = _llm_service(
        monkeypatch, llm, FakeListing(_facts(stock=12)), COPILOT_TIMEOUT_SECONDS=0.2
    )

    started = time.monotonic()
    response = await _suggest(service)
    elapsed = time.monotonic() - started

    assert elapsed <= 0.2 + 0.2
    assert len(response.quick_replies) == 3
    assert response.quick_replies != LLM_DRAFTS
    assert response.model_version == "rules-v1"


async def test_prompt_has_no_ids_or_pii(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Prompt carries no ids"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    service = _llm_service(monkeypatch, llm, FakeListing(_facts(stock=12)))

    await _suggest(service, message=f"{STOCK_Q} gọi em {PHONE} nhé")

    prompt = llm.prompt_text()
    assert llm.inputs
    assert SELLER not in prompt
    assert LISTING not in prompt
    assert PHONE not in prompt


async def test_prompt_omits_other_sellers_listing(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Listing owned by another seller (nothing of it in the prompt)"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    facts = _facts(stock=5, seller_id=OTHER_SELLER)
    service = _llm_service(monkeypatch, llm, FakeListing(facts))

    response = await _suggest(service)

    prompt = llm.prompt_text()
    assert facts.title not in prompt
    assert OTHER_SELLER not in prompt
    assert not any(claims_availability(d) for d in response.quick_replies)


async def test_prompt_message_is_truncated_to_1000_chars(
    monkeypatch: pytest.MonkeyPatch,
):
    """Scenario: Prompt carries no ids (message truncated to 1000 characters)"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    service = _llm_service(monkeypatch, llm, FakeListing(_facts(stock=12)))

    await _suggest(service, message="zebra " * 300)

    count = llm.prompt_text().count("zebra")
    assert 100 <= count <= 167


async def test_llm_model_version_names_prompt(monkeypatch: pytest.MonkeyPatch):
    """Scenario: Copilot LLM output names the prompt version (backend side)"""
    llm = FakeLLM(json.dumps(LLM_DRAFTS))
    service = _llm_service(monkeypatch, llm, FakeListing(_facts(stock=12)))

    response = await _suggest(service)

    assert response.quick_replies == LLM_DRAFTS
    assert response.model_version == f"llm_router:{COPILOT_PROMPT_VERSION}"
