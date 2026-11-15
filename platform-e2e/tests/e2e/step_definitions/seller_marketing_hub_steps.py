"""Steps for seller/seller_marketing_hub.feature (change seller-marketing-hub).

Fixtures are created through the gateway as the owning seller; everything the change promises is
asserted on the rendered hub plus the stored state read back through the gateway. Fresh accounts
per scenario. Server-side RPC arguments are not visible to a browser, so request-shape claims of
the spec stay with the unit tests; here the observable outcome is asserted.
"""

from __future__ import annotations

import re
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages import SellerMarketingPage
from src.pages.seller_marketing_page import (
    CAMPAIGN_STATES,
    FLASH_TAB,
    VOUCHER_TAB,
)
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import audit_support as aud
from tests.e2e.support import seller_marketing_hub_support as sh
from tests.e2e.support.world import World

LIST_WITHIN_MS = 3_000
FLASH_URL = "?tab=flashsale"


def _s(world: World) -> dict:
    return sh.state(world)


def _page(world: World) -> SellerMarketingPage:
    return SellerMarketingPage(world.page)


def _open(world: World, query: str = "") -> None:
    page = _page(world)
    page.goto(f"{page.app_base_url()}{page.path}{query}")


def _seller(world: World) -> None:
    st = _s(world)
    if "token" not in st:
        st["token"], st["id"] = sh.register(world, "seller", "smh_a")


def _other(world: World) -> None:
    st = _s(world)
    if "b_token" not in st:
        st["b_token"], st["b_id"] = sh.register(world, "seller", "smh_b")


def _listing(world: World, who: str = "token") -> tuple[str, str]:
    return sh.make_listing(world, _s(world)[who], "camp")


def _campaign(world: World, who: str, **kw) -> str:
    st = _s(world)
    listing_id, title = sh.make_listing(world, st[who], "camp")
    campaign_id = ac.seller_campaign(st[who], listing_id, **kw)
    st.setdefault("campaigns", {})[campaign_id] = {"listing": listing_id, "title": title}
    return campaign_id


_SCHEDULED = {"starts_in_s": 86400 * 2, "ends_in_s": 86400 * 4}


# ── Givens: accounts ─────────────────────────────────────────────────────
@given("a seller is signed in the seller app")
def seller_signed_in(world: World) -> None:
    _seller(world)
    sh.sign_in(world, _s(world)["token"])


@given("a seller is signed in the buyer app")
def seller_signed_in_buyer_app(world: World) -> None:
    seller_signed_in(world)


@given("a buyer is signed in the seller app")
def buyer_signed_in(world: World) -> None:
    st = _s(world)
    st["buyer_token"], st["buyer_id"] = sh.register(world, "buyer", "smh_buyer")
    sh.sign_in(world, st["buyer_token"])


@given("a buyer who holds a voucher is signed in the buyer app")
def buyer_with_voucher(world: World) -> None:
    buyer_signed_in(world)
    st = _s(world)
    st["held_code"] = sh.wait_for_my_voucher_code(st["buyer_token"])


@given("an anonymous visitor")
def anonymous(world: World) -> None:
    world.context.clear_cookies()


# ── Givens: vouchers ─────────────────────────────────────────────────────
@given("the seller owns an active, a scheduled, an ended and a deactivated shop voucher")
def four_states(world: World) -> None:
    st = _s(world)
    token = st["token"]
    codes = {}
    codes["active"], _ = sh.shop_voucher(token, "SMHA")
    codes["scheduled"], _ = sh.shop_voucher(
        token, "SMHS", starts_at=av.iso(days=1), ends_at=av.iso(days=30)
    )
    codes["ended"], _ = sh.shop_voucher(token, "SMHE")
    av.backdate_end(codes["ended"])
    codes["deactivated"], dead_id = sh.shop_voucher(token, "SMHD")
    resp = av.gw(token, "DeactivateMyShopVoucher", {"voucherId": dead_id})
    assert resp.status_code == 200, f"seeding a deactivated voucher failed: {resp.text}"
    st["codes"] = codes


@given("the seller owns an active shop voucher")
def one_active(world: World) -> None:
    st = _s(world)
    st["code"], st["voucher_id"] = sh.shop_voucher(st["token"], "SMHO")
    st["before_ids"] = {v["id"] for v in sh.my_vouchers(st["token"])}


@given("another seller owns a shop voucher and a platform voucher exists")
def other_voucher_and_platform(world: World) -> None:
    other_voucher(world)
    st = _s(world)
    admin = aud.admin_token(world)
    st["platform_code"] = av.fresh_code("SMHP")
    av.create_voucher(admin, st["platform_code"], scope=av.PLATFORM)


@given("another seller owns a shop voucher")
def other_voucher(world: World) -> None:
    _other(world)
    st = _s(world)
    st["b_code"], st["b_voucher_id"] = sh.shop_voucher(st["b_token"], "SMHB")


@given("the seller owns 21 shop vouchers")
def twenty_one(world: World) -> None:
    st = _s(world)
    st["prefix"] = f"PG{uuid.uuid4().hex[:6].upper()}"
    for i in range(21):
        av.create_voucher(st["token"], f"{st['prefix']}{i:02d}", scope=av.SHOP)


@given("a buyer can preview that voucher")
def preview_control(world: World) -> None:
    st = _s(world)
    st["buyer_token"], st["buyer_id"] = sh.register(world, "buyer", "smh_buyer")
    assert sh.preview_valid(
        st["buyer_token"], st["buyer_id"], st["id"], st["code"]
    ), "positive control: the active voucher is not previewable"


@given("a public platform voucher exists")
def public_platform(world: World) -> None:
    st = _s(world)
    st["platform_code"] = av.fresh_code("SMHP")
    av.create_voucher(aud.admin_token(world), st["platform_code"], scope=av.PLATFORM)


# ── Givens: campaigns ────────────────────────────────────────────────────
@given("the seller owns a published listing")
def published_listing(world: World) -> None:
    st = _s(world)
    st["listing"], st["listing_title"] = _listing(world)


@given(parsers.parse('the seller owns published listings "ao" and "quan" with distinct titles'))
def two_listings(world: World) -> None:
    st = _s(world)
    st["tok"] = uuid.uuid4().hex[:8]
    st["ao_listing"], st["ao_title"] = sh.make_listing(
        world, st["token"], "ao", f"[E2E] ao-{st['tok']} thun"
    )
    st["quan_listing"], st["quan_title"] = sh.make_listing(
        world, st["token"], "quan", f"[E2E] quan-{st['tok']} jean"
    )


@given("the seller owns a scheduled and a cancelled campaign on separate listings")
def scheduled_and_cancelled(world: World) -> None:
    st = _s(world)
    st["sched"] = _campaign(world, "token", **_SCHEDULED)
    st["cancelled"] = _campaign(world, "token", **_SCHEDULED)
    resp = ac.gw(st["token"], "CancelMyCampaign", {"campaignId": st["cancelled"]})
    assert ac.campaign_of(resp)["status"] == ac.CANCELLED


@given("the seller owns a scheduled campaign")
def one_scheduled(world: World) -> None:
    st = _s(world)
    st["campaign"] = _campaign(world, "token", **_SCHEDULED)


@given("the seller owns a scheduled, an active, an ended and a cancelled campaign")
def four_campaigns(world: World) -> None:
    st = _s(world)
    st["sched"] = _campaign(world, "token", **_SCHEDULED)
    st["active"] = _campaign(world, "token")
    st["ended"] = _campaign(world, "token")
    ac.end_now(st["ended"])
    st["cancelled"] = _campaign(world, "token", **_SCHEDULED)
    resp = ac.gw(st["token"], "CancelMyCampaign", {"campaignId": st["cancelled"]})
    assert ac.campaign_of(resp)["status"] == ac.CANCELLED


@given("another seller owns a campaign")
def other_campaign(world: World) -> None:
    _other(world)
    st = _s(world)
    st["b_campaign"] = _campaign(world, "b_token")


@given("another seller owns a scheduled campaign")
def other_scheduled(world: World) -> None:
    _other(world)
    st = _s(world)
    st["b_campaign"] = _campaign(world, "b_token", **_SCHEDULED)


@given("the seller is on the seller centre")
def on_centre(world: World) -> None:
    page = _page(world)
    page.goto(f"{page.app_base_url()}/seller")


# ── Whens ────────────────────────────────────────────────────────────────
@when("the seller opens the marketing hub")
@when("the visitor opens the marketing hub")
def opens_hub(world: World) -> None:
    _open(world)


@when(parsers.parse('the seller opens the "Flash sale" tab of the marketing hub'))
def opens_flash(world: World) -> None:
    _open(world, FLASH_URL)


@when(parsers.parse('the seller clicks "{name}" in the seller navigation'))
def clicks_nav(world: World, name: str) -> None:
    link = _page(world).nav_link
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()


@when(parsers.parse('the seller selects the "{label}" tab'))
def selects_tab(world: World, label: str) -> None:
    tab = _page(world).tab(label).first
    _page(world).wait_until_interactive(tab)
    tab.click()
    world.page.wait_for_url(re.compile(r"tab=flashsale"), timeout=timeouts.NAVIGATION)


@when("the seller reloads the page")
def reloads(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@when("the seller submits a new voucher with PERCENT 10, quota 100 and a window starting tomorrow")
def submits_voucher(world: World) -> None:
    st = _s(world)
    st["new_code"] = av.fresh_code("GIAM")
    page = _page(world)
    page.fill_voucher_form(st["new_code"], 100, av.iso(days=1), av.iso(days=30))
    page.submit_voucher()


@when("the seller submits a new voucher reusing that code")
def submits_duplicate(world: World) -> None:
    st = _s(world)
    page = _page(world)
    page.fill_voucher_form(st["code"], 100, av.iso(days=1), av.iso(days=30))
    page.submit_voucher()


@when(parsers.parse('the seller confirms "Tắt voucher" on that voucher'))
def confirms_deactivate(world: World) -> None:
    page = _page(world)
    row = page.row(_s(world)["code"]).first
    page.press(row, re.compile(r"^tắt voucher$", re.I))
    page.confirm()


@when("the seller deactivates the other seller's voucher id through the gateway")
def deactivates_foreign(world: World) -> None:
    st = _s(world)
    st["resp"] = av.gw(st["token"], "DeactivateMyShopVoucher", {"voucherId": st["b_voucher_id"]})


@when("the seller cancels the other seller's campaign id through the gateway")
def cancels_foreign(world: World) -> None:
    st = _s(world)
    st["resp"] = ac.gw(st["token"], "CancelMyCampaign", {"campaignId": st["b_campaign"]})


@when(
    "the seller creates a flash sale for that listing with sale price 50000, cap 10 and a future window"
)
def creates_campaign(world: World) -> None:
    st = _s(world)
    page = _page(world)
    cta = world.page.get_by_role("button", name=re.compile(r"tạo flash sale", re.I)).or_(
        world.page.get_by_role("link", name=re.compile(r"tạo flash sale", re.I))
    )
    if cta.count():
        cta.first.click()
    page.field(r"sản phẩm|listing").fill(st["listing_title"])
    page.picker_options.filter(has_text=st["listing_title"]).first.click()
    page.field(r"giá").fill(str(sh.SALE_PRICE_FORM))
    page.field(r"giới hạn|số lượng|tồn kho|cap").fill("10")
    page.field(r"bắt đầu|start").fill(av.iso(days=1)[:16])
    page.field(r"kết thúc|end").fill(av.iso(days=5)[:16])
    page.submit_campaign()


@when('the seller types the "ao" title token in the listing picker')
def types_picker(world: World) -> None:
    st = _s(world)
    page = _page(world)
    cta = world.page.get_by_role("button", name=re.compile(r"tạo flash sale", re.I)).or_(
        world.page.get_by_role("link", name=re.compile(r"tạo flash sale", re.I))
    )
    if cta.count():
        cta.first.click()
    page.field(r"sản phẩm|listing").fill(f"ao-{st['tok']}")


@when(parsers.parse('the seller selects the status filter "{label}"'))
def selects_filter(world: World, label: str) -> None:
    pg = world.page
    control = (
        pg.get_by_role("tab", name=label)
        .or_(pg.get_by_role("link", name=label, exact=True))
        .or_(pg.get_by_role("button", name=label, exact=True))
        .or_(pg.get_by_role("radio", name=label))
    )
    if control.count() == 0:
        pg.get_by_role("combobox").first.select_option(label=label)
    else:
        control.first.click()
    pg.wait_for_url(re.compile(r"status="), timeout=timeouts.NAVIGATION)


@when(parsers.parse('the seller confirms "Huỷ" on that campaign'))
def confirms_cancel(world: World) -> None:
    st = _s(world)
    page = _page(world)
    row = page.row(st["campaigns"][st["campaign"]]["title"]).first
    page.press(row, re.compile(r"^huỷ$", re.I))
    page.confirm()


@when("the campaign starts while the page is open")
def campaign_starts(world: World) -> None:
    sh.start_now(_s(world)["campaign"])


# ── Thens: access and navigation ─────────────────────────────────────────
@then("the browser is on the marketing hub")
def on_hub(world: World) -> None:
    expect(world.page).to_have_url(
        re.compile(r".*/seller/marketing(\?.*)?$"), timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the "{label}" tab is selected'))
def tab_selected(world: World, label: str) -> None:
    expect(_page(world).selected_tab(label).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the URL contains "{fragment}"'))
def url_contains(world: World, fragment: str) -> None:
    expect(world.page).to_have_url(re.compile(re.escape(fragment)), timeout=timeouts.NAVIGATION)


@then("the seller onboarding screen is shown instead of the hub")
def onboarding_view(world: World) -> None:
    expect(world.page.get_by_role("button", name="Đồng ý và trở thành Người Bán")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("no marketing tab content is shown")
def no_tab_content(world: World) -> None:
    page = _page(world)
    expect(page.tab(VOUCHER_TAB)).to_have_count(0)
    expect(page.tab(FLASH_TAB)).to_have_count(0)


@then("the visitor lands on the login page")
def login_page(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/login(\?.*)?$"), timeout=timeouts.NAVIGATION)


# ── Thens: vouchers ──────────────────────────────────────────────────────
@then(
    "four voucher rows show the labels Đang chạy, Sắp diễn ra, Đã kết thúc and Đã tắt with used over quota"
)
def four_rows(world: World) -> None:
    st = _s(world)
    page = _page(world)
    labels = {
        "active": sh.LABEL_ACTIVE,
        "scheduled": sh.LABEL_SCHEDULED,
        "ended": sh.LABEL_ENDED,
        "deactivated": sh.LABEL_DEACTIVATED,
    }
    for key, label in labels.items():
        row = page.row(st["codes"][key])
        expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
        expect(row).to_contain_text(label)
        expect(row).to_contain_text("0/100")


@then("the seller's own voucher row is shown")
def own_voucher_row(world: World) -> None:
    expect(_page(world).row(_s(world)["code"])).to_have_count(1, timeout=timeouts.NAVIGATION)


@then("no row for the other seller's or the platform voucher is shown")
def no_foreign_rows(world: World) -> None:
    st = _s(world)
    page = _page(world)
    expect(page.row(st["b_code"])).to_have_count(0)
    expect(page.row(st["platform_code"])).to_have_count(0)


@then("a row for the new voucher with label Sắp diễn ra appears in the list")
def new_voucher_row(world: World) -> None:
    row = _page(world).row(_s(world)["new_code"])
    expect(row).to_have_count(1, timeout=LIST_WITHIN_MS)
    expect(row).to_contain_text(sh.LABEL_SCHEDULED)


@then(
    "the seller's stored vouchers hold exactly one shop-scope voucher with that code and quota 100"
)
def stored_new_voucher(world: World) -> None:
    st = _s(world)
    mine = [v for v in sh.my_vouchers(st["token"]) if v.get("code") == st["new_code"]]
    assert len(mine) == 1, mine
    assert mine[0].get("scope") == av.SHOP and str(mine[0].get("quota")) == "100", mine
    assert mine[0].get("sellerId") == st["id"], mine


@then(parsers.parse('the error "{message}" is shown'))
def error_shown(world: World, message: str) -> None:
    expect(_page(world).text(message)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the voucher form keeps the typed code")
def form_keeps_code(world: World) -> None:
    expect(_page(world).field(r"^(mã voucher|code)")).to_have_value(_s(world)["code"])


@then("the seller's stored vouchers are unchanged")
def vouchers_unchanged(world: World) -> None:
    st = _s(world)
    assert {v["id"] for v in sh.my_vouchers(st["token"])} == st["before_ids"]


@then("that voucher row shows Đã tắt")
def voucher_row_off(world: World) -> None:
    row = _page(world).row(_s(world)["code"])
    expect(row).to_contain_text(sh.LABEL_DEACTIVATED, timeout=LIST_WITHIN_MS)


@then("the stored voucher is deactivated")
def stored_deactivated(world: World) -> None:
    st = _s(world)
    mine = sh.by_id(sh.my_vouchers(st["token"]), st["voucher_id"])
    assert mine and not mine.get("active") and mine.get("deactivatedAt"), mine


@then("a buyer can no longer preview that voucher")
def preview_gone(world: World) -> None:
    st = _s(world)
    assert not sh.preview_valid(st["buyer_token"], st["buyer_id"], st["id"], st["code"])


@then(parsers.parse('the gateway answers "{code}"'))
def gateway_answers(world: World, code: str) -> None:
    resp = _s(world)["resp"]
    assert av.code_of(resp) == code, f"want {code}: {resp.status_code} {resp.text}"


@then("the other seller's voucher is still active")
def other_voucher_active(world: World) -> None:
    st = _s(world)
    mine = sh.by_id(sh.my_vouchers(st["b_token"]), st["b_voucher_id"])
    assert mine and mine.get("active") and not mine.get("deactivatedAt"), mine


# ── Thens: campaigns ─────────────────────────────────────────────────────
def _title(world: World, campaign_id: str) -> str:
    return _s(world)["campaigns"][campaign_id]["title"]


@then("a campaign row for that listing with label Sắp diễn ra appears in the list")
def new_campaign_row(world: World) -> None:
    row = _page(world).row(_s(world)["listing_title"])
    expect(row).to_have_count(1, timeout=LIST_WITHIN_MS)
    expect(row).to_contain_text(CAMPAIGN_STATES["SCHEDULED"])


@then(
    "the seller's stored campaigns hold exactly one scheduled campaign for that listing with price 50000 and cap 10"
)
def stored_new_campaign(world: World) -> None:
    st = _s(world)
    mine = [c for c in sh.my_campaigns(st["token"]) if c.get("listingId") == st["listing"]]
    assert len(mine) == 1, mine
    assert mine[0]["status"] == ac.SCHEDULED, mine
    assert str(mine[0].get("salePrice")) == str(sh.SALE_PRICE_FORM), mine
    assert str(mine[0].get("stockCap")) == "10", mine


@then('the picker offers the "ao" listing and not the "quan" listing')
def picker_options(world: World) -> None:
    st = _s(world)
    options = _page(world).picker_options
    expect(options.filter(has_text=st["ao_title"])).to_have_count(1, timeout=timeouts.DEFAULT)
    expect(options.filter(has_text=st["quan_title"])).to_have_count(0)


@then("the picker offers at most 20 options")
def picker_cap(world: World) -> None:
    assert _page(world).picker_options.count() <= 20


@then("only the cancelled campaign row is shown")
def only_cancelled(world: World) -> None:
    st = _s(world)
    page = _page(world)
    expect(page.row(_title(world, st["cancelled"]))).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(page.row(_title(world, st["sched"]))).to_have_count(0)


@then("that campaign row shows Đã huỷ")
def campaign_row_cancelled(world: World) -> None:
    st = _s(world)
    row = _page(world).row(_title(world, st["campaign"]))
    expect(row).to_contain_text(CAMPAIGN_STATES["CANCELLED"], timeout=LIST_WITHIN_MS)


@then("the stored campaign is cancelled")
def stored_cancelled(world: World) -> None:
    st = _s(world)
    mine = sh.by_id(sh.my_campaigns(st["token"]), st["campaign"])
    assert mine and mine["status"] == ac.CANCELLED, mine


@then("the listing of the campaign has no active flash sale")
def no_active_sale(world: World) -> None:
    st = _s(world)
    listing = st["campaigns"][st["campaign"]]["listing"]
    resp = ac.gw(st["token"], "GetActiveFlashSale", {"listingId": listing})
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert not resp.json().get("active"), f"flash price still active: {resp.text}"


_CANCEL = re.compile(r"^huỷ$", re.I)


def _cancel_controls(world: World, campaign_id: str):
    row = _page(world).row(_title(world, campaign_id))
    return row, row.get_by_role("button", name=_CANCEL).or_(row.get_by_role("link", name=_CANCEL))


@then(parsers.parse('the scheduled campaign row has a "Huỷ" control'))
def sched_has_cancel(world: World) -> None:
    row, control = _cancel_controls(world, _s(world)["sched"])
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(control).to_have_count(1)


@then(parsers.parse('the active, ended and cancelled campaign rows have no "Huỷ" control'))
def others_no_cancel(world: World) -> None:
    st = _s(world)
    for key, label in (("active", "ACTIVE"), ("ended", "ENDED"), ("cancelled", "CANCELLED")):
        row, control = _cancel_controls(world, st[key])
        expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
        expect(row).to_contain_text(CAMPAIGN_STATES[label])
        expect(control).to_have_count(0)


@then(parsers.parse('the message "{message}" is shown'))
def message_shown(world: World, message: str) -> None:
    expect(_page(world).text(message)).to_be_visible(timeout=timeouts.DEFAULT)


@then("that campaign row shows Đang diễn ra")
def campaign_row_active(world: World) -> None:
    st = _s(world)
    row = _page(world).row(_title(world, st["campaign"]))
    expect(row).to_contain_text(CAMPAIGN_STATES["ACTIVE"], timeout=timeouts.DEFAULT)


@then("the stored campaign is active")
def stored_active(world: World) -> None:
    st = _s(world)
    mine = sh.by_id(sh.my_campaigns(st["token"]), st["campaign"])
    assert mine and mine["status"] == ac.ACTIVE, mine


@then("the seller's own campaign row is shown")
def own_campaign_row(world: World) -> None:
    st = _s(world)
    expect(_page(world).row(_title(world, st["campaign"]))).to_have_count(
        1, timeout=timeouts.NAVIGATION
    )


@then("no row for the other seller's campaign is shown")
def no_foreign_campaign(world: World) -> None:
    st = _s(world)
    expect(_page(world).row(_title(world, st["b_campaign"]))).to_have_count(0)


@then("the other seller's campaign is still scheduled")
def other_still_scheduled(world: World) -> None:
    st = _s(world)
    mine = sh.by_id(sh.my_campaigns(st["b_token"]), st["b_campaign"])
    assert mine and mine["status"] == ac.SCHEDULED and not mine.get("cancelledAt"), mine


# ── Buyer app ────────────────────────────────────────────────────────────
@when("the visitor opens the buyer voucher page")
def opens_buyer_vouchers(world: World) -> None:
    world.page.goto(
        f"{world.settings.base_url.rstrip('/')}/vouchers", wait_until="domcontentloaded"
    )


@then("the voucher catalogue shows the platform voucher")
def catalogue_shows(world: World) -> None:
    code = _s(world)["platform_code"]
    expect(world.page.get_by_text(code).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("no create-voucher form is rendered")
def no_create_form(world: World) -> None:
    pg = world.page
    expect(pg.get_by_label("Create voucher")).to_have_count(0)
    expect(
        pg.get_by_role("button", name=re.compile(r"create voucher|tạo voucher", re.I))
    ).to_have_count(0)
    expect(pg.get_by_label(re.compile(r"^(mã voucher|code)$", re.I))).to_have_count(0)


@then("the held voucher appears on the page")
def held_voucher(world: World) -> None:
    code = _s(world)["held_code"]
    expect(world.page.get_by_text(code).first).to_be_visible(timeout=timeouts.NAVIGATION)


# ── Paging and empty states ──────────────────────────────────────────────
@then("20 voucher rows are shown with a next-page control")
def twenty_rows(world: World) -> None:
    page = _page(world)
    rows = page.rows_with(_s(world)["prefix"])
    expect(rows).to_have_count(20, timeout=timeouts.NAVIGATION)
    expect(page.next_page.first).to_be_visible()


@then("the next page shows 1 voucher row")
def next_page_one(world: World) -> None:
    page = _page(world)
    page.next_page.first.click()
    rows = page.rows_with(_s(world)["prefix"])
    expect(rows).to_have_count(1, timeout=timeouts.NAVIGATION)


def _empty_state(world: World, cta: str) -> None:
    pg = world.page
    control = pg.get_by_role("button", name=re.compile(cta, re.I)).or_(
        pg.get_by_role("link", name=re.compile(cta, re.I))
    )
    expect(control.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_page(world).alerts).to_have_count(0)


@then(
    parsers.parse('the voucher tab shows an empty state with a "{cta}" call to action and no error')
)
def voucher_empty(world: World, cta: str) -> None:
    _empty_state(world, cta)


@then(
    parsers.parse(
        'the flash sale tab shows an empty state with a "{cta}" call to action and no error'
    )
)
def flash_empty(world: World, cta: str) -> None:
    _empty_state(world, cta)
