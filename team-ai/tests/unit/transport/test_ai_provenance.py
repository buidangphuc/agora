"""Provenance on ChatCopilot, MagicListing and SummarizeReviews responses."""

from __future__ import annotations

import dataclasses
import json
import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import pytest

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.copilot_llm import COPILOT_PROMPT_VERSION
from app.modules.business.ai_assistant.factory import (
    build_ai_assistant_services,
    build_copilot_service,
)
from app.modules.business.listing_screen.mock import MockScreenService
from app.modules.business.tag_classifier.service import TagClassifierService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.ai import ai_service_registration
from tests.factories import allow_all_limiter, build_test_settings
from tests.unit.modules.business.ai_assistant.test_copilot_backend import (
    LLM_DRAFTS,
    FakeLLM,
    _use_llm,
)
from tests.unit.modules.business.ai_assistant.test_copilot_grounding import (
    LISTING,
    SELLER,
    STOCK_Q,
    FakeListing,
    _facts,
)
from tests.unit.transport.test_ai_servicer import _fails, _md

_HEX32 = re.compile(r"^[0-9a-f]{32}$")


@asynccontextmanager
async def _serving_copilot(
    listing: FakeListing | None = None, copilot: ChatCopilotService | None = None
) -> AsyncIterator[ai_pb2_grpc.AIServiceStub]:
    services = build_ai_assistant_services()
    services = dataclasses.replace(
        services,
        copilot=copilot
        or ChatCopilotService(
            listing=listing, redaction=RedactionPolicy(mode="redacted")
        ),
    )
    server = build_grpc_server(
        registrations=(
            ai_service_registration(
                services,
                TagClassifierService(store=None),
                screen=MockScreenService("x"),
                require_ai_use=False,
            ),
        ),
        rate_limiter=allow_all_limiter(),
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            yield ai_pb2_grpc.AIServiceStub(channel)
    finally:
        await server.stop(None)


def _copilot_req(**kw) -> ai_pb2.ChatCopilotRequest:
    return ai_pb2.ChatCopilotRequest(buyer_message=STOCK_Q, seller_id=SELLER, **kw)


async def test_chat_copilot_has_provenance():
    """Scenario: Copilot response carries provenance"""
    async with _serving_copilot(FakeListing(None)) as stub:
        reply = await stub.ChatCopilot(
            _copilot_req(), metadata=_md(SELLER, "listing.write")
        )

    assert _HEX32.match(reply.request_id)
    assert reply.model_version == "rules-v1"


async def test_llm_model_version_names_prompt_via_servicer(
    monkeypatch: pytest.MonkeyPatch,
):
    """Scenario: Copilot LLM output names the prompt version"""
    _use_llm(monkeypatch, FakeLLM(json.dumps(LLM_DRAFTS)))
    listing = FakeListing(_facts(stock=5))
    service = build_copilot_service(
        build_test_settings(COPILOT_BACKEND="llm_router"), listing
    )
    async with _serving_copilot(copilot=service) as stub:
        reply = await stub.ChatCopilot(
            _copilot_req(listing_id=LISTING), metadata=_md(SELLER, "listing.write")
        )

    assert reply.model_version == f"llm_router:{COPILOT_PROMPT_VERSION}"
    assert _HEX32.match(reply.request_id)


async def test_magic_listing_has_provenance():
    """Scenario: Magic Listing response carries provenance"""
    async with _serving_copilot() as stub:
        reply = await stub.MagicListing(
            ai_pb2.MagicListingRequest(title_hint="Tai nghe", category_hint="audio"),
            metadata=_md(SELLER, "listing.write"),
        )

    assert _HEX32.match(reply.request_id)
    assert reply.model_version == "rules-v1"
    assert reply.generated_title


async def test_summarize_reviews_has_provenance():
    """Scenario: Review summary carries provenance"""
    async with _serving_copilot() as stub:
        reply = await stub.SummarizeReviews(
            ai_pb2.SummarizeReviewsRequest(
                listing_id="l1",
                reviews=[
                    ai_pb2.ReviewInput(rating=5, comment="rất tốt"),
                    ai_pb2.ReviewInput(rating=4, comment="chất lượng ổn"),
                    ai_pb2.ReviewInput(rating=5, comment="giao nhanh"),
                ],
            ),
            metadata=_md("u1", "listing.read"),
        )

    assert _HEX32.match(reply.request_id)
    assert reply.model_version == "rules-v1"


async def test_request_id_unique_per_call():
    """Scenario: Copilot request ids are unique per call"""
    async with _serving_copilot(FakeListing(None)) as stub:
        first = await stub.ChatCopilot(
            _copilot_req(), metadata=_md(SELLER, "listing.write")
        )
        second = await stub.ChatCopilot(
            _copilot_req(), metadata=_md(SELLER, "listing.write")
        )

    assert _HEX32.match(first.request_id)
    assert _HEX32.match(second.request_id)
    assert first.request_id != second.request_id


async def test_error_has_no_response_body():
    """Scenario: Failed call returns no provenance"""
    async with _serving_copilot(FakeListing(None)) as stub:
        call = stub.ChatCopilot(
            ai_pb2.ChatCopilotRequest(buyer_message="", seller_id=SELLER),
            metadata=_md(SELLER, "listing.write"),
        )
        error = await _fails(call)

    assert error.code() == grpc.StatusCode.INVALID_ARGUMENT
