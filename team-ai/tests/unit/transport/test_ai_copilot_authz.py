"""ChatCopilot authz: seller impersonation and scope checks happen before any listing lookup."""

from __future__ import annotations

import grpc
import pytest

from tests.unit.transport.test_ai_servicer import _fails, _md
from tests.unit.transport.test_ai_provenance import _serving_copilot
from tests.unit.modules.business.ai_assistant.test_copilot_grounding import (
    LISTING,
    OTHER_SELLER,
    SELLER,
    STOCK_Q,
    FakeListing,
    _facts,
    says_out_of_stock,
)
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2


async def test_other_seller_id_denied_before_lookup():
    """Scenario: Seller impersonation is denied"""
    listing = FakeListing(_facts(stock=0, seller_id=OTHER_SELLER))
    async with _serving_copilot(listing) as stub:
        error = await _fails(
            stub.ChatCopilot(
                ai_pb2.ChatCopilotRequest(
                    buyer_message=STOCK_Q, seller_id=OTHER_SELLER, listing_id=LISTING
                ),
                metadata=_md(SELLER, "listing.write"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert listing.calls == []


async def test_buyer_scope_denied():
    """Scenario: Caller without the seller scope is refused"""
    listing = FakeListing(_facts(stock=0))
    async with _serving_copilot(listing) as stub:
        error = await _fails(
            stub.ChatCopilot(
                ai_pb2.ChatCopilotRequest(
                    buyer_message=STOCK_Q, seller_id=SELLER, listing_id=LISTING
                ),
                metadata=_md("buyer-1", "listing.read"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert listing.calls == []


async def test_empty_seller_id_uses_caller_id_as_effective_seller():
    """Scenario: Seller impersonation is denied (effective seller is the caller)"""
    listing = FakeListing(_facts(stock=0, seller_id=SELLER))
    async with _serving_copilot(listing) as stub:
        reply = await stub.ChatCopilot(
            ai_pb2.ChatCopilotRequest(buyer_message=STOCK_Q, listing_id=LISTING),
            metadata=_md(SELLER, "listing.write"),
        )

    assert listing.calls == [LISTING]
    assert all(says_out_of_stock(d) for d in reply.quick_replies)
