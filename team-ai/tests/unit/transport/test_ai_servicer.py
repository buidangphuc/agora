"""AIService servicer: scopes, seller rule, proto mapping and error mapping, over a real channel."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import pytest

from app.modules.business.ai_assistant.factory import build_ai_assistant_services
from app.modules.business.tag_classifier.service import TagClassifierService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.ai import ai_service_registration
from tests.factories import allow_all_limiter


def _md(principal_id: str = "u1", *scopes: str, ptype: str = "user"):
    return (
        ("x-principal-id", principal_id),
        ("x-principal-type", ptype),
        ("x-principal-scopes", ",".join(scopes)),
    )


@asynccontextmanager
async def _serving(
    *, require_ai_use: bool = False
) -> AsyncIterator[ai_pb2_grpc.AIServiceStub]:
    services = build_ai_assistant_services(None, rag_min_score=0.0)
    server = build_grpc_server(
        registrations=(
            ai_service_registration(
                services,
                TagClassifierService(store=None),
                require_ai_use=require_ai_use,
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


async def _fails(call) -> grpc.aio.AioRpcError:
    with pytest.raises(grpc.aio.AioRpcError) as exc:
        await call
    return exc.value


def test_only_shopping_assistant_is_rate_limited():
    registration = ai_service_registration(
        build_ai_assistant_services(None, rag_min_score=0.0),
        TagClassifierService(store=None),
        require_ai_use=False,
    )

    assert registration.service_name == "platform.ai.v1.AIService"
    assert registration.rate_limited_methods == frozenset(
        {"/platform.ai.v1.AIService/ShoppingAssistant"}
    )


async def test_shopping_assistant_maps_cards_and_followups():
    async with _serving() as stub:
        reply = await stub.ShoppingAssistant(
            ai_pb2.ShoppingAssistantRequest(
                message="tai nghe bluetooth", user_id="b1", previous_context=["hi"]
            ),
            metadata=_md("b1"),
        )

    assert reply.reply_text
    assert 1 <= len(reply.product_cards) <= 4
    card = reply.product_cards[0]
    assert card.listing_id and card.title and card.price > 0
    assert card.currency == "VND"
    assert list(reply.suggested_followups)


async def test_shopping_assistant_requires_ai_use_when_flag_on():
    async with _serving(require_ai_use=True) as stub:
        request = ai_pb2.ShoppingAssistantRequest(message="tai nghe")
        error = await _fails(stub.ShoppingAssistant(request, metadata=_md("b1")))
        assert error.code() == grpc.StatusCode.PERMISSION_DENIED
        assert error.details() == "insufficient_scope: missing ['ai:use']"

        reply = await stub.ShoppingAssistant(request, metadata=_md("b1", "ai:use"))
        assert reply.reply_text


async def test_magic_listing_maps_fields():
    async with _serving() as stub:
        reply = await stub.MagicListing(
            ai_pb2.MagicListingRequest(title_hint="áo thun nam"),
            metadata=_md("s1", "listing.write"),
        )

    assert reply.generated_title
    assert reply.generated_description
    assert reply.suggested_category_id
    assert reply.suggested_price_max >= reply.suggested_price_min > 0
    assert list(reply.highlight_tags)


async def test_magic_listing_missing_scope_is_permission_denied():
    async with _serving() as stub:
        error = await _fails(
            stub.MagicListing(
                ai_pb2.MagicListingRequest(title_hint="áo thun"),
                metadata=_md("s1", "listing.read"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert error.details() == "insufficient_scope: missing ['listing.write']"


async def test_magic_listing_short_title_is_invalid_argument_naming_field_only():
    async with _serving() as stub:
        error = await _fails(
            stub.MagicListing(
                ai_pb2.MagicListingRequest(title_hint="a"),
                metadata=_md("s1", "listing.write"),
            )
        )

    assert error.code() == grpc.StatusCode.INVALID_ARGUMENT
    assert error.details() == "invalid request: title_hint"


async def test_chat_copilot_maps_quick_replies():
    async with _serving() as stub:
        reply = await stub.ChatCopilot(
            ai_pb2.ChatCopilotRequest(buyer_message="còn hàng không shop?"),
            metadata=_md("s1", "listing.write"),
        )

    assert list(reply.quick_replies)


async def test_chat_copilot_rejects_foreign_seller_id():
    async with _serving() as stub:
        error = await _fails(
            stub.ChatCopilot(
                ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="other"),
                metadata=_md("s1", "listing.write"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert error.details() == "insufficient_scope: seller_id does not match the caller"


async def test_chat_copilot_allows_own_seller_id():
    async with _serving() as stub:
        reply = await stub.ChatCopilot(
            ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="s1"),
            metadata=_md("s1", "listing.write"),
        )

    assert list(reply.quick_replies)


async def test_chat_copilot_admin_may_use_any_seller_id():
    async with _serving() as stub:
        reply = await stub.ChatCopilot(
            ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="other"),
            metadata=_md("a1", "listing.write", "admin"),
        )

    assert list(reply.quick_replies)


async def test_summarize_reviews_maps_fields():
    async with _serving() as stub:
        reply = await stub.SummarizeReviews(
            ai_pb2.SummarizeReviewsRequest(
                listing_id="l1",
                reviews=[
                    ai_pb2.ReviewInput(rating=5, comment="rất tốt, giao nhanh"),
                    ai_pb2.ReviewInput(rating=4, comment="chất lượng ổn"),
                ],
            ),
            metadata=_md("u1", "listing.read"),
        )

    assert reply.summary
    assert reply.sentiment == "positive"


async def test_summarize_reviews_requires_listing_read():
    async with _serving() as stub:
        error = await _fails(
            stub.SummarizeReviews(
                ai_pb2.SummarizeReviewsRequest(listing_id="l1"),
                metadata=_md("u1", "listing.write"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert error.details() == "insufficient_scope: missing ['listing.read']"


def _service_md(*scopes: str):
    return _md("svc", *scopes, ptype="service")


async def test_classify_tags_without_variants_returns_spu_tags():
    async with _serving() as stub:
        reply = await stub.ClassifyTags(
            ai_pb2.ClassifyTagsRequest(
                title="Tai nghe Bluetooth 5.3 chống ồn chủ động, sạc nhanh 65W"
            ),
            metadata=_service_md("ai.classify"),
        )

    by_slug = {tag.slug: tag for tag in reply.tags}
    assert {"bluetooth-5-3", "chong-on-chu-dong-anc"} <= set(by_slug)
    tag = by_slug["bluetooth-5-3"]
    assert (tag.name, tag.facet_group) == ("Bluetooth 5.3", "connectivity")
    assert 0 < tag.confidence <= 1
    assert list(reply.skus) == []


async def test_classify_tags_with_variants_returns_skus_in_request_order():
    async with _serving() as stub:
        reply = await stub.ClassifyTags(
            ai_pb2.ClassifyTagsRequest(
                title="Điện thoại Titan",
                variants=[
                    ai_pb2.ClassifyVariant(variant_id="v2", name="Xanh Titan / 512GB"),
                    ai_pb2.ClassifyVariant(variant_id="v1", name="Đen / 256GB"),
                ],
            ),
            metadata=_service_md("ai.classify"),
        )

    assert [sku.variant_id for sku in reply.skus] == ["v2", "v1"]
    assert {t.slug for t in reply.skus[0].tags} >= {"512gb"}


async def test_classify_tags_accepts_unnamed_and_negative_variants():
    async with _serving() as stub:
        reply = await stub.ClassifyTags(
            ai_pb2.ClassifyTagsRequest(
                title="Điện thoại Titan",
                variants=[
                    ai_pb2.ClassifyVariant(sku_code="X-512GB", price=-1, stock=-5)
                ],
            ),
            metadata=_service_md("ai.classify"),
        )

    assert len(reply.skus) == 1


async def test_classify_tags_user_principal_is_refused():
    async with _serving() as stub:
        error = await _fails(
            stub.ClassifyTags(
                ai_pb2.ClassifyTagsRequest(title="tai nghe"),
                metadata=_md("u1", "ai.classify", ptype="user"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert (
        error.details()
        == "insufficient_scope: ClassifyTags is for service principals only"
    )


async def test_classify_tags_requires_the_scope():
    async with _serving() as stub:
        error = await _fails(
            stub.ClassifyTags(
                ai_pb2.ClassifyTagsRequest(title="tai nghe"),
                metadata=_service_md("listing.read"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED
    assert error.details() == "insufficient_scope: missing ['ai.classify']"


@pytest.mark.parametrize("title", ["", " a ", "x"])
async def test_classify_tags_short_title_is_invalid_argument(title):
    async with _serving() as stub:
        error = await _fails(
            stub.ClassifyTags(
                ai_pb2.ClassifyTagsRequest(title=title),
                metadata=_service_md("ai.classify"),
            )
        )

    assert error.code() == grpc.StatusCode.INVALID_ARGUMENT
    assert error.details() == "invalid argument: title"
