"""Grounded ChatCopilot: drafts follow the listing stock and promise no delivery time.

Interfaces these tests pin (implementer creates them):
  adapters.listing.ListingFacts gains stock: int = 0 and seller_id: str = ""
  GrpcListingReader.get(...) fills stock (variant sum when variants exist) and seller_id
  copilot.ChatCopilotService(listing=ListingPort, redaction=, listing_timeout_s=0.2)
      async .suggest(request) -> ChatCopilotResponse; effective seller = request.seller_id
"""

import asyncio
import re
import time
import unicodedata
from typing import Any

import grpc
import pytest
from loguru import logger

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.adapters.errors import RetrievalError
from app.modules.business.ai_assistant.adapters.listing import (
    GrpcListingReader,
    ListingFacts,
)
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.schemas import ChatCopilotRequest
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2

REDACTION = RedactionPolicy(mode="redacted")
SELLER = "seller-4e8b1d90"
OTHER_SELLER = "seller-99aa00bb"
LISTING = "listing-5c7d2f31"
PHONE = "0912345678"
STOCK_Q = "còn hàng không?"
DELIVERY_Q = "giao hàng bao lâu vậy shop?"


def _norm(text: str) -> str:
    nfkd = unicodedata.normalize("NFKD", text.replace("đ", "d").replace("Đ", "D"))
    return "".join(c for c in nfkd if not unicodedata.combining(c)).lower()


_NEGATED_AVAILABILITY = (
    "het hang",
    "khong con hang",
    "khong con san",
    "khong co san",
    "chua co san",
    "chua con hang",
    "tam het",
    "khong con",
)
_AVAILABILITY = ("con hang", "con san", "co san", "con bang", "shop con ")


def claims_availability(draft: str) -> bool:
    text = _norm(draft)
    for neg in _NEGATED_AVAILABILITY:
        text = text.replace(neg, " ")
    return any(p in text for p in _AVAILABILITY)


def says_out_of_stock(draft: str) -> bool:
    text = _norm(draft)
    return any(p in text for p in ("het hang", "khong con hang", "tam het"))


_DELIVERY_PROMISE = re.compile(
    r"\d+\s*(?:-\s*\d+\s*)?(?:ngay|gio|tieng|h\b|phut)"
    r"|trong ngay|hoa toc|ngay hom nay|truoc \d+\s*h|ngay lap tuc|trong \d+"
)


def promises_delivery(draft: str) -> bool:
    return _DELIVERY_PROMISE.search(_norm(draft)) is not None


def _facts(
    *,
    stock: int,
    seller_id: str = SELLER,
    published: bool = True,
) -> ListingFacts:
    return ListingFacts(
        listing_id=LISTING,
        title="Tai nghe Model 1",
        price=350000,
        currency="VND",
        published=published,
        stock=stock,
        seller_id=seller_id,
    )


class FakeListing:
    def __init__(self, outcome: Any = None, *, hang: bool = False) -> None:
        self.outcome = outcome
        self.hang = hang
        self.calls: list[str] = []

    async def get(self, listing_id: str, *, timeout_s: float) -> ListingFacts | None:
        self.calls.append(listing_id)
        if self.hang:
            await asyncio.sleep(30)
        if isinstance(self.outcome, Exception):
            raise self.outcome
        return self.outcome


def _service(listing: FakeListing, **kwargs: Any) -> ChatCopilotService:
    return ChatCopilotService(listing=listing, redaction=REDACTION, **kwargs)


async def _ask(
    listing: FakeListing,
    message: str = STOCK_Q,
    *,
    listing_id: str = LISTING,
    seller_id: str = SELLER,
    **kwargs: Any,
) -> list[str]:
    response = await _service(listing, **kwargs).suggest(
        ChatCopilotRequest(
            buyer_message=message, seller_id=seller_id, listing_id=listing_id
        )
    )
    return response.quick_replies


async def test_out_of_stock_listing_claims_no_stock_and_says_so():
    """Scenario: Out-of-stock listing"""
    drafts = await _ask(FakeListing(_facts(stock=0)))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)
    assert any(says_out_of_stock(d) for d in drafts)


async def test_in_stock_listing_confirms_stock():
    """Scenario: In-stock listing"""
    drafts = await _ask(FakeListing(_facts(stock=12)))

    assert len(drafts) == 3
    assert all(claims_availability(d) for d in drafts)
    assert not any(says_out_of_stock(d) for d in drafts)
    for d in drafts:
        assert all(int(n) <= 12 for n in re.findall(r"\d+", d))


async def test_unknown_listing_makes_no_stock_claim():
    """Scenario: Copilot stock question for an unknown listing"""
    drafts = await _ask(FakeListing(None))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_lookup_timeout_degrades_without_claim():
    """Scenario: Listing lookup fails"""
    started = time.monotonic()
    drafts = await _ask(FakeListing(hang=True), listing_timeout_s=0.05)
    elapsed = time.monotonic() - started

    assert elapsed < 0.5
    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_lookup_timeout_error_degrades_without_claim():
    """Scenario: Listing lookup fails"""
    drafts = await _ask(FakeListing(TimeoutError("deadline")))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_lookup_error_degrades_without_claim():
    """Scenario: Listing lookup fails"""
    drafts = await _ask(FakeListing(RetrievalError("listing lookup rejected")))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_empty_listing_id_skips_lookup():
    """Scenario: No listing id given"""
    listing = FakeListing(_facts(stock=12))

    drafts = await _ask(listing, listing_id="")

    assert listing.calls == []
    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


@pytest.mark.parametrize("seller_id", [SELLER, ""])
async def test_lookup_uses_the_requested_listing_id(seller_id: str):
    """Scenario: In-stock listing (lookup is made for the discussed listing)"""
    listing = FakeListing(_facts(stock=3))

    await _ask(listing, seller_id=seller_id)

    assert listing.calls == [LISTING]


async def test_unpublished_listing_makes_no_stock_claim():
    """Scenario: Unpublished listing"""
    drafts = await _ask(FakeListing(_facts(stock=9, published=False)))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_other_sellers_listing_makes_no_stock_claim_and_leaks_nothing():
    """Scenario: Listing owned by another seller"""
    drafts = await _ask(FakeListing(_facts(stock=5, seller_id=OTHER_SELLER)))

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)
    joined = " ".join(drafts)
    assert OTHER_SELLER not in joined
    assert "Tai nghe Model 1" not in joined
    assert "350000" not in joined


async def test_stock_of_one_is_in_stock():
    """Scenario: Stock boundary of one"""
    drafts = await _ask(FakeListing(_facts(stock=1)))

    assert len(drafts) == 3
    assert all(claims_availability(d) for d in drafts)
    assert not any(says_out_of_stock(d) for d in drafts)


async def test_default_drafts_have_no_availability_claim():
    """Scenario: Default drafts carry no availability claim"""
    drafts = await _ask(FakeListing(_facts(stock=0)), "shop ơi")

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_default_drafts_without_listing_have_no_availability_claim():
    """Scenario: Default drafts carry no availability claim"""
    drafts = await _ask(FakeListing(None), "shop ơi", listing_id="")

    assert len(drafts) == 3
    assert not any(claims_availability(d) for d in drafts)


async def test_delivery_question_makes_no_promise_and_asks_for_address():
    """Scenario: Delivery-time question"""
    drafts = await _ask(FakeListing(_facts(stock=12)), DELIVERY_Q)

    assert len(drafts) == 3
    assert not any(promises_delivery(d) for d in drafts)
    assert any("dia chi" in _norm(d) for d in drafts)


@pytest.mark.parametrize(
    "message",
    [
        "ship mấy ngày vậy",
        "khi nào nhận được hàng",
        "có giao hỏa tốc không",
        "còn hàng không, giao hàng bao lâu?",
    ],
)
async def test_delivery_variants_make_no_promise(message: str):
    """Scenario: Delivery-time question"""
    drafts = await _ask(FakeListing(_facts(stock=12)), message)

    assert len(drafts) == 3
    assert not any(promises_delivery(d) for d in drafts)


async def test_delivery_question_without_lookup_has_no_duration():
    """Scenario: Delivery question with a failing lookup"""
    drafts = await _ask(
        FakeListing(RetrievalError("UNAVAILABLE")),
        DELIVERY_Q,
    )

    assert len(drafts) == 3
    assert not any(promises_delivery(d) for d in drafts)


@pytest.mark.parametrize(
    "message",
    [
        STOCK_Q,
        DELIVERY_Q,
        "có giảm giá không shop",
        "hàng chính hãng không",
        "tư vấn size giúp mình",
        "shop ơi",
    ],
)
@pytest.mark.parametrize("stock", [0, 7])
async def test_every_intent_returns_three_short_drafts_without_promises(
    message: str, stock: int
):
    """Scenario: Default drafts carry no availability claim (draft shape limits)"""
    drafts = await _ask(FakeListing(_facts(stock=stock)), message)

    assert len(drafts) == 3
    assert all(0 < len(d) <= 300 for d in drafts)
    assert not any(promises_delivery(d) for d in drafts)
    assert not any("luon co san" in _norm(d) for d in drafts)


async def test_very_long_message_still_returns_three_drafts():
    """Scenario: Default drafts carry no availability claim (long input)"""
    drafts = await _ask(FakeListing(_facts(stock=2)), "còn hàng không? " * 400)

    assert len(drafts) == 3
    assert all(len(d) <= 300 for d in drafts)


async def test_logs_carry_no_raw_ids():
    """Scenario: Copilot logs are redacted"""
    lines: list[str] = []
    sink_id = logger.add(lambda m: lines.append(str(m)), level="DEBUG")
    try:
        await _ask(
            FakeListing(
                RetrievalError(f"lookup {LISTING} for {SELLER} failed {PHONE}")
            ),
            f"còn hàng không? gọi {PHONE}",
        )
    finally:
        logger.remove(sink_id)

    output = "\n".join(lines)
    assert output
    assert PHONE not in output
    assert SELLER not in output
    assert LISTING not in output


class _RpcError(grpc.aio.AioRpcError):
    def __init__(self, code: grpc.StatusCode) -> None:
        super().__init__(code, grpc.aio.Metadata(), grpc.aio.Metadata())


class _Channel:
    def __init__(self, reply: Any) -> None:
        self.reply = reply
        self.calls: list[tuple[Any, float | None]] = []

    def unary_unary(self, path: str, *args: Any, **kwargs: Any):
        async def call(request, timeout=None, metadata=None, **_: Any):
            self.calls.append((request, timeout))
            if isinstance(self.reply, Exception):
                raise self.reply
            return self.reply

        return call


def _reader(channel: _Channel) -> GrpcListingReader:
    return GrpcListingReader(
        channel,  # type: ignore[arg-type]
        principal_id="team-ai",
        principal_scopes=["listing.read"],
    )


def _reply(**fields: Any) -> listing_pb2.GetListingResponse:
    base = {
        "id": LISTING,
        "title": "Tai nghe",
        "price": 350000,
        "currency": "VND",
        "status": listing_pb2.LISTING_STATUS_PUBLISHED,
        "seller_id": SELLER,
    }
    base.update(fields)
    return listing_pb2.GetListingResponse(listing=listing_pb2.Listing(**base))


async def test_reader_returns_plain_stock_and_seller():
    """Scenario: In-stock listing (adapter facts)"""
    facts = await _reader(_Channel(_reply(stock=12))).get(LISTING, timeout_s=0.2)

    assert facts is not None
    assert facts.stock == 12
    assert facts.seller_id == SELLER
    assert facts.published is True


async def test_reader_sums_variant_stock_when_variants_exist():
    """Scenario: In-stock listing (variant sum stock rule)"""
    variants = [
        listing_pb2.Variant(id="v1", stock=0),
        listing_pb2.Variant(id="v2", stock=4),
        listing_pb2.Variant(id="v3", stock=3),
    ]

    facts = await _reader(_Channel(_reply(stock=99, variants=variants))).get(
        LISTING, timeout_s=0.2
    )

    assert facts is not None
    assert facts.stock == 7


async def test_reader_all_variants_empty_is_stock_zero():
    """Scenario: Out-of-stock listing (variant sum stock rule)"""
    variants = [
        listing_pb2.Variant(id="v1", stock=0),
        listing_pb2.Variant(id="v2", stock=0),
    ]

    facts = await _reader(_Channel(_reply(stock=5, variants=variants))).get(
        LISTING, timeout_s=0.2
    )

    assert facts is not None
    assert facts.stock == 0


async def test_reader_marks_draft_listing_unpublished():
    """Scenario: Unpublished listing"""
    facts = await _reader(
        _Channel(_reply(stock=3, status=listing_pb2.LISTING_STATUS_DRAFT))
    ).get(LISTING, timeout_s=0.2)

    assert facts is not None
    assert facts.published is False


async def test_reader_not_found_returns_none():
    """Scenario: Copilot stock question for an unknown listing"""
    reader = _reader(_Channel(_RpcError(grpc.StatusCode.NOT_FOUND)))

    assert await reader.get(LISTING, timeout_s=0.2) is None


async def test_reader_deadline_raises_timeout_error():
    """Scenario: Listing lookup fails"""
    reader = _reader(_Channel(_RpcError(grpc.StatusCode.DEADLINE_EXCEEDED)))

    with pytest.raises(TimeoutError):
        await reader.get(LISTING, timeout_s=0.2)


async def test_reader_unavailable_raises_retrieval_error():
    """Scenario: Delivery question with a failing lookup"""
    reader = _reader(_Channel(_RpcError(grpc.StatusCode.UNAVAILABLE)))

    with pytest.raises(RetrievalError):
        await reader.get(LISTING, timeout_s=0.2)
