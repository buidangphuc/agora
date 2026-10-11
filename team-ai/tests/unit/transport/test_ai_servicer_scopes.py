"""AIService scope gates: who may call which RPC (design D7).

Drives the servicer with a fake context and a bound Principal, so no network or
interceptor is involved. ``ai:use`` is not granted by team-identity yet, so
ShoppingAssistant is open unless ``require_ai_use`` (AI_USE_SCOPE_REQUIRED) is on;
the seller / public-read gates are always on.
"""

from __future__ import annotations

import grpc
import pytest

from app.modules.business.ai_assistant.service import AIAssistantService
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.scopes import AI_SERVICE_SCOPES, AI_USE_SCOPE
from app.transport.grpc.servicers.ai import AIServicer


class _Aborted(Exception):
    def __init__(self, code: grpc.StatusCode, details: str) -> None:
        super().__init__(details)
        self.code = code
        self.details = details


class _FakeContext:
    async def abort(self, code: grpc.StatusCode, details: str = ""):
        raise _Aborted(code, details)


ANONYMOUS = Principal(
    id="anonymous", type="anonymous", scopes=("listing.read", "search:read")
)
BUYER = Principal(
    id="buyer-1", type="user", scopes=("listing.read", "search:read", AI_USE_SCOPE)
)
BUYER_OLD_TOKEN = Principal(  # token issued before identity granted ai:use
    id="buyer-2", type="user", scopes=("listing.read", "search:read")
)
SELLER = Principal(
    id="seller-1",
    type="user",
    scopes=("listing.read", "listing.write", "search:read", AI_USE_SCOPE),
)
ADMIN = Principal(
    id="admin-1",
    type="user",
    scopes=("admin", "listing.read", "listing.write", "search:read", AI_USE_SCOPE),
)


class _CountingProvider:
    """AIAssistantService provider that records every time the assistant is reached."""

    def __init__(self) -> None:
        self.calls = 0

    def __call__(self) -> AIAssistantService:
        self.calls += 1
        return AIAssistantService()


def _requests():
    return {
        "MagicListing": ai_pb2.MagicListingRequest(title_hint="ao thun"),
        "ChatCopilot": ai_pb2.ChatCopilotRequest(buyer_message="con hang khong?"),
        "ShoppingAssistant": ai_pb2.ShoppingAssistantRequest(message="ao thun"),
        "ClassifyTags": ai_pb2.ClassifyTagsRequest(title="Tai nghe Bluetooth 5.3"),
        "SummarizeReviews": ai_pb2.SummarizeReviewsRequest(
            listing_id="l1", reviews=[ai_pb2.ReviewInput(rating=5, comment="tot")]
        ),
    }


# RPCs each principal may call with the ai:use gate OFF (default today).
# ClassifyTags is internal (service principals only): no user role may call it.
ALL_RPCS = (set(AI_SERVICE_SCOPES) - {"ClassifyTags"}) | {"ShoppingAssistant"}
ALLOWED: dict[str, tuple[Principal, set[str]]] = {
    "anonymous": (ANONYMOUS, {"SummarizeReviews", "ShoppingAssistant"}),
    "buyer": (BUYER, {"ShoppingAssistant", "SummarizeReviews"}),
    "buyer_without_ai_use": (
        BUYER_OLD_TOKEN,
        {"SummarizeReviews", "ShoppingAssistant"},
    ),
    "seller": (SELLER, ALL_RPCS),
    "admin": (ADMIN, ALL_RPCS),
}
# Same, with the ai:use gate ON (AI_USE_SCOPE_REQUIRED=true).
ALLOWED_ENFORCED = {
    **ALLOWED,
    "anonymous": (ANONYMOUS, {"SummarizeReviews"}),
    "buyer_without_ai_use": (BUYER_OLD_TOKEN, {"SummarizeReviews"}),
}


async def _call(servicer: AIServicer, rpc: str, principal: Principal):
    token = bind_principal(principal)
    try:
        return await getattr(servicer, rpc)(_requests()[rpc], _FakeContext())
    finally:
        reset_principal(token)


@pytest.mark.parametrize("enforced", [False, True])
@pytest.mark.parametrize("who", ALLOWED)
@pytest.mark.parametrize("rpc", sorted(ALL_RPCS))
async def test_scope_matrix(who: str, rpc: str, enforced: bool):
    principal, allowed = (ALLOWED_ENFORCED if enforced else ALLOWED)[who]
    provider = _CountingProvider()
    servicer = AIServicer(provider, require_ai_use=enforced)

    if rpc in allowed:
        assert await _call(servicer, rpc, principal) is not None
        assert provider.calls == 1
    else:
        with pytest.raises(_Aborted) as info:
            await _call(servicer, rpc, principal)
        assert info.value.code is grpc.StatusCode.PERMISSION_DENIED
        assert "insufficient_scope" in info.value.details
        assert provider.calls == 0, "assistant logic must not run for a denied caller"


@pytest.mark.parametrize("who", ALLOWED)
async def test_classify_tags_is_denied_to_every_user_role(who: str):
    principal, _ = ALLOWED[who]
    with pytest.raises(_Aborted) as info:
        await _call(AIServicer(_CountingProvider()), "ClassifyTags", principal)
    assert info.value.code is grpc.StatusCode.PERMISSION_DENIED


async def test_classify_tags_needs_a_service_principal_with_the_scope():
    svc = Principal(id="service-team-search", type="service", scopes=("ai.classify",))
    res = await _call(AIServicer(_CountingProvider()), "ClassifyTags", svc)
    assert {t.facet_group for t in res.tags} >= {"connectivity"}
    # the scope on a USER principal is not enough, and a service without it is denied
    user_with_scope = Principal(id="u", type="user", scopes=("ai.classify",))
    no_scope = Principal(id="service-x", type="service", scopes=("listing.read",))
    for p in (user_with_scope, no_scope):
        with pytest.raises(_Aborted) as info:
            await _call(AIServicer(_CountingProvider()), "ClassifyTags", p)
        assert info.value.code is grpc.StatusCode.PERMISSION_DENIED


async def test_classify_tags_with_variants_returns_per_sku_tags_in_order():
    svc = Principal(id="service-team-search", type="service", scopes=("ai.classify",))
    req = ai_pb2.ClassifyTagsRequest(
        title="iPhone 15 Pro Max Titanium",
        variants=[
            ai_pb2.ClassifyVariant(
                variant_id="a", name="Titan Tự Nhiên / 256GB", stock=3
            ),
            ai_pb2.ClassifyVariant(variant_id="b", name="Xanh Navy / 512GB"),
        ],
    )
    token = bind_principal(svc)
    try:
        res = await AIServicer(_CountingProvider()).ClassifyTags(req, _FakeContext())
    finally:
        reset_principal(token)
    assert [s.variant_id for s in res.skus] == ["a", "b"]
    by_group = [{t.facet_group: t.slug for t in s.tags} for s in res.skus]
    assert by_group == [
        {"color": "titan-tu-nhien", "capacity": "256gb"},
        {"color": "xanh-navy", "capacity": "512gb"},
    ]
    assert all(0 < t.confidence <= 1 for s in res.skus for t in s.tags)


async def test_classify_tags_rejects_a_missing_title():
    svc = Principal(id="service-team-search", type="service", scopes=("ai.classify",))
    token = bind_principal(svc)
    try:
        with pytest.raises(_Aborted) as info:
            await AIServicer(_CountingProvider()).ClassifyTags(
                ai_pb2.ClassifyTagsRequest(title=" "), _FakeContext()
            )
    finally:
        reset_principal(token)
    assert info.value.code is grpc.StatusCode.INVALID_ARGUMENT


async def test_no_principal_is_unauthenticated():
    servicer = AIServicer(_CountingProvider())
    with pytest.raises(_Aborted) as info:
        await servicer.MagicListing(_requests()["MagicListing"], _FakeContext())
    assert info.value.code is grpc.StatusCode.UNAUTHENTICATED


async def test_seller_cannot_act_as_another_seller():
    provider = _CountingProvider()
    servicer = AIServicer(provider)
    other = ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="seller-2")
    own = ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="seller-1")

    token = bind_principal(SELLER)
    try:
        with pytest.raises(_Aborted) as info:
            await servicer.ChatCopilot(other, _FakeContext())
        assert info.value.code is grpc.StatusCode.PERMISSION_DENIED
        assert provider.calls == 0
        assert await servicer.ChatCopilot(own, _FakeContext()) is not None
    finally:
        reset_principal(token)


async def test_admin_may_act_as_another_seller():
    servicer = AIServicer(_CountingProvider())
    request = ai_pb2.ChatCopilotRequest(buyer_message="hi", seller_id="seller-2")
    token = bind_principal(ADMIN)
    try:
        assert await servicer.ChatCopilot(request, _FakeContext()) is not None
    finally:
        reset_principal(token)


def test_every_aiservice_rpc_has_a_scope_entry():
    """A future RPC cannot ship ungated: the table must cover the descriptor."""
    methods = {m.name for m in ai_pb2.DESCRIPTOR.services_by_name["AIService"].methods}
    gated = set(AI_SERVICE_SCOPES) | {"ShoppingAssistant"}  # see ai_use_scopes
    assert methods == gated, (
        f"AIService RPCs without a scope entry: {sorted(methods - gated)}; "
        f"stale entries: {sorted(gated - methods)}"
    )
    assert all(AI_SERVICE_SCOPES[m] for m in AI_SERVICE_SCOPES), "empty scope tuple"


def test_anonymous_public_scopes_never_satisfy_an_llm_rpc():
    public = set(ANONYMOUS.scopes)
    for rpc in ("MagicListing", "ChatCopilot"):
        assert not set(AI_SERVICE_SCOPES[rpc]) <= public, rpc
    assert AI_USE_SCOPE not in public
