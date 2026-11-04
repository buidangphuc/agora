"""Steps for promo/promotion_admin_campaigns.feature (change promotion-admin-campaigns).

Black box through the gateway (Connect JSON); hold RPCs use the service principal over grpcurl.
Stored state is read back with ListCampaigns and the audit log with QueryAuditLog. The two UI
scenarios drive the admin app in the browser.
"""

from __future__ import annotations

import json
import re
import time
from datetime import datetime

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages.admin_campaigns_page import AdminCampaignsPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import audit_support as aud
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support.world import World

_KEY = re.compile(r"idempotency_?key\W+([A-Za-z0-9_-]{8,64})", re.I)
_KEY_VALUE = re.compile(r"[A-Za-z0-9_-]{8,64}")
_SNAPSHOT_KEYS = ("salePrice", "stockCap", "startsAt", "endsAt", "status", "cancelledAt")


def _action_key(body: str) -> str:
    try:
        args = json.loads(body)
    except ValueError:
        args = None
    if isinstance(args, list):
        for item in reversed(args):
            if isinstance(item, str) and _KEY_VALUE.fullmatch(item):
                return item
    found = _KEY.search(body)
    return found.group(1) if found else ""


def _x(world: World) -> dict:
    return ac.state(world)


def _ts(value: str | None) -> int | None:
    if not value:
        return None
    return int(datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp())


def _snapshot(campaign: dict) -> dict:
    snap = {k: campaign.get(k) for k in _SNAPSHOT_KEYS}
    for key in ("startsAt", "endsAt", "cancelledAt"):
        snap[key] = _ts(snap[key])
    snap["salePrice"] = str(snap["salePrice"])
    snap["stockCap"] = str(snap["stockCap"])
    return snap


def _stored(world: World, campaign_id: str) -> dict:
    campaign = ac.find(_x(world)["admin"], campaign_id)
    assert campaign is not None, f"campaign {campaign_id} is not in the admin list"
    return campaign


def _last(world: World) -> httpx.Response:
    return _x(world)["resp"]


def _record(world: World, resp: httpx.Response) -> httpx.Response:
    _x(world)["resp"] = resp
    return resp


def _update_body(campaign_id: str, price: int = 4_100_000, cap: int = 20, days: float = 4) -> dict:
    return {
        "campaignId": campaign_id,
        "salePrice": str(price),
        "stockCap": str(cap),
        "startsAt": ac.iso(days=days),
        "endsAt": ac.iso(days=days + 2),
    }


def _new_listing(world: World) -> str:
    return ac.make_listing(world, _x(world)["seller"])


def _active_pages(token: str | None) -> tuple[list[dict], bool]:
    found: list[dict] = []
    disabled = False
    cursor = ""
    for _ in range(60):
        resp = ac.gw(token, "ListActiveCampaigns", {"page": {"pageSize": 200, "cursor": cursor}})
        assert resp.status_code == 200, f"ListActiveCampaigns {resp.status_code}: {resp.text}"
        data = resp.json()
        found.extend(data.get("campaigns") or [])
        disabled = disabled or bool(data.get("flashSaleDisabled"))
        cursor = (data.get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found, disabled


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin for the campaign admin checks")
def seeded_admin(world: World) -> None:
    st = _x(world)
    st["admin"] = aud.admin_token(world)
    st["admin_id"] = ac.av.vs.token_sub(st["admin"])


@given("a seller with a published listing for the campaign admin checks")
def seller_with_listing(world: World) -> None:
    st = _x(world)
    st["seller"] = world.service_factory.auth.register(
        fake.unique_username("ac_seller"), world.settings.seed_password, "seller"
    )
    st["listing"] = ac.make_listing(world, st["seller"])


@given("an active campaign on the listing")
def active_campaign(world: World) -> None:
    st = _x(world)
    st["campaign"] = ac.seller_campaign(st["seller"], st["listing"])


@given(parsers.parse("an active campaign on the listing with stock cap {cap:d}"))
def active_campaign_cap(world: World, cap: int) -> None:
    st = _x(world)
    st["campaign"] = ac.seller_campaign(st["seller"], st["listing"], stock_cap=cap)


@given("a scheduled campaign on the listing")
def scheduled_campaign(world: World) -> None:
    st = _x(world)
    st["campaign"] = ac.seller_campaign(
        st["seller"], st["listing"], starts_in_s=86400 * 2, ends_in_s=86400 * 4
    )
    st["before"] = _snapshot(_stored(world, st["campaign"]))


@given("one scheduled, one active, one ended and one cancelled campaign exist")
def four_campaigns(world: World) -> None:
    st = _x(world)
    seller, admin = st["seller"], st["admin"]
    camps = {
        "scheduled": ac.seller_campaign(
            seller, st["listing"], starts_in_s=86400 * 2, ends_in_s=86400 * 4
        ),
        "active": ac.seller_campaign(seller, _new_listing(world)),
        "ended": ac.seller_campaign(seller, _new_listing(world)),
        "cancelled": ac.seller_campaign(seller, _new_listing(world)),
    }
    ac.end_now(camps["ended"])
    resp = ac.gw(admin, "CancelCampaign", {"campaignId": camps["cancelled"]})
    assert resp.status_code == 200, (
        f"seeding a cancelled campaign failed: {resp.status_code} {resp.text}"
    )
    st["camps"] = camps
    st["campaign"] = camps["scheduled"]
    st["before_all"] = {k: _snapshot(_stored(world, v)) for k, v in camps.items()}


@given("the admin list holds the campaign")
def admin_list_holds(world: World) -> None:
    st = _x(world)
    assert ac.find(st["admin"], st["campaign"]) is not None


@given("the active campaign list shows it with the kill-switch on")
def active_list_shows(world: World) -> None:
    st = _x(world)
    seen, disabled = _active_pages(None)
    assert not disabled, "kill-switch is already off before the scenario"
    assert st["campaign"] in {c["id"] for c in seen}, "active campaign missing from the public list"


@given("the flash-sale kill-switch is switched off")
def kill_switch_off(world: World) -> None:
    def restore() -> None:
        ac.set_flash_flag(True)
        ac.wait_active_disabled(False)

    world.add_cleanup(restore)
    ac.set_flash_flag(False)
    ac.wait_active_disabled(True)


@given(parsers.parse("{sold:d} units of the campaign are sold and {held:d} unit is held"))
def sold_and_held(world: World, sold: int, held: int) -> None:
    st = _x(world)
    base = (st["campaign"], st["listing"])
    rid = f"ac-sold-{fake.unique_username('r')}"
    code, body = fh.reserve(rid, *base, sold, ac.SALE_PRICE)
    assert code == "ok" and body.get("reserved"), f"seed reserve refused: {code} {body}"
    code, body = fh.commit(rid)
    assert code == "ok" and body.get("committed"), f"seed commit failed: {code} {body}"
    hold = f"ac-held-{fake.unique_username('r')}"
    code, body = fh.reserve(hold, *base, held, ac.SALE_PRICE)
    assert code == "ok" and body.get("reserved"), f"seed hold refused: {code} {body}"
    st["held"] = hold


@given("team-audit is stopped for the campaign admin checks")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@given("team-listing is stopped for the campaign admin checks")
def listing_stopped(world: World) -> None:
    ac.stop_listing(world, _x(world)["admin"])


@given("an admin create with a valid payload succeeds on another listing")
def control_create(world: World) -> None:
    st = _x(world)
    other = _new_listing(world)
    resp = ac.gw(st["admin"], "CreateCampaign", ac.admin_create_body(other, ac.fresh_key()))
    ac.campaign_of(resp)


@given("an admin create on the listing succeeded with a fresh key")
def admin_created(world: World) -> None:
    st = _x(world)
    st["key"] = ac.fresh_key()
    st["body"] = ac.admin_create_body(st["listing"], st["key"])
    st["created"] = ac.campaign_of(ac.gw(st["admin"], "CreateCampaign", st["body"]))
    st["campaign"] = st["created"]["id"]


@given("the admin has cancelled the created campaign")
def admin_cancelled(world: World) -> None:
    st = _x(world)
    campaign = ac.campaign_of(ac.gw(st["admin"], "CancelCampaign", {"campaignId": st["campaign"]}))
    assert campaign.get("status") == ac.CANCELLED, campaign


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse('the admin lists campaigns filtered by "{status}"'))
def admin_lists_filtered(world: World, status: str) -> None:
    st = _x(world)
    st["listed"], st["disabled"] = ac.all_campaigns(st["admin"], [status])


@when("anyone lists the active campaigns")
def anyone_lists_active(world: World) -> None:
    st = _x(world)
    st["active_listed"], st["active_disabled"] = _active_pages(None)


@when("the seller lists all campaigns")
def seller_lists(world: World) -> None:
    st = _x(world)
    st["seller_resp"] = ac.list_campaigns(st["seller"])


@when("an anonymous caller lists all campaigns")
def anon_lists(world: World) -> None:
    _x(world)["anon_resp"] = ac.list_campaigns(None)


@when("the admin cancels the campaign")
def admin_cancels(world: World) -> None:
    st = _x(world)
    _record(world, ac.gw(st["admin"], "CancelCampaign", {"campaignId": st["campaign"]}))


@when("the admin cancels the campaign successfully")
def admin_cancels_ok(world: World) -> None:
    st = _x(world)
    resp = _record(world, ac.gw(st["admin"], "CancelCampaign", {"campaignId": st["campaign"]}))
    assert resp.status_code == 200, f"CancelCampaign {resp.status_code}: {resp.text}"


@when("an anonymous caller cancels the campaign")
def anon_cancels(world: World) -> None:
    st = _x(world)
    _record(world, ac.gw(None, "CancelCampaign", {"campaignId": st["campaign"]}))


@when(parsers.parse("a buyer tries to reserve {qty:d} unit of the campaign"))
def buyer_reserves(world: World, qty: int) -> None:
    st = _x(world)
    st["reserve"] = fh.reserve(
        f"ac-new-{fake.unique_username('r')}", st["campaign"], st["listing"], qty, ac.SALE_PRICE
    )


@when("the held unit is committed")
def commit_held(world: World) -> None:
    _x(world)["commit"] = fh.commit(_x(world)["held"])


@when("the admin updates the campaign to sale price 90000, stock cap 50 and a later window")
def admin_updates(world: World) -> None:
    st = _x(world)
    st["update_body"] = _update_body(st["campaign"], price=90000, cap=50, days=5)
    _record(world, ac.gw(st["admin"], "UpdateCampaign", st["update_body"]))


@when("an anonymous caller updates the campaign")
def anon_updates(world: World) -> None:
    st = _x(world)
    _record(world, ac.gw(None, "UpdateCampaign", _update_body(st["campaign"])))


@when("the admin updates the active, the ended and the cancelled campaign")
def admin_updates_started(world: World) -> None:
    st = _x(world)
    st["started_resps"] = {
        name: ac.gw(st["admin"], "UpdateCampaign", _update_body(st["camps"][name]))
        for name in ("active", "ended", "cancelled")
    }


@when("the admin creates a campaign with sale price equal to the listing price")
def create_price_equal(world: World) -> None:
    st = _x(world)
    st["since"] = ac.iso(offset_s=-2)
    st["events_before"] = ac.admin_create_events(world, st["since"])
    body = ac.admin_create_body(st["listing"], ac.fresh_key(), sale_price=ac.LISTING_PRICE)
    _record(world, ac.gw(st["admin"], "CreateCampaign", body))


@when("the admin creates a campaign for a listing id that does not exist")
def create_unknown_listing(world: World) -> None:
    st = _x(world)
    body = ac.admin_create_body(f"no-such-listing-{fake.unique_username('x')}", ac.fresh_key())
    _record(world, ac.gw(st["admin"], "CreateCampaign", body))


@when("the admin creates a campaign on the listing and the time is measured")
def create_timed(world: World) -> None:
    st = _x(world)
    body = ac.admin_create_body(st["listing"], ac.fresh_key())
    started = time.monotonic()
    _record(world, ac.gw(st["admin"], "CreateCampaign", body))
    st["elapsed"] = time.monotonic() - started


@when("the admin creates a campaign on the listing with a fresh key")
def create_fresh_key(world: World) -> None:
    st = _x(world)
    st["key"] = ac.fresh_key()
    st["body"] = ac.admin_create_body(st["listing"], st["key"])
    _record(world, ac.gw(st["admin"], "CreateCampaign", st["body"]))


@when("the admin creates a campaign on the listing with a fresh key twice with the same payload")
def create_twice(world: World) -> None:
    st = _x(world)
    st["key"] = ac.fresh_key()
    st["body"] = ac.admin_create_body(st["listing"], st["key"])
    st["twice"] = [ac.gw(st["admin"], "CreateCampaign", st["body"]) for _ in range(2)]


@when("the admin reuses the key with a different sale price")
def reuse_key_different(world: World) -> None:
    st = _x(world)
    body = dict(st["body"], salePrice=str(ac.SALE_PRICE - 1000))
    _record(world, ac.gw(st["admin"], "CreateCampaign", body))


@when("the admin replays the original create request with the same key and payload")
def replay(world: World) -> None:
    st = _x(world)
    _record(world, ac.gw(st["admin"], "CreateCampaign", st["body"]))


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('the list holds the scheduled campaign with status "{status}"'))
def list_holds_scheduled(world: World, status: str) -> None:
    st = _x(world)
    hits = [c for c in st["listed"] if c.get("id") == st["camps"]["scheduled"]]
    assert len(hits) == 1, f"scheduled campaign not listed once: {hits}"
    assert hits[0].get("status") == status, hits[0]
    assert all(c.get("status") == status for c in st["listed"]), "a non-matching status leaked"


@then("the list holds none of the active, ended and cancelled campaigns")
def list_holds_none(world: World) -> None:
    st = _x(world)
    ids = {c.get("id") for c in st["listed"]}
    for name in ("active", "ended", "cancelled"):
        assert st["camps"][name] not in ids, f"{name} campaign leaked into the SCHEDULED filter"


@then("an unfiltered list shows the four campaigns with their derived statuses")
def unfiltered_four(world: World) -> None:
    st = _x(world)
    listed, _ = ac.all_campaigns(st["admin"])
    by_id = {c["id"]: c for c in listed}
    want = {
        "scheduled": ac.SCHEDULED,
        "active": ac.ACTIVE,
        "ended": ac.ENDED,
        "cancelled": ac.CANCELLED,
    }
    for name, status in want.items():
        got = by_id.get(st["camps"][name])
        assert got is not None, f"{name} campaign missing from the unfiltered list"
        assert got.get("status") == status, f"{name}: {got.get('status')} != {status}"
    assert by_id[st["camps"]["cancelled"]].get("cancelledAt"), (
        "cancelled campaign has no cancelledAt"
    )


@then("the admin list still holds the campaign and reports flash_sale_disabled")
def admin_list_disabled(world: World) -> None:
    st = _x(world)
    assert st["disabled"] is True, "ListCampaigns did not report flash_sale_disabled"
    assert st["campaign"] in {c["id"] for c in st["listed"]}, (
        "campaigns vanished with the switch off"
    )


@then("the active campaign list is empty and reports flash_sale_disabled")
def active_disabled(world: World) -> None:
    st = _x(world)
    assert st["active_disabled"] is True
    assert st["active_listed"] == [], f"active list not empty: {st['active_listed']}"


@then(parsers.parse('the seller is refused with "{code}" and no campaign is returned'))
def seller_refused(world: World, code: str) -> None:
    resp = _x(world)["seller_resp"]
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"
    assert not (resp.json().get("campaigns") or [])


@then(parsers.parse('the anonymous caller is refused with "{code}" and no campaign is returned'))
def anon_refused(world: World, code: str) -> None:
    resp = _x(world)["anon_resp"]
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"
    assert not (resp.json().get("campaigns") or [])


@then(parsers.parse('the call is refused with "{code}"'))
def call_refused(world: World, code: str) -> None:
    resp = _last(world)
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the call is rejected with "{code}" on field "{field}"'))
def call_rejected_field(world: World, code: str, field: str) -> None:
    resp = _last(world)
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"
    assert field in ac.message_of(resp), f"message does not name {field}: {resp.text}"


@then(parsers.parse('the call fails with "{code}" and reason "{reason}" within 2.5 seconds'))
def call_fails_fast(world: World, code: str, reason: str) -> None:
    st = _x(world)
    resp = _last(world)
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"
    assert ac.error_info(resp).get("reason") == reason, resp.text
    assert st["elapsed"] <= 2.5, f"outage took {st['elapsed']:.2f}s"


@then(parsers.parse('the call fails with "{code}" and reason "{reason}" in domain "{domain}"'))
def call_fails_domain(world: World, code: str, reason: str, domain: str) -> None:
    resp = _last(world)
    assert ac.code_of(resp) == code, f"{resp.status_code} {resp.text}"
    info = ac.error_info(resp)
    assert info.get("reason") == reason and info.get("domain") == domain, resp.text


@then(parsers.parse('the campaign is still "{status}"'))
def campaign_still(world: World, status: str) -> None:
    st = _x(world)
    assert _stored(world, st["campaign"]).get("status") == status


@then(parsers.parse('once team-audit is back the campaign is still "{status}"'))
def audit_back_still(world: World, status: str) -> None:
    aud.start_audit(world)
    campaign_still(world, status)


@then("the admin can then cancel the campaign")
def admin_can_cancel(world: World) -> None:
    st = _x(world)
    campaign = ac.campaign_of(ac.gw(st["admin"], "CancelCampaign", {"campaignId": st["campaign"]}))
    assert campaign.get("status") == ac.CANCELLED, campaign


@then(parsers.parse('the new reserve is refused with reason "{reason}"'))
def reserve_refused(world: World, reason: str) -> None:
    code, body = _x(world)["reserve"]
    assert code == "ok" and not body.get("reserved"), (code, body)
    assert body.get("reason") == reason, body


@then("the commit of the held unit succeeds")
def commit_ok(world: World) -> None:
    code, body = _x(world)["commit"]
    assert code == "ok" and body.get("committed"), (code, body)


@then(parsers.parse('the campaign shows status "{status}" and stock_sold {sold:d}'))
def campaign_sold(world: World, status: str, sold: int) -> None:
    campaign = _stored(world, _x(world)["campaign"])
    assert campaign.get("status") == status, campaign
    assert int(campaign.get("stockSold", 0) or 0) == sold, campaign


def _assert_values(campaign: dict, body: dict) -> None:
    assert str(campaign.get("salePrice")) == body["salePrice"], campaign
    assert str(campaign.get("stockCap")) == body["stockCap"], campaign
    assert _ts(campaign.get("startsAt")) == _ts(body["startsAt"]), campaign
    assert _ts(campaign.get("endsAt")) == _ts(body["endsAt"]), campaign


@then(parsers.parse('the update response shows those values with status "{status}"'))
def update_response(world: World, status: str) -> None:
    st = _x(world)
    campaign = ac.campaign_of(_last(world))
    _assert_values(campaign, st["update_body"])
    assert campaign.get("status") == status, campaign


@then(parsers.parse('a following list shows those values with status "{status}"'))
def update_listed(world: World, status: str) -> None:
    st = _x(world)
    campaign = _stored(world, st["campaign"])
    _assert_values(campaign, st["update_body"])
    assert campaign.get("status") == status, campaign


@then(parsers.parse('exactly one "{action}" audit event exists for the campaign'))
def one_audit_event(world: World, action: str) -> None:
    st = _x(world)
    events = ac.audit_events(world, st["campaign"], action)
    assert len(events) == 1, events
    assert events[0].get("actorId") == st["admin_id"], events[0]


@then(parsers.parse('each update is refused with "{code}"'))
def each_refused(world: World, code: str) -> None:
    for name, resp in _x(world)["started_resps"].items():
        assert ac.code_of(resp) == code, f"{name}: {resp.status_code} {resp.text}"


@then("the stored values of those three campaigns are unchanged")
def three_unchanged(world: World) -> None:
    st = _x(world)
    for name in ("active", "ended", "cancelled"):
        assert _snapshot(_stored(world, st["camps"][name])) == st["before_all"][name], name


@then("the stored values of the campaign are unchanged")
def one_unchanged(world: World) -> None:
    st = _x(world)
    assert _snapshot(_stored(world, st["campaign"])) == st["before"]


@then("the admin can still update the scheduled campaign")
def admin_can_update(world: World) -> None:
    st = _x(world)
    body = _update_body(st["campaign"], price=4_200_000, cap=30, days=6)
    campaign = ac.campaign_of(ac.gw(st["admin"], "UpdateCampaign", body))
    _assert_values(campaign, body)


@then("no campaign exists for the listing and no create audit entry was written")
def nothing_created(world: World) -> None:
    st = _x(world)
    assert ac.for_listing(st["admin"], st["listing"]) == []
    assert ac.admin_create_events(world, st["since"]) == st["events_before"]


@then("the admin can create a campaign with the sale price one below the listing price")
def create_boundary(world: World) -> None:
    st = _x(world)
    body = ac.admin_create_body(st["listing"], ac.fresh_key(), sale_price=ac.LISTING_PRICE - 1)
    campaign = ac.campaign_of(ac.gw(st["admin"], "CreateCampaign", body))
    assert campaign.get("status") == ac.SCHEDULED, campaign


@then("once team-listing is back no campaign exists for the listing")
def listing_back_none(world: World) -> None:
    st = _x(world)
    ac.start_listing(world, st["admin"])
    assert ac.for_listing(st["admin"], st["listing"]) == []


@then("once team-audit is back no campaign exists for the listing")
def audit_back_none(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    assert ac.for_listing(st["admin"], st["listing"]) == []


@then("retrying the create with the same key succeeds")
def retry_same_key(world: World) -> None:
    st = _x(world)
    campaign = ac.campaign_of(ac.gw(st["admin"], "CreateCampaign", st["body"]))
    assert campaign.get("listingId") == st["listing"], campaign


@then("both calls return the same campaign id")
def same_id(world: World) -> None:
    first, second = (ac.campaign_of(r) for r in _x(world)["twice"])
    assert first["id"] == second["id"], (first, second)
    _x(world)["campaign"] = first["id"]


@then("the list holds exactly one campaign for the listing")
def exactly_one(world: World) -> None:
    st = _x(world)
    rows = ac.for_listing(st["admin"], st["listing"])
    assert len(rows) == 1, rows


@then("the original campaign is unchanged and is the only one for the listing")
def original_unchanged(world: World) -> None:
    st = _x(world)
    rows = ac.for_listing(st["admin"], st["listing"])
    assert [r["id"] for r in rows] == [st["campaign"]], rows
    assert str(rows[0].get("salePrice")) == str(st["created"].get("salePrice")), rows[0]


@then(parsers.parse('the replay returns the same campaign id with status "{status}"'))
def replay_same(world: World, status: str) -> None:
    st = _x(world)
    campaign = ac.campaign_of(_last(world))
    assert campaign["id"] == st["campaign"], campaign
    assert campaign.get("status") == status, campaign


# ── Admin app (browser) ──────────────────────────────────────────────────
def _page(world: World) -> AdminCampaignsPage:
    return AdminCampaignsPage(world.page)


@given("an admin is signed in the admin app for the campaign checks")
def ui_signed_in(world: World) -> None:
    from src.utils import get_test_data_manager

    login_via_api(world, get_test_data_manager().get_user_by_role("admin"))


@given("the admin has opened the campaign list screen")
def ui_open_list(world: World) -> None:
    page = _page(world)
    page.navigate()
    page.wait_loaded()


@given("the admin has the campaign create form filled for the listing")
def ui_create_form(world: World) -> None:
    st = _x(world)
    page = _page(world)
    page.goto(f"{page.app_base_url()}/admin/campaigns/new")
    start, end = ac.tomorrow_window()
    page.fill_create_form(st["listing"], ac.SALE_PRICE, 10, start, end)


@given("the first create request loses its response")
def ui_first_lost(world: World) -> None:
    st = _x(world)
    st["keys"] = []

    def handler(route) -> None:
        request = route.request
        if request.method != "POST":
            route.continue_()
            return
        st["keys"].append(_action_key(request.post_data or ""))
        if len(st["keys"]) == 1:
            route.fetch()
            route.abort("timedout")
        else:
            route.continue_()

    world.page.route("**/admin/campaigns/new", handler)


@when(
    parsers.parse('the admin picks the filter "{status}" and confirms cancel on the campaign row')
)
def ui_cancel(world: World, status: str) -> None:
    st = _x(world)
    page = _page(world)
    page.choose_filter(status)
    row = page.find_row(st["listing"], st["campaign"])
    assert row is not None, "the active campaign is not listed under the active filter"
    page.cancel_row(row)


@when("the admin submits the form and then submits it again")
def ui_submit_twice(world: World) -> None:
    st = _x(world)
    page = _page(world)
    page.submit_form()
    deadline = time.monotonic() + 30
    while len(st["keys"]) < 1 and time.monotonic() < deadline:
        time.sleep(0.25)
    assert st["keys"], "the first create request was never sent"
    time.sleep(1)
    page.submit_form()
    deadline = time.monotonic() + 30
    while len(st["keys"]) < 2 and time.monotonic() < deadline:
        time.sleep(0.25)


@then("the row shows status cancelled")
def ui_row_cancelled(world: World) -> None:
    st = _x(world)
    page = _page(world)
    page.choose_filter("all")
    row = page.find_row(st["listing"], st["campaign"])
    assert row is not None, "the campaign row is gone from the unfiltered list"
    expect(row).to_contain_text(re.compile(r"cancelled|đã h(ủy|uỷ)", re.I), timeout=timeouts.DEFAULT)


@then("the active filter no longer lists the campaign")
def ui_not_active(world: World) -> None:
    st = _x(world)
    page = _page(world)
    page.choose_filter("active")
    page.page.wait_for_load_state("networkidle")
    expect(page.row_of(st["listing"], st["campaign"])).to_have_count(0, timeout=timeouts.DEFAULT)


@then(parsers.parse('the stored campaign is "{status}"'))
def stored_campaign(world: World, status: str) -> None:
    campaign_still(world, status)


@then("both create requests carry the same idempotency_key")
def ui_same_key(world: World) -> None:
    keys = _x(world)["keys"]
    assert len(keys) == 2, f"expected two create requests, saw {keys}"
    assert keys[0] and keys[0] == keys[1], keys
