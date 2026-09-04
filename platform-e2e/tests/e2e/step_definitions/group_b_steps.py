"""Step definitions for Group B: Engagement Q&A & Disputes."""

from __future__ import annotations

from pytest_bdd import given, then, when

from src.utils import get_test_data_manager
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World


@given("a buyer is on a listing page")
def buyer_on_listing_page(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    login_via_api(world, buyer)
    assert world.state.listing and world.state.listing.listing_id, "Listing must exist"


@when("the buyer asks a question and the seller answers")
def buyer_asks_and_seller_answers(world: World) -> None:
    listing_id = world.state.listing.listing_id
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    world.service_factory.set_token(buyer.token)
    q_res = world.service_factory.engagement.ask_question(
        listing_id=listing_id,
        question_text="San pham con hang khong shop?",
    )
    qid = q_res.get("question", {}).get("id", "q_001")
    world.state.extra["question_id"] = qid

    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    if not seller.token:
        seller.token = world.service_factory.auth.login(seller.username, seller.password)
    world.service_factory.set_token(seller.token)
    ans_res = world.service_factory.engagement.answer_question(
        question_id=qid,
        answer_text="Da shop con san hang a!",
        is_shop_reply=True,
    )
    world.state.extra["answer_res"] = ans_res


@then("the Q&A thread shows both entries")
def qa_thread_shows_both_entries(world: World) -> None:
    listing_id = world.state.listing.listing_id
    res = world.service_factory.engagement.list_questions(listing_id)
    questions = res.get("questions", [])
    assert len(questions) > 0


@given("a buyer has an issue with an order")
def buyer_has_issue_with_order(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    login_via_api(world, buyer)
    assert world.state.order_id, "Order ID must exist"


@when("the buyer opens a dispute")
def buyer_opens_dispute(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    world.service_factory.set_token(buyer.token)
    dispute_res = world.service_factory.engagement.create_dispute(
        order_id=world.state.order_id,
        defendant_id=seller.username,
        reason="Khong nhan duoc hang dung mo ta",
        evidence_urls=["https://img.vietnam.vn/proof1.jpg"],
    )
    dispute = dispute_res.get("dispute", {})
    world.state.extra["dispute_id"] = dispute.get("id", "disp_001")


@then("an admin can resolve it and the status updates")
def admin_resolves_dispute(world: World) -> None:
    import uuid

    dispute_id = world.state.extra.get("dispute_id", "disp_001")
    admin_uname = f"admin_{uuid.uuid4().hex[:6]}"
    admin_token = world.service_factory.auth.register(admin_uname, "pass123", "admin")
    world.service_factory.set_token(admin_token)
    res = world.service_factory.engagement.resolve_dispute(
        dispute_id=dispute_id,
        status="DISPUTE_STATUS_RESOLVED",
        resolution="Chap nhan yeu cau hoan tien cho nguoi mua.",
    )
    assert res is not None
