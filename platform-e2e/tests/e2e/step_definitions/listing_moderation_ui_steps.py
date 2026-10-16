"""Steps for the four frontend moderation features (change frontend-listing-moderation,
initiative q6-trust-enforcement): seller_listing_status_ui, listing_report_ui,
admin_listing_moderation_ui, admin_cases_ui.

Listings, reports and moderation decisions that are only set-up go through the gateway (Connect
JSON); everything the change promises is asserted on the rendered page, plus the stored state read
back through the gateway. Fresh accounts per scenario, never a shared seeded seller or buyer. The
flagged listing holds the seeded keyword `replica` and the team-ai mock marker
`screen-review-marker` (same fixture as seller/listing_moderation.feature).
"""

from __future__ import annotations

import re
import time
import uuid
from urllib.parse import quote

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.models import User
from src.pages.admin_moderation_page import (
    FORBIDDEN_TITLE,
    AdminModerationPage,
    page_text,
)
from src.pages.login_page import LoginPage
from src.pages.seller_listings_page import SellerListingsPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support import audit_support as aud
from tests.e2e.support import report_support as rs
from tests.e2e.support import srm_support as s
from tests.e2e.support.oic_order_support import Actor
from tests.e2e.support.world import World

LISTING = "/platform.listing.v1.ListingService/"
FLAGGED_MARKERS = "Replica screen-review-marker"
ADMIN_ROUTE = "/admin/cases"


def _x(world: World) -> dict:
    return world.state.extra.setdefault("modui", {})


def _base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def _admin_page(world: World) -> AdminModerationPage:
    return AdminModerationPage(world.page)


def _anonymous(world: World) -> None:
    world.context.clear_cookies()
    # clear_cookies also drops the consent choice seeded by setup_web_context; re-seed it so
    # the cookie banner does not cover the page.
    world.context.add_cookies(
        [
            {
                "name": "agora_consent",
                "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
                "url": world.settings.base_url,
            }
        ]
    )
    world.service_factory.set_token(None)


def _sign_in(world: World, user: dict) -> None:
    _anonymous(world)
    login_via_api(world, User(user["username"], user["password"], user["role"]))


def _ensure_seller(world: World) -> None:
    """Sign the scenario's seller in the browser unless that session is already active."""
    seller = _x(world)["seller"]
    current = world.state.current_user
    if current is None or current.username != seller["username"] or not world.context.cookies():
        _sign_in(world, seller)


def _register(world: World, role: str, prefix: str) -> dict:
    username = fake.unique_username(prefix)
    password = get_settings().seed_password
    token = world.service_factory.auth.register(username, password, role)
    return {
        "username": username,
        "password": password,
        "role": role,
        "token": token,
        "id": s.subject(token),
    }


def _listing_body(title: str) -> dict:
    return {
        "listing": {
            "title": title,
            "description": "moderation ui e2e",
            "categoryId": "cat-electronics",
            "price": 100000,
            "stock": 3,
            "status": "LISTING_STATUS_PUBLISHED",
            "currency": "VND",
        }
    }


def _create(world: World, token: str, title: str) -> str:
    resp = s.post(LISTING + "CreateListing", _listing_body(title), token)
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    listing_id = resp.json()["listing"]["id"]

    def cleanup() -> None:
        try:
            s.post(LISTING + "DeleteListing", {"id": listing_id}, _x(world)["admin_token"])
        except Exception:  # best effort: a banned listing may refuse deletes
            pass

    world.add_cleanup(cleanup)
    return listing_id


def _status(token: str | None, listing_id: str) -> tuple[int, dict]:
    resp = s.post(LISTING + "GetListing", {"id": listing_id}, token)
    return resp.status_code, resp.json() if resp.content else {}


def _admin_token(world: World) -> str:
    x = _x(world)
    if "admin_token" not in x:
        x["admin_token"] = aud.admin_token(world)
    return x["admin_token"]


def _admin_actor(world: World) -> Actor:
    token = _admin_token(world)
    return Actor("admin", token, "admin", s.subject(token))


def _listing_title(world: World, which: str) -> str:
    return _x(world)[f"{which}_title"]


# ── Givens ───────────────────────────────────────────────────────────────
@given("a moderation seller who published a clean and a flagged listing")
def seller_clean_and_flagged(world: World) -> None:
    x = _x(world)
    _admin_token(world)
    x["seller"] = _register(world, "seller", "modui_seller")
    uniq = f"modui{uuid.uuid4().hex[:10]}"
    x["clean_title"] = f"Clean laptop stand {uniq}"
    x["flagged_title"] = f"{FLAGGED_MARKERS} {uniq}"
    x["clean_id"] = _create(world, x["seller"]["token"], x["clean_title"])
    x["flagged_id"] = _create(world, x["seller"]["token"], x["flagged_title"])


@given("a moderation seller with a published clean listing")
def seller_clean(world: World) -> None:
    x = _x(world)
    _admin_token(world)
    x["seller"] = _register(world, "seller", "modui_seller")
    x["listing_title"] = f"Clean laptop stand modui{uuid.uuid4().hex[:10]}"
    x["listing_id"] = _create(world, x["seller"]["token"], x["listing_title"])
    code, body = _status(None, x["listing_id"])
    assert code == 200 and body["listing"]["status"] == "LISTING_STATUS_PUBLISHED", (
        f"setup: clean listing not published for anonymous: {code} {body}"
    )


@given("a freshly registered moderation buyer")
def fresh_buyer(world: World) -> None:
    _x(world)["buyer"] = _register(world, "buyer", "modui_buyer")


@given("the engagement service knows the moderation listing")
def engagement_knows(world: World) -> None:
    rs.wait_seller_listing_known(_x(world)["listing_id"])


@given("the moderation buyer is signed in the browser")
def buyer_signed_in(world: World) -> None:
    _sign_in(world, _x(world)["buyer"])


@given("the moderation seller is signed in the browser")
@when("the moderation seller is signed in the browser")
def seller_signed_in(world: World) -> None:
    _sign_in(world, _x(world)["seller"])


@given("an admin is signed in the browser")
@when("an admin is signed in the browser")
def admin_signed_in(world: World) -> None:
    from src.utils import get_test_data_manager

    _anonymous(world)
    login_via_api(world, get_test_data_manager().get_user_by_role("admin"))


@given(
    parsers.parse(
        'an admin has rejected the flagged listing with reason "{reason}" through the gateway'
    )
)
def admin_rejected(world: World, reason: str) -> None:
    x = _x(world)
    resp = s.post(
        LISTING + "ModerateListing",
        {
            "listingId": x["flagged_id"],
            "decision": "MODERATION_DECISION_REJECT",
            "reasonCode": reason,
            "note": "Ảnh là hàng nhái",
        },
        _admin_token(world),
    )
    assert resp.status_code == 200, f"setup ModerateListing {resp.status_code}: {resp.text}"


@given(parsers.parse('the buyer has reported the listing through the gateway with the note "{note}"'))
def buyer_reported(world: World, note: str) -> None:
    x = _x(world)
    buyer = x["buyer"]
    actor = Actor(buyer["username"], buyer["token"], "buyer", buyer["id"])
    resp = rs.create_report(actor, x["listing_id"], "scam", note)
    assert resp.status_code == 200, f"setup CreateReport {resp.status_code}: {resp.text}"
    x["report_id"] = resp.json()["report"]["id"]
    x["report_note"] = note


@given("team-audit is down for moderation")
def audit_down(world: World) -> None:
    aud.stop_audit(world)


@when("team-audit is up again for moderation")
def audit_up(world: World) -> None:
    aud.start_audit(world)


# ── Whens: seller surfaces ───────────────────────────────────────────────
@when("the seller opens the seller center")
def open_seller_center(world: World) -> None:
    _ensure_seller(world)
    world.page.goto(f"{_base(world)}/seller", wait_until="domcontentloaded")


def _open_edit(world: World, listing_id: str) -> None:
    world.page.goto(f"{_base(world)}/seller/{listing_id}/edit", wait_until="domcontentloaded")


@when("the seller opens the edit page of the flagged listing")
def open_edit_flagged(world: World) -> None:
    _ensure_seller(world)
    _open_edit(world, _x(world)["flagged_id"])


@when("the seller opens the edit page of the listing")
def open_edit_listing(world: World) -> None:
    _ensure_seller(world)
    _open_edit(world, _x(world)["listing_id"])


@when("the seller saves an edit of the flagged listing in the browser")
def seller_saves_edit(world: World) -> None:
    x = _x(world)
    _ensure_seller(world)
    _open_edit(world, x["flagged_id"])
    title = world.page.locator("#title")
    expect(title).to_be_visible(timeout=timeouts.NAVIGATION)
    submit = world.page.get_by_role("button", name="Lưu thay đổi")
    _admin_page(world).wait_until_interactive(submit)
    title.fill(f"{x['flagged_title']} v2")
    submit.click()


# ── Whens: report from the PDP ───────────────────────────────────────────
@when("the visitor opens the reported listing page")
def open_reported_page(world: World) -> None:
    world.page.goto(f"{_base(world)}/listing/{_x(world)['listing_id']}", wait_until="domcontentloaded")


@when(parsers.parse('the visitor clicks "{name}"'))
def click_report_control(world: World, name: str) -> None:
    # A guest sees a login link (returnUrl) instead of a button; accept either role.
    page = world.page
    control = page.get_by_role("button", name=name).or_(page.get_by_role("link", name=name))
    expect(control.first).to_be_visible(timeout=timeouts.NAVIGATION)
    _admin_page(world).wait_until_interactive(control)
    control.first.click()


@when("the moderation buyer signs in on that login page")
def buyer_logs_in(world: World) -> None:
    buyer = _x(world)["buyer"]
    LoginPage(world.page).login(buyer["username"], buyer["password"])


@when(
    parsers.parse(
        'the moderation buyer reports the listing in the browser with reason "{reason}" and the note "{note}"'
    )
)
def buyer_reports_in_browser(world: World, reason: str, note: str) -> None:
    page = _admin_page(world)
    open_reported_page(world)
    click_report_control(world, "Báo cáo tin đăng")
    dialog = page.dialog
    expect(dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    page.choose(dialog, "Lý do", reason)
    page.fill_note(dialog, note)
    page.press(dialog, "Gửi báo cáo")


# ── Whens: admin pages ───────────────────────────────────────────────────
@when("the visitor opens the moderation queue")
def open_queue(world: World) -> None:
    world.page.goto(f"{_base(world)}/admin/listings", wait_until="domcontentloaded")


@when("the admin opens the cases inbox")
def open_cases(world: World) -> None:
    world.page.goto(f"{_base(world)}{ADMIN_ROUTE}", wait_until="domcontentloaded")


@when("the admin opens the cockpit")
def open_cockpit(world: World) -> None:
    world.page.goto(f"{_base(world)}/admin/cockpit", wait_until="domcontentloaded")


@when(parsers.parse('the admin follows the link "{name}"'))
def follow_link(world: World, name: str) -> None:
    link = world.page.get_by_role("link", name=name).first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()


@when(parsers.parse('the buyer follows the footer link "{name}"'))
def follow_footer_link(world: World, name: str) -> None:
    world.page.goto(f"{_base(world)}/", wait_until="domcontentloaded")
    link = world.page.get_by_role("contentinfo").get_by_role("link", name=name).first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()


@when(parsers.parse('the admin presses "{name}" on the flagged listing'))
def admin_presses(world: World, name: str) -> None:
    page = _admin_page(world)
    row = page.find_row(_x(world)["flagged_title"])
    assert row is not None, "flagged listing is not in the moderation queue"
    page.press(row, name)


@when(
    parsers.parse(
        'the admin rejects the flagged listing with reason "{reason}" and the note "{note}"'
    )
)
def admin_rejects(world: World, reason: str, note: str) -> None:
    page = _admin_page(world)
    row = page.find_row(_x(world)["flagged_title"])
    assert row is not None, "flagged listing is not in the moderation queue"
    page.press(row, "Từ chối")
    dialog = page.dialog
    expect(dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    page.choose(dialog, "Lý do", reason)
    page.fill_note(dialog, note)
    page.confirm(dialog)


def _report_row(world: World):
    page = _admin_page(world)
    row = page.find_row(_x(world)["listing_id"])
    assert row is not None, "report for the listing is not in the open reports"
    return row


@when(
    parsers.parse(
        'the admin starts "Xử lý tin đăng" on the report with decision "{decision}" and reason "{reason}"'
    )
)
def admin_handles_listing(world: World, decision: str, reason: str) -> None:
    page = _admin_page(world)
    page.press(_report_row(world), "Xử lý tin đăng")
    dialog = page.dialog
    expect(dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    page.choose(dialog, "Quyết định", decision)
    page.choose(dialog, "Lý do", reason)
    page.confirm(dialog)
    expect(dialog).to_be_hidden(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the admin closes the report as "{outcome}" with the note "{note}"'))
def admin_closes_report(world: World, outcome: str, note: str) -> None:
    page = _admin_page(world)
    page.press(_report_row(world), "Đóng báo cáo")
    dialog = page.dialog
    expect(dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    page.choose(dialog, "Kết quả", outcome)
    page.fill_note(dialog, note)
    page.confirm(dialog)


# ── Thens: seller surfaces ───────────────────────────────────────────────
def _seller_row(world: World, which: str):
    return SellerListingsPage(world.page).product_row(_listing_title(world, which))


@then(parsers.parse('the seller list shows the {which} listing with the tag "{tag}"'))
def seller_tag(world: World, which: str, tag: str) -> None:
    row = _seller_row(world, which)
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(row).to_contain_text(tag, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the {which} listing has no tag "{tag}" in the seller list'))
@then(parsers.parse('the flagged listing has no tag "{tag}" in the seller list'))
def seller_no_tag(world: World, tag: str, which: str = "flagged") -> None:
    row = _seller_row(world, which)
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(row).not_to_contain_text(tag)


@then("the seller list shows the listing")
def seller_lists_listing(world: World) -> None:
    row = SellerListingsPage(world.page).product_row(_x(world)["listing_title"])
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)


@then("the edit page shows the listing title")
def edit_shows_title(world: World) -> None:
    expect(world.page.locator("#title")).to_have_value(
        _x(world)["listing_title"], timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the edit page shows "{text}"'))
@then(parsers.parse('the edit form shows "{text}"'))
def edit_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the edit page shows the reason "{reason}" and the note "{note}"'))
def edit_shows_reason_note(world: World, reason: str, note: str) -> None:
    expect(world.page.get_by_text(reason).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(note).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the edit page text contains no screen flag")
def edit_no_flags(world: World) -> None:
    # Positive control: the edit form itself rendered before the absence is asserted.
    expect(world.page.locator("#title")).to_be_visible(timeout=timeouts.NAVIGATION)
    text = page_text(world.page)
    assert not re.search(r"keyword:|\bai:", text), f"screen flag leaked to the seller: {text[:400]}"


@then("an admin reads a screen flag on the flagged listing through the gateway")
def admin_reads_flag(world: World) -> None:
    code, body = _status(_admin_token(world), _x(world)["flagged_id"])
    assert code == 200, f"admin GetListing {code}: {body}"
    flags = (body["listing"].get("moderation") or {}).get("screenFlags") or []
    assert flags, f"the admin sees no screen flag, so the seller-side check is vacuous: {body}"


@then("an anonymous visitor can open the clean listing page")
def anon_clean_page(world: World) -> None:
    _anonymous(world)
    _anon_page(world, _x(world)["clean_id"], _x(world)["clean_title"])


@then("an anonymous visitor can open the flagged listing page")
def anon_flagged_page(world: World) -> None:
    _anonymous(world)
    _anon_page(world, _x(world)["flagged_id"], _x(world)["flagged_title"])


@then("an anonymous visitor can open the reported listing page")
def anon_reported_page(world: World) -> None:
    _anonymous(world)
    _anon_page(world, _x(world)["listing_id"], _x(world)["listing_title"])


def _anon_page(world: World, listing_id: str, title: str) -> None:
    world.page.goto(f"{_base(world)}/listing/{listing_id}", wait_until="domcontentloaded")
    expect(world.page.locator("h1").first).to_contain_text(title, timeout=timeouts.NAVIGATION)


def _assert_not_found(listing_id: str) -> None:
    code, body = _status(None, listing_id)
    assert code == 404 and body.get("code") == "not_found", (
        f"listing visible to an anonymous reader: {code} {body}"
    )


@then("an anonymous reader gets not_found for the flagged listing")
def anon_not_found_flagged(world: World) -> None:
    _assert_not_found(_x(world)["flagged_id"])


@then("an anonymous reader gets not_found for the reported listing")
def anon_not_found_reported(world: World) -> None:
    _assert_not_found(_x(world)["listing_id"])


# ── Thens: report from the PDP ───────────────────────────────────────────
@then("the browser lands on the login page with a returnUrl for the reported listing")
def login_with_return_to_listing(world: World) -> None:
    path = f"/listing/{_x(world)['listing_id']}"
    world.page.wait_for_url(
        re.compile(r"/login\?.*returnUrl=" + re.escape(quote(path, safe="")) + r"(&|$)"),
        timeout=timeouts.NAVIGATION,
    )


@then("the browser is on the reported listing page")
def on_reported_page(world: World) -> None:
    path = f"/listing/{_x(world)['listing_id']}"
    world.page.wait_for_url(f"{_base(world)}{path}", timeout=timeouts.NAVIGATION)


@then(parsers.parse('the reported listing page shows the "{name}" button'))
def reported_page_button(world: World, name: str) -> None:
    expect(world.page.get_by_role("button", name=name).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the toast "{text}" is shown'))
def toast_shown(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the gateway lists one open scam report on the listing by the buyer")
def gateway_has_report(world: World) -> None:
    x = _x(world)
    reports = rs.reports_of(_admin_actor(world), x["listing_id"], rs.OPEN)
    assert len(reports) == 1, reports
    assert reports[0].get("reasonCode") == "scam", reports
    assert reports[0].get("reporterId") == x["buyer"]["id"], reports
    x["report_id"] = reports[0]["id"]


@then(
    parsers.parse(
        'the open reports show a row for the listing with "{reason}" and the buyer as reporter'
    )
)
def open_reports_row_reporter(world: World, reason: str) -> None:
    x = _x(world)
    row = _report_row(world)
    expect(row).to_contain_text(reason, timeout=timeouts.NAVIGATION)
    expect(row).to_contain_text(x["buyer"]["id"], timeout=timeouts.NAVIGATION)


@then("the open reports show a row for the listing")
def open_reports_row(world: World) -> None:
    expect(_report_row(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the page text hides the reporter and the report note")
def hides_reporter(world: World) -> None:
    x = _x(world)
    # Positive control: the report really exists at the gateway, so there is something to leak.
    reports = rs.reports_of(_admin_actor(world), x["listing_id"], rs.OPEN)
    assert any(r.get("reporterId") == x["buyer"]["id"] for r in reports), reports
    world.page.wait_for_load_state("networkidle")
    text = page_text(world.page)
    for secret in (x["buyer"]["id"], x["buyer"]["username"], x["report_note"]):
        assert secret not in text, f"{secret!r} leaked to the seller page"


# ── Thens: admin pages ───────────────────────────────────────────────────
@then(parsers.parse('the browser lands on the login page with a returnUrl for "{path}"'))
def login_with_return(world: World, path: str) -> None:
    world.page.wait_for_url(
        re.compile(r"/login\?.*returnUrl=" + re.escape(quote(path, safe="")) + r"(&|$)"),
        timeout=timeouts.NAVIGATION,
    )


@then("the moderation queue is shown")
def queue_shown(world: World) -> None:
    page = _admin_page(world)
    expect(page.content_or_empty).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.forbidden).to_have_count(0)


@then(parsers.parse('the moderation page shows "{text}" and no table'))
def forbidden_no_table(world: World, text: str) -> None:
    assert text == FORBIDDEN_TITLE
    page = _admin_page(world)
    expect(page.forbidden.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.table).to_have_count(0)


@then("the queue lists the flagged listing with a screen flag")
def queue_lists_flagged(world: World) -> None:
    row = _admin_page(world).find_row(_x(world)["flagged_title"])
    assert row is not None, "flagged listing is missing from the moderation queue"
    expect(row).to_contain_text(re.compile(r"(keyword|ai):"))
    expect(row).to_contain_text(_x(world)["seller"]["id"])


@then("the queue does not list the clean listing")
def queue_omits_clean(world: World) -> None:
    page = _admin_page(world)
    # Positive control first: the flagged sibling is in the same queue.
    assert page.find_row(_x(world)["flagged_title"]) is not None
    open_queue(world)
    assert page.find_row(_x(world)["clean_title"]) is None, "clean listing is in the queue"


@then("the flagged listing row leaves the queue")
def row_leaves(world: World) -> None:
    page = _admin_page(world)
    expect(page.rows_of(_x(world)["flagged_title"])).to_have_count(0, timeout=timeouts.NAVIGATION)
    open_queue(world)
    assert page.find_row(_x(world)["flagged_title"]) is None, "row came back after reload"


@then(parsers.parse('the page shows "{text}"'))
def page_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the flagged listing is still pending review at the gateway")
def still_pending(world: World) -> None:
    x = _x(world)
    code, body = _status(x["seller"]["token"], x["flagged_id"])
    assert body["listing"]["status"] == "LISTING_STATUS_PENDING_REVIEW", body


@then(parsers.parse('the browser is on "{path}"'))
def browser_on(world: World, path: str) -> None:
    world.page.wait_for_url(re.compile(re.escape(f"{_base(world)}{path}") + r"(\?.*)?$"), timeout=timeouts.NAVIGATION)


@then(parsers.parse('the "{name}" tab is selected'))
def tab_selected(world: World, name: str) -> None:
    tab = world.page.get_by_role("navigation", name="Hồ sơ cần xử lý").get_by_role(
        "link", name=name, exact=True
    )
    expect(tab).to_have_attribute("aria-current", "page", timeout=timeouts.NAVIGATION)


@then(parsers.parse('the report is listed under "{filt}" and no longer under "Đang mở"'))
def report_moved(world: World, filt: str) -> None:
    x = _x(world)
    status = {"Đã xử lý": "ACTIONED", "Đã bác bỏ": "DISMISSED"}[filt]
    page = _admin_page(world)
    base = f"{_base(world)}{ADMIN_ROUTE}?tab=reports"
    world.page.goto(f"{base}&status={status}", wait_until="domcontentloaded")
    row = page.find_row(x["listing_id"])
    assert row is not None, f"report is not listed under {filt}"
    world.page.goto(f"{base}&status=OPEN", wait_until="domcontentloaded")
    expect(page.content_or_empty).to_be_visible(timeout=timeouts.NAVIGATION)
    assert page.find_row(x["listing_id"]) is None, "report is still under Đang mở"
    stored = rs.reports_of(_admin_actor(world), x["listing_id"])
    assert [r["status"] for r in stored] == [f"REPORT_STATUS_{status}"], stored

