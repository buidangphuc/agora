"""Login returns the shopper to where they were (change: frontend-login-return).

Black-box: guards and the purchase buttons send an anonymous visitor to
`/login?returnUrl=<encoded path>`; after a real login through the form the browser must land on
that path (or `/` when the value is not a same-origin path). The add-to-cart / buy-now server
action runs inside Next, so "no add-to-cart call" is observed two ways: no Next server-action POST
after the landing, and the buyer's persisted cart (read through the gateway) stays empty.
"""

from __future__ import annotations

import time
from urllib.parse import quote, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService
from src.api.services.cart_service import CartService
from src.constants import PageName, timeouts
from src.models import User
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.d1_pdp_steps import _VARIANTS, _detail, _listing_id, _open, _seed
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
MAX_PATH = 2048


def _x(world: World) -> dict:
    return world.state.extra


def _enc(value: str) -> str:
    """encodeURIComponent for the characters these scenarios use."""
    return quote(value, safe="-_.!~*'()")


def _wait_url(world: World, expected: str, what: str) -> None:
    try:
        world.page.wait_for_url(lambda u: u == expected, timeout=timeouts.NAVIGATION)
    except Exception as exc:  # noqa: BLE001
        raise AssertionError(f"{what}: expected {expected}, browser is on {world.page.url}") from exc


def _cart_items(world: World) -> list[dict]:
    token = AuthService().login(_x(world)["rr_username"], _x(world)["rr_password"])
    return (CartService(token=token).get_cart().get("cart") or {}).get("items") or []


def _expected_pdp_login(world: World) -> str:
    x = _x(world)
    return f"{BASE}/login?returnUrl={_enc(x['rr_pdp_path'])}"


# ── givens ───────────────────────────────────────────────────────────────────
@given("a registered buyer for return-to-page")
def registered_buyer(world: World) -> None:
    username = fake.unique_username("return")
    AuthService().register(username, SETTINGS.seed_password, "buyer")
    _x(world).update(rr_username=username, rr_password=SETTINGS.seed_password)


@given("the buyer is signed in in the browser")
def signed_in(world: World) -> None:
    x = _x(world)
    login_via_api(world, User(x["rr_username"], x["rr_password"], "buyer"))


@given("a published listing with variants and a guest on its page with the 256GB variant selected")
def guest_on_pdp(world: World) -> None:
    _seed(world, variants=_VARIANTS)
    variant_id = _x(world)["pdp_variant_ids"]["256GB"]
    path = f"/listing/{_listing_id(world)}?variant={variant_id}"
    _x(world)["rr_pdp_path"] = path
    _open(world, query=f"?variant={variant_id}")


# ── whens ────────────────────────────────────────────────────────────────────
def _sign_in_form(world: World) -> None:
    x = _x(world)
    world.get_page(PageName.LOGIN).login(x["rr_username"], x["rr_password"])  # type: ignore[attr-defined]


@when(parsers.parse('the buyer opens the login page with returnUrl "{target}" and signs in'))
def open_login_and_sign_in(world: World, target: str) -> None:
    world.page.goto(f"{BASE}/login?returnUrl={_enc(target)}", wait_until="domcontentloaded")
    _sign_in_form(world)


@when(parsers.parse('an anonymous visitor requests "{path}"'))
def visitor_requests(world: World, path: str) -> None:
    world.page.goto(f"{BASE}{path}", wait_until="domcontentloaded")


@when(parsers.parse('an anonymous visitor requests "/chat?thread=" followed by {n:d} characters'))
def visitor_requests_overlong(world: World, n: int) -> None:
    world.page.goto(f"{BASE}/chat?thread={'a' * n}", wait_until="domcontentloaded")


@when("an anonymous visitor requests a chat path whose path plus query is exactly 2048 characters")
def visitor_requests_exact(world: World) -> None:
    prefix = "/chat?thread="
    path = prefix + "a" * (MAX_PATH - len(prefix))
    assert len(path) == MAX_PATH
    _x(world)["rr_exact_path"] = path
    world.page.goto(f"{BASE}{path}", wait_until="domcontentloaded")


@when(parsers.parse('the signed-in buyer requests "{path}"'))
def buyer_requests(world: World, path: str) -> None:
    world.page.goto(f"{BASE}{path}", wait_until="domcontentloaded")


@when("the buyer signs in on the login page that opened")
def sign_in_on_opened_login(world: World) -> None:
    try:
        world.page.wait_for_url(lambda u: urlsplit(u).path == "/login", timeout=timeouts.NAVIGATION)
    except Exception as exc:  # noqa: BLE001
        raise AssertionError(f"no login page opened, browser is on {world.page.url}") from exc
    _sign_in_form(world)


@when("the guest clicks Thêm vào giỏ")
def guest_add(world: World) -> None:
    _detail(world).add_to_cart_button.first.click()


@when("the guest clicks Mua ngay while recording page visits")
def guest_buy_now(world: World) -> None:
    visits: list[str] = []
    world.page.on(
        "request",
        lambda r: visits.append(r.url) if "/checkout" in urlsplit(r.url).path else None,
    )
    _x(world)["rr_checkout_visits"] = visits
    _detail(world).buy_now_button.first.click()


@when("the shopper clicks Thêm vào giỏ again")
def shopper_add_again(world: World) -> None:
    btn = _detail(world).add_to_cart_button.first
    expect(btn).to_be_visible(timeout=timeouts.NAVIGATION)
    btn.click()


# ── thens ────────────────────────────────────────────────────────────────────
@then(parsers.parse('the browser lands on "{path}"'))
def lands_on(world: World, path: str) -> None:
    _wait_url(world, f"{BASE}{path}", "landing")
    _start_action_recorder(world)


@then("the browser lands on the home page of the storefront and never on an external host")
def lands_home(world: World) -> None:
    _wait_url(world, f"{BASE}/", "landing")
    assert urlsplit(world.page.url).netloc == urlsplit(BASE).netloc, world.page.url


@then("the browser lands on the listing page with the selected variant")
def lands_on_pdp(world: World) -> None:
    _wait_url(world, f"{BASE}{_x(world)['rr_pdp_path']}", "landing")
    _start_action_recorder(world)


def _start_action_recorder(world: World) -> None:
    """Record Next server-action POSTs issued from now on (after the landing)."""
    actions: list[str] = []
    world.page.on(
        "request",
        lambda r: actions.append(r.url)
        if r.method == "POST" and "next-action" in {k.lower() for k in r.headers}
        else None,
    )
    _x(world)["rr_actions"] = actions


@then(parsers.parse('the visitor is on the login page with returnUrl "{login_url}"'))
def on_login_with_return(world: World, login_url: str) -> None:
    _wait_url(world, f"{BASE}{login_url}", "guard redirect")


@then("the visitor is on the plain login page without a returnUrl")
def on_plain_login(world: World) -> None:
    _wait_url(world, f"{BASE}/login", "over-length fallback")


@then("the visitor is on the login page whose returnUrl is that same path")
def on_login_exact(world: World) -> None:
    _wait_url(world, f"{BASE}/login?returnUrl={_enc(_x(world)['rr_exact_path'])}", "boundary path")


@then("the guest is on the login page for the listing with the selected variant")
def on_pdp_login(world: World) -> None:
    _wait_url(world, _expected_pdp_login(world), "purchase button redirect")


@then(parsers.parse('the page shows "{text}"'))
def page_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the browser is still on "{path}"'))
def still_on(world: World, path: str) -> None:
    assert world.page.url == f"{BASE}{path}", world.page.url


@then("no add-to-cart call was issued after login")
def no_replay(world: World) -> None:
    world.page.wait_for_load_state("networkidle")
    time.sleep(3)  # a replay would fire right after hydration; give it a window to show
    assert _x(world)["rr_actions"] == [], _x(world)["rr_actions"]


@then("checkout was never visited")
def no_checkout(world: World) -> None:
    assert _x(world)["rr_checkout_visits"] == [], _x(world)["rr_checkout_visits"]
    assert "/checkout" not in urlsplit(world.page.url).path


@then("the buyer's cart is empty")
def cart_empty(world: World) -> None:
    assert _cart_items(world) == []


@then("the buyer's cart holds the listing")
def cart_holds(world: World) -> None:
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        items = _cart_items(world)
        if any(i.get("listingId") == _listing_id(world) for i in items):
            return
        time.sleep(1)
    raise AssertionError(f"listing not in cart: {_cart_items(world)}")
