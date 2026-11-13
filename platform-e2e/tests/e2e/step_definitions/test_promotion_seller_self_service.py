"""Binds promo/promotion_seller_self_service.feature (change promotion-seller-self-service, initiative seller-centre).

Black box through the gateway (Connect JSON). Campaigns and vouchers are seeded with the public
create RPCs as the owning seller; the only direct SQL is fixture shaping (an expired voucher).
"""

from __future__ import annotations

import uuid
from typing import Any

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from src.utils import data as fake
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import audit_support as aud
from tests.e2e.support import voucher_scope_support as vs
from tests.e2e.support.world import World

scenarios("promo/promotion_seller_self_service.feature")

_SCHEDULED_WINDOW = {"starts_in_s": 86400 * 2, "ends_in_s": 86400 * 4}


def _x(world: World) -> dict:
    return world.state.extra.setdefault("sss", {})


def _mine(token: str, method: str, body: dict[str, Any] | None = None) -> httpx.Response:
    return ac.gw(token, method, body or {})


def _voucher_gw(token: str | None, method: str, body: dict[str, Any]) -> httpx.Response:
    return av.gw(token, method, body)


def _list_mine(token: str, statuses: list[str] | None = None) -> list[dict]:
    found: list[dict] = []
    cursor = ""
    for _ in range(30):
        body: dict[str, Any] = {"page": {"pageSize": 200, "cursor": cursor}}
        if statuses:
            body["statuses"] = statuses
        resp = _mine(token, "ListMyCampaigns", body)
        assert resp.status_code == 200, f"ListMyCampaigns {resp.status_code}: {resp.text}"
        found.extend(resp.json().get("campaigns") or [])
        cursor = (resp.json().get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found


def _list_vouchers(token: str) -> list[dict]:
    found: list[dict] = []
    cursor = ""
    for _ in range(30):
        resp = _voucher_gw(
            token, "ListMyShopVouchers", {"page": {"pageSize": 200, "cursor": cursor}}
        )
        assert resp.status_code == 200, f"ListMyShopVouchers {resp.status_code}: {resp.text}"
        found.extend(resp.json().get("vouchers") or [])
        cursor = (resp.json().get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found


def _by_id(items: list[dict], item_id: str) -> dict | None:
    return next((i for i in items if i.get("id") == item_id), None)


def _new_listing(world: World, seller: str) -> str:
    return ac.make_listing(world, seller)


def _ensure_admin(world: World) -> None:
    st = _x(world)
    if "admin" not in st:
        st["admin"] = aud.admin_token(world)
        st["admin_id"] = vs.token_sub(st["admin"])


# ── Givens ───────────────────────────────────────────────────────────────
@given("the self-service sellers A and B, each with a published listing")
def two_sellers(world: World) -> None:
    st = _x(world)
    for tag in ("a", "b"):
        token = world.service_factory.auth.register(
            fake.unique_username(f"sss_{tag}"), world.settings.seed_password, "seller"
        )
        st[f"seller_{tag}"] = token
        st[f"id_{tag}"] = vs.token_sub(token)
        st[f"listing_{tag}"] = ac.make_listing(world, token)


@given("the seeded admin for the self-service checks")
def seeded_admin(world: World) -> None:
    _ensure_admin(world)


@given("a self-service buyer")
def buyer(world: World) -> None:
    st = _x(world)
    st["buyer"] = world.service_factory.auth.register(
        fake.unique_username("sss_buyer"), world.settings.seed_password, "buyer"
    )
    st["buyer_id"] = vs.token_sub(st["buyer"])


@given("seller A has one scheduled and one active campaign and seller B has one campaign")
def campaigns_ab(world: World) -> None:
    st = _x(world)
    st["a_sched"] = ac.seller_campaign(st["seller_a"], st["listing_a"], **_SCHEDULED_WINDOW)
    st["a_active"] = ac.seller_campaign(
        st["seller_a"], _new_listing(world, st["seller_a"]), stock_cap=7
    )
    st["b_camp"] = ac.seller_campaign(st["seller_b"], st["listing_b"])


@given("seller A has a scheduled campaign for the self-service cancel")
def scheduled_a(world: World) -> None:
    st = _x(world)
    st["campaign"] = ac.seller_campaign(st["seller_a"], st["listing_a"], **_SCHEDULED_WINDOW)


@given("seller A has an active campaign for the self-service cancel")
def active_a(world: World) -> None:
    st = _x(world)
    st["campaign"] = ac.seller_campaign(st["seller_a"], st["listing_a"])


@given("team-audit is stopped for the self-service checks")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@given("seller A has an active, a deactivated and an expired shop voucher")
def vouchers_a(world: World) -> None:
    st = _x(world)
    _ensure_admin(world)
    codes = {k: av.fresh_code(f"SSS{k[0].upper()}") for k in ("active", "deactivated", "expired")}
    ids = {k: av.create_voucher(st["seller_a"], c, scope=av.SHOP)["id"] for k, c in codes.items()}
    resp = av.deactivate(st["admin"], ids["deactivated"])
    assert resp.status_code == 200, f"seeding a deactivated voucher failed: {resp.text}"
    av.backdate_end(codes["expired"])
    st["codes"], st["ids"] = codes, ids


@given("seller B has a shop voucher and a platform voucher exists")
def vouchers_b(world: World) -> None:
    st = _x(world)
    st["b_code"] = av.fresh_code("SSSB")
    st["b_voucher"] = av.create_voucher(st["seller_b"], st["b_code"], scope=av.SHOP)["id"]
    st["p_code"] = av.fresh_code("SSSP")
    st["p_voucher"] = av.create_voucher(st["admin"], st["p_code"], scope=av.PLATFORM)["id"]


def _preview(world: World, code: str) -> dict:
    st = _x(world)
    resp = _voucher_gw(
        st["buyer"],
        "ValidateAndReserve",
        {
            "reservationId": f"preview:{st['buyer_id']}:{uuid.uuid4().hex[:12]}",
            "code": code,
            "buyerId": st["buyer_id"],
            "cartSubtotal": "1000000",
            "sellerId": st["id_a"],
        },
    )
    assert resp.status_code == 200, f"preview: {resp.status_code} {resp.text}"
    return resp.json()


@given("the buyer can preview the active voucher of seller A")
def preview_control(world: World) -> None:
    body = _preview(world, _x(world)["codes"]["active"])
    assert body.get("valid"), f"positive control: the active voucher is not valid: {body}"


# ── Whens ────────────────────────────────────────────────────────────────
@when("seller A lists their own campaigns")
def a_lists(world: World) -> None:
    st = _x(world)
    st["listed"] = _list_mine(st["seller_a"])


@when(parsers.parse('seller A lists their own campaigns filtered by "{status}"'))
def a_lists_filtered(world: World, status: str) -> None:
    st = _x(world)
    st["listed"] = _list_mine(st["seller_a"], [status])


@when("the buyer lists their own campaigns and cancels a campaign of seller A")
def buyer_campaign_calls(world: World) -> None:
    st = _x(world)
    st["resps"] = [
        _mine(st["buyer"], "ListMyCampaigns", {"page": {"pageSize": 50}}),
        _mine(st["buyer"], "CancelMyCampaign", {"campaignId": st["a_sched"]}),
    ]


@when("seller A cancels that campaign")
def a_cancels(world: World) -> None:
    st = _x(world)
    st["resp"] = _mine(st["seller_a"], "CancelMyCampaign", {"campaignId": st["campaign"]})


@when("seller B cancels that campaign and then a campaign id that does not exist")
def b_cancels(world: World) -> None:
    st = _x(world)
    st["resps"] = [
        _mine(st["seller_b"], "CancelMyCampaign", {"campaignId": st["campaign"]}),
        _mine(st["seller_b"], "CancelMyCampaign", {"campaignId": str(uuid.uuid4())}),
    ]


@when("seller A lists their shop vouchers")
def a_lists_vouchers(world: World) -> None:
    st = _x(world)
    st["vlisted"] = _list_vouchers(st["seller_a"])


@when("the buyer lists shop vouchers and deactivates the active voucher of seller A")
def buyer_voucher_calls(world: World) -> None:
    st = _x(world)
    st["resps"] = [
        _voucher_gw(st["buyer"], "ListMyShopVouchers", {"page": {"pageSize": 50}}),
        _voucher_gw(st["buyer"], "DeactivateMyShopVoucher", {"voucherId": st["ids"]["active"]}),
    ]


@when("seller A deactivates their active shop voucher")
def a_deactivates(world: World) -> None:
    st = _x(world)
    st["resp"] = _voucher_gw(
        st["seller_a"], "DeactivateMyShopVoucher", {"voucherId": st["ids"]["active"]}
    )


@when(
    "seller B deactivates the active voucher of seller A and then a voucher id that does not exist"
)
def b_deactivates(world: World) -> None:
    st = _x(world)
    st["resps"] = [
        _voucher_gw(st["seller_b"], "DeactivateMyShopVoucher", {"voucherId": st["ids"]["active"]}),
        _voucher_gw(st["seller_b"], "DeactivateMyShopVoucher", {"voucherId": str(uuid.uuid4())}),
    ]


@when("seller A lists their own campaigns and then calls the admin campaign list")
def a_own_then_admin(world: World) -> None:
    st = _x(world)
    st["resps"] = [
        _mine(st["seller_a"], "ListMyCampaigns", {"page": {"pageSize": 50}}),
        ac.list_campaigns(st["seller_a"]),
    ]


# ── Thens: campaigns ─────────────────────────────────────────────────────
@then(
    "the list holds exactly A's scheduled and active campaigns with seller id A, their statuses and stock figures"
)
def exactly_a(world: World) -> None:
    st = _x(world)
    listed = st["listed"]
    assert {c["id"] for c in listed} == {st["a_sched"], st["a_active"]}, listed
    assert all(c.get("sellerId") == st["id_a"] for c in listed), listed
    sched, active = _by_id(listed, st["a_sched"]), _by_id(listed, st["a_active"])
    assert sched["status"] == ac.SCHEDULED and active["status"] == ac.ACTIVE, listed
    assert str(active.get("stockCap")) == "7" and str(active.get("stockSold") or "0") == "0", active
    assert str(sched.get("stockCap")) == "10", sched


@then("the list holds none of seller B's campaigns")
def none_of_b(world: World) -> None:
    st = _x(world)
    assert _by_id(st["listed"], st["b_camp"]) is None, st["listed"]


@then("the list holds only A's scheduled campaign")
def only_sched(world: World) -> None:
    st = _x(world)
    assert [c["id"] for c in st["listed"]] == [st["a_sched"]], st["listed"]
    assert st["listed"][0]["status"] == ac.SCHEDULED


@then("an unfiltered list of seller A still holds both campaigns")
def unfiltered_a(world: World) -> None:
    st = _x(world)
    assert {c["id"] for c in _list_mine(st["seller_a"])} == {st["a_sched"], st["a_active"]}


def _assert_all_refused(resps: list[httpx.Response], code: str) -> None:
    for resp in resps:
        assert av.code_of(resp) == code, f"want {code}: {resp.status_code} {resp.text}"


@then(
    parsers.parse(
        'the buyer\'s campaign calls are refused with "{code}" and carry no campaign data'
    )
)
def buyer_campaign_refused(world: World, code: str) -> None:
    resps = _x(world)["resps"]
    _assert_all_refused(resps, code)
    assert all(not (r.json().get("campaigns") or r.json().get("campaign")) for r in resps), [
        r.text for r in resps
    ]


@then("seller A can still list their two campaigns")
def a_still_lists(world: World) -> None:
    st = _x(world)
    assert {c["id"] for c in _list_mine(st["seller_a"])} == {st["a_sched"], st["a_active"]}


@then(parsers.parse('the cancel response shows status "{status}" and a cancelled_at'))
def cancel_shows(world: World, status: str) -> None:
    campaign = ac.campaign_of(_x(world)["resp"])
    assert campaign["status"] == status, campaign
    assert campaign.get("cancelledAt"), campaign


@then(parsers.parse('seller A\'s list shows the campaign "{status}"'))
def list_shows(world: World, status: str) -> None:
    st = _x(world)
    mine = _by_id(_list_mine(st["seller_a"]), st["campaign"])
    assert mine and mine["status"] == status, mine


@then("the listing of the campaign has no active flash sale")
def no_active_sale(world: World) -> None:
    st = _x(world)
    resp = ac.gw(st["seller_a"], "GetActiveFlashSale", {"listingId": st["listing_a"]})
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert not resp.json().get("active"), f"flash price still active: {resp.text}"


@then(parsers.parse('the self-service call is refused with "{code}"'))
def call_refused(world: World, code: str) -> None:
    _assert_all_refused([_x(world)["resp"]], code)


@then(parsers.parse('the campaign stays "{status}" with no cancelled_at'))
def stays(world: World, status: str) -> None:
    st = _x(world)
    mine = _by_id(_list_mine(st["seller_a"]), st["campaign"])
    assert mine and mine["status"] == status and not mine.get("cancelledAt"), mine


@then("seller A can still cancel a scheduled campaign")
def a_can_cancel_other(world: World) -> None:
    st = _x(world)
    other = ac.seller_campaign(
        st["seller_a"], _new_listing(world, st["seller_a"]), **_SCHEDULED_WINDOW
    )
    resp = _mine(st["seller_a"], "CancelMyCampaign", {"campaignId": other})
    assert ac.campaign_of(resp)["status"] == ac.CANCELLED


@then(parsers.parse('both calls are refused with "{code}" and the same message'))
def same_message(world: World, code: str) -> None:
    resps = _x(world)["resps"]
    _assert_all_refused(resps, code)
    messages = {av.message_of(r) for r in resps}
    assert len(messages) == 1 and messages != {""}, f"messages differ or are empty: {messages}"


@then(parsers.parse('the campaign of seller A keeps status "{status}"'))
def a_keeps(world: World, status: str) -> None:
    st = _x(world)
    mine = _by_id(_list_mine(st["seller_a"]), st["campaign"])
    assert mine and mine["status"] == status, mine


@then("seller A can still cancel the campaign")
def a_can_cancel(world: World) -> None:
    st = _x(world)
    resp = _mine(st["seller_a"], "CancelMyCampaign", {"campaignId": st["campaign"]})
    assert ac.campaign_of(resp)["status"] == ac.CANCELLED


@then(
    parsers.parse(
        'no "{action}" audit event exists for the campaign after team-audit is started again'
    )
)
def no_campaign_event(world: World, action: str) -> None:
    st = _x(world)
    aud.start_audit(world)
    aud.assert_no_event(world, target_type="flash_sale", target_id=st["campaign"], action=action)


@then("an admin cancel of another scheduled campaign is audited as a positive control")
def admin_cancel_control(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    other = ac.seller_campaign(
        st["seller_a"], _new_listing(world, st["seller_a"]), **_SCHEDULED_WINDOW
    )
    ac.campaign_of(ac.gw(st["admin"], "CancelCampaign", {"campaignId": other}))
    aud.assert_one_event(
        world,
        target_type="flash_sale",
        target_id=other,
        action="campaign.cancel",
        actor_id=st["admin_id"],
    )


# ── Thens: vouchers ──────────────────────────────────────────────────────
@then("the voucher list holds exactly A's three vouchers with quota and used set")
def exactly_a_vouchers(world: World) -> None:
    st = _x(world)
    listed = st["vlisted"]
    assert {v["id"] for v in listed} == set(st["ids"].values()), listed
    for voucher in listed:
        assert voucher.get("sellerId") == st["id_a"], voucher
        assert str(voucher.get("quota")) == "100", voucher
        assert str(voucher.get("used") or "0") == "0", voucher


@then("the voucher list holds neither seller B's voucher nor the platform voucher")
def neither(world: World) -> None:
    st = _x(world)
    assert _by_id(st["vlisted"], st["b_voucher"]) is None, st["vlisted"]
    assert _by_id(st["vlisted"], st["p_voucher"]) is None, st["vlisted"]


@then("the three vouchers show their active, deactivated and expired state")
def states(world: World) -> None:
    st = _x(world)
    active = _by_id(st["vlisted"], st["ids"]["active"])
    deactivated = _by_id(st["vlisted"], st["ids"]["deactivated"])
    expired = _by_id(st["vlisted"], st["ids"]["expired"])
    assert active.get("active") and not active.get("deactivatedAt"), active
    assert not deactivated.get("active") and deactivated.get("deactivatedAt"), deactivated
    assert expired.get("endsAt") and expired["endsAt"] < av.iso(), expired


@then(
    parsers.parse('the buyer\'s voucher calls are refused with "{code}" and carry no voucher data')
)
def buyer_voucher_refused(world: World, code: str) -> None:
    resps = _x(world)["resps"]
    _assert_all_refused(resps, code)
    assert all(not (r.json().get("vouchers") or r.json().get("voucher")) for r in resps)


@then("the active voucher of seller A is still active")
def still_active(world: World) -> None:
    st = _x(world)
    mine = _by_id(_list_vouchers(st["seller_a"]), st["ids"]["active"])
    assert mine and mine.get("active") and not mine.get("deactivatedAt"), mine


@then("the voucher response shows active false and a deactivated_at")
def voucher_deactivated(world: World) -> None:
    voucher = av.voucher_of(_x(world)["resp"])
    assert not voucher.get("active"), voucher
    assert voucher.get("deactivatedAt"), voucher


@then("the buyer previewing the voucher gets valid false")
def preview_invalid(world: World) -> None:
    body = _preview(world, _x(world)["codes"]["active"])
    assert not body.get("valid"), f"deactivated voucher still valid: {body}"


@then("seller A can still deactivate their active voucher")
def a_can_deactivate(world: World) -> None:
    st = _x(world)
    resp = _voucher_gw(
        st["seller_a"], "DeactivateMyShopVoucher", {"voucherId": st["ids"]["active"]}
    )
    assert not av.voucher_of(resp).get("active")


@then(
    parsers.parse(
        'no "{action}" audit event exists for the voucher after team-audit is started again'
    )
)
def no_voucher_event(world: World, action: str) -> None:
    st = _x(world)
    aud.start_audit(world)
    aud.assert_no_event(
        world, target_type="voucher", target_id=st["codes"]["active"], action=action
    )


@then("an admin deactivation of another shop voucher is audited as a positive control")
def admin_deactivate_control(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    code = av.fresh_code("SSSC")
    voucher = av.create_voucher(st["seller_a"], code, scope=av.SHOP)
    assert av.deactivate(st["admin"], voucher["id"]).status_code == 200
    assert av.audit_count(world, code, "voucher.deactivate") == 1


# ── Then: gateway edge ───────────────────────────────────────────────────
@then("the own-campaign list succeeds")
def own_ok(world: World) -> None:
    resp = _x(world)["resps"][0]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the admin campaign list is refused with "{code}"'))
def admin_refused(world: World, code: str) -> None:
    _assert_all_refused([_x(world)["resps"][1]], code)


# ── Admin regression guards ──────────────────────────────────────────────
@when("the admin cancels that campaign through the admin RPC")
def admin_cancels(world: World) -> None:
    st = _x(world)
    st["resp"] = ac.gw(st["admin"], "CancelCampaign", {"campaignId": st["campaign"]})


@when("the admin deactivates the active voucher of seller A through the admin RPC")
def admin_deactivates(world: World) -> None:
    st = _x(world)
    st["resp"] = av.deactivate(st["admin"], st["ids"]["active"])


@then(parsers.parse('exactly one "{action}" audit event exists for the campaign by the admin'))
def admin_campaign_event(world: World, action: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type="flash_sale",
        target_id=st["campaign"],
        action=action,
        actor_id=st["admin_id"],
    )


@then(parsers.parse('exactly one "{action}" audit event exists for the active voucher of seller A'))
def admin_voucher_event(world: World, action: str) -> None:
    st = _x(world)
    assert av.audit_count(world, st["codes"]["active"], action) == 1
