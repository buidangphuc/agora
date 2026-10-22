from __future__ import annotations

from playwright.sync_api import Page, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.flows.auth_flow import SESSION_COOKIE
from tests.e2e.support import dde_support as d
from tests.e2e.support import dispute_cases_ui_support as ui
from tests.e2e.support import odr_support as odr
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld
from tests.e2e.support.world import World

ESCALATE = "Khiếu nại lên Agora"
RETURNS_TAB = "Trả hàng / Hoàn tiền"
ADMIN_H2 = "Khiếu nại cần xử lý"
FORBIDDEN = "Cần tài khoản Quản trị"
CONFIRM_TEXT = (
    "Xác nhận phán quyết người mua thắng? Yêu cầu trả hàng sẽ được duyệt; "
    "nếu đơn đã thanh toán trực tuyến, người mua sẽ được hoàn tiền."
)


def world_of(world: World) -> OicWorld:
    return world.state.extra.setdefault("dcu", OicWorld())


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def sign_in(world: World, token: str) -> None:
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}]
    )


def goto(world: World, path: str) -> None:
    world.page.goto(f"{base(world)}{path}", wait_until="domcontentloaded")


def dispute_path(w: OicWorld) -> str:
    return f"/admin/cases?tab=disputes&dispute={w.data['dispute_id']}"


def open_returns_tab(page: Page) -> None:
    page.get_by_role("tab", name=RETURNS_TAB).click()


def open_order_as_buyer(world: World) -> None:
    w = world_of(world)
    sign_in(world, w.actors["buyer"].token)
    goto(world, f"/account/orders/{w.orders['order']}")
    open_returns_tab(world.page)


def detail_card(page: Page):
    card = page.locator("ul").first
    return card if card.count() else page


def choose_ruling(page: Page, label: str) -> None:
    radio = page.get_by_role("radio", name=label)
    if radio.count():
        radio.first.check()
        return
    page.get_by_label("Phán quyết").select_option(label=label)


@given("a buyer with a paid online order whose return the seller rejected")
def seeded_rejected(world: World) -> None:
    ui.seed(world_of(world), "rejected")


@given("an OPEN dispute on that rejected return")
def seeded_open(world: World) -> None:
    ui.seed(world_of(world), "open")


@given("an INVESTIGATING dispute on that rejected return")
def seeded_investigating(world: World) -> None:
    ui.seed(world_of(world), "investigating")


@given("the dispute admin is signed in the browser")
@when("the dispute admin is signed in the browser")
def admin_signed_in(world: World) -> None:
    sign_in(world, d.admin(world_of(world)).token)


@given("the dispute buyer is signed in the browser")
@when("the dispute buyer is signed in the browser")
def buyer_signed_in(world: World) -> None:
    sign_in(world, world_of(world).actors["buyer"].token)


@when("the buyer opens the returns tab of the order")
def buyer_opens_returns(world: World) -> None:
    open_order_as_buyer(world)


@then("the return card offers the escalate button")
def escalate_offered(world: World) -> None:
    expect(world.page.get_by_role("button", name=ESCALATE)).to_have_count(
        1, timeout=timeouts.DEFAULT
    )


@when(parsers.parse('the buyer escalates the return with the reason "{reason}"'))
def buyer_escalates(world: World, reason: str) -> None:
    page = world.page
    page.get_by_role("button", name=ESCALATE).click()
    page.get_by_label("Lý do khiếu nại").fill(reason)
    page.get_by_role("button", name="Gửi khiếu nại").click()


@then(parsers.parse('the order detail shows a dispute "{status}" and no escalate button'))
def dispute_shown_button_gone(world: World, status: str) -> None:
    page = world.page
    expect(page.get_by_text(status).first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("button", name=ESCALATE)).to_have_count(0)


@then("the gateway lists exactly one OPEN dispute for the order linked to the return")
def gateway_one_open(world: World) -> None:
    w = world_of(world)
    found = ui.order_disputes(w)
    assert len(found) == 1, found
    assert found[0].get("status") == d.OPEN, found
    assert found[0].get("returnId") == w.data["return_id"], found


@when("the admin opens the dispute detail by deep link")
def admin_deep_link(world: World) -> None:
    goto(world, dispute_path(world_of(world)))


@then(parsers.parse('the dispute detail shows "{text}"'))
def detail_shows(world: World, text: str) -> None:
    expect(detail_card(world.page).get_by_text(text).first).to_be_visible(timeout=timeouts.DEFAULT)


@when(parsers.parse('the admin clicks "{name}" on the dispute detail'))
def admin_clicks(world: World, name: str) -> None:
    detail_card(world.page).get_by_role("button", name=name, exact=True).click()


@then(parsers.parse('the dispute detail offers "{name}"'))
def detail_offers(world: World, name: str) -> None:
    expect(detail_card(world.page).get_by_role("button", name=name, exact=True)).to_be_visible(
        timeout=timeouts.DEFAULT
    )


@then(parsers.parse('the dispute detail offers no "{name}" button'))
def detail_offers_not(world: World, name: str) -> None:
    expect(detail_card(world.page).get_by_role("button", name=name, exact=True)).to_have_count(0)


@then(parsers.parse('the browser is on the dispute detail at "{query}"'))
def on_detail(world: World, query: str) -> None:
    w = world_of(world)
    expect(world.page).to_have_url(
        f"{base(world)}/admin/cases?{query.replace('<id>', w.data['dispute_id'])}",
        timeout=timeouts.DEFAULT,
    )


@when(parsers.parse('the admin picks the ruling "{label}"'))
def admin_picks_ruling(world: World, label: str) -> None:
    choose_ruling(world.page, label)


@then("the dispute detail shows the buyer-favour confirmation")
def shows_confirmation(world: World) -> None:
    expect(world.page.get_by_text(CONFIRM_TEXT)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the dispute reads INVESTIGATING at the gateway")
def gw_investigating(world: World) -> None:
    w = world_of(world)
    assert d.get_dispute(w, d.admin(w), w.data["dispute_id"]).get("status") == d.INVESTIGATING


@then("the dispute still reads INVESTIGATING at the gateway")
def gw_still_investigating(world: World) -> None:
    gw_investigating(world)


@then("the dispute reads OPEN at the gateway")
def gw_open(world: World) -> None:
    w = world_of(world)
    assert d.get_dispute(w, d.admin(w), w.data["dispute_id"]).get("status") == d.OPEN


@when("the buyer reloads the order after the refund settles")
def buyer_reloads_after_refund(world: World) -> None:
    w = world_of(world)
    odr.wait_return_state(w, r.REFUNDED, 60)
    open_order_as_buyer(world)


@then(parsers.parse('the returns tab shows the return as "{status}"'))
def return_text(world: World, status: str) -> None:
    expect(world.page.get_by_test_id("return-status").first).to_have_text(
        status, timeout=timeouts.DEFAULT
    )


@then(parsers.parse('the order detail dispute shows the ruling "{ruling}"'))
def dispute_ruling(world: World, ruling: str) -> None:
    expect(world.page.get_by_text(ruling).first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the order detail dispute shows "{status}"'))
def dispute_status(world: World, status: str) -> None:
    expect(world.page.get_by_text(status).first).to_be_visible(timeout=timeouts.DEFAULT)


@then("the return still reads REJECTED at the gateway after a quiet period")
def return_stays_rejected(world: World) -> None:
    odr.stays_return_state(world_of(world), r.REJECTED, 6)


@when("the buyer opens the dispute tab in the browser")
def buyer_opens_tab(world: World) -> None:
    goto(world, "/admin/cases?tab=disputes")


@then("the page shows the admin-required result and no dispute list")
def forbidden(world: World) -> None:
    page = world.page
    expect(page.get_by_text(FORBIDDEN).first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_text(ADMIN_H2)).to_have_count(0)


@when("the admin opens the dispute tab in the browser")
def admin_opens_tab(world: World) -> None:
    goto(world, "/admin/cases?tab=disputes")


@then("the dispute tab shows the dispute list heading")
def list_heading(world: World) -> None:
    expect(world.page.get_by_text(ADMIN_H2).first).to_be_visible(timeout=timeouts.DEFAULT)
