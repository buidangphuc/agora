"""Steps for frontend/referrer_hygiene.feature (change: frontend-referrer-hygiene).

Black-box: response headers via httpx, the rendered <meta name="referrer"> and window.dataLayer via
the browser, the outbound Referer through a real cross-origin navigation intercepted at the
browser context, and the guest redirect through the browser URL. Google hosts are mocked
(ga4_datalayer_steps._start_recording); the cross-origin host is a reserved .invalid name that is
fulfilled locally, so nothing leaves the machine.
"""

from __future__ import annotations

import re
from urllib.parse import urlsplit

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService
from src.constants import timeouts
from src.models import User
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.ga4_datalayer_steps import _start_recording
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
ADMIN_BASE = SETTINGS.admin_base_url.rstrip("/")


def _host(path: str) -> str:
    return ADMIN_BASE if path.startswith("/admin") else BASE
POLICY = "strict-origin-when-cross-origin"
WEAKER = {
    "unsafe-url",
    "no-referrer-when-downgrade",
    "origin-when-cross-origin",
    "same-origin",
    "origin",
    "",
}
ADMIN_PAGES = ("cockpit", "users", "audit", "listings", "cases")
XORIGIN = "http://xorigin.e2e.invalid"


def _x(world: World) -> dict:
    return world.state.extra


def _meta(world: World) -> list[str | None]:
    loc = world.page.locator('head meta[name="referrer"]')
    return [loc.nth(i).get_attribute("content") for i in range(loc.count())]


def _assert_meta(world: World, what: str) -> None:
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    assert _meta(world) == ["no-referrer"], f"{what}: meta referrer tags = {_meta(world)} at {world.page.url}"


# ── headers ──────────────────────────────────────────────────────────────────
@when(parsers.parse('the storefront is requested at "{a}", "{b}" and "{c}" without following redirects'))
def request_paths(world: World, a: str, b: str, c: str) -> None:
    responses = {}
    with httpx.Client(follow_redirects=False, timeout=30) as client:
        for path in (a, b, c):
            responses[path] = client.get(f"{_host(path)}{path}")
    _x(world)["rh_responses"] = responses


@then(parsers.parse('every response has the header Referrer-Policy "{value}"'))
def every_has_policy(world: World, value: str) -> None:
    for path, r in _x(world)["rh_responses"].items():
        assert r.headers.get("referrer-policy") == value, (
            f"{path} (HTTP {r.status_code}): Referrer-Policy = {r.headers.get('referrer-policy')!r}"
        )


@then("every response still has a Content-Security-Policy header")
def every_has_csp(world: World) -> None:
    for path, r in _x(world)["rh_responses"].items():
        assert r.headers.get("content-security-policy"), f"{path}: no Content-Security-Policy header"


@then("each response has exactly one Referrer-Policy value and it is not a weaker policy")
def one_not_weaker(world: World) -> None:
    for path, r in _x(world)["rh_responses"].items():
        values = r.headers.get_list("referrer-policy")
        assert len(values) == 1, f"{path}: Referrer-Policy header values = {values}"
        tokens = {t.strip().lower() for t in values[0].split(",")}
        assert tokens == {POLICY}, f"{path}: Referrer-Policy = {values[0]!r}"
        assert not (tokens & WEAKER), f"{path}: weaker policy {tokens & WEAKER}"


# ── meta referrer ────────────────────────────────────────────────────────────
@when(parsers.parse('the admin opens "{path}"'))
def admin_opens(world: World, path: str) -> None:
    world.page.goto(f"{_host(path)}{path}", wait_until="domcontentloaded")
    _x(world)["rh_path"] = path


@then("the page is the audit viewer")
def is_audit_viewer(world: World) -> None:
    expect(world.page.get_by_role("heading", name="Nhật ký kiểm toán")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the document head contains the meta referrer "{value}"'))
def head_has_meta(world: World, value: str) -> None:
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    assert _meta(world) == [value], f"meta referrer tags = {_meta(world)} at {world.page.url}"


@when("the admin opens each of the admin pages cockpit, users, audit, listings and cases")
def admin_opens_all(world: World) -> None:
    seen: dict[str, list] = {}
    for name in ADMIN_PAGES:
        world.page.goto(f"{ADMIN_BASE}/admin/{name}", wait_until="load")
        # positive control: the admin is on the page, not bounced to login / 404
        assert urlsplit(world.page.url).path == f"/admin/{name}", world.page.url
        seen[name] = _meta(world)
    _x(world)["rh_admin_meta"] = seen


@then(parsers.parse('every admin page contains the meta referrer "{value}"'))
def all_admin_meta(world: World, value: str) -> None:
    bad = {n: m for n, m in _x(world)["rh_admin_meta"].items() if m != [value]}
    assert not bad, f"admin pages without exactly one meta referrer={value}: {bad}"


@when(parsers.parse('an anonymous visitor opens "{path}" in a fresh browser context'))
def anonymous_opens(world: World, path: str) -> None:
    world.context.clear_cookies()
    world.page.goto(f"{_host(path)}{path}", wait_until="load")
    assert urlsplit(world.page.url).path == path, world.page.url


# ── 403 view ─────────────────────────────────────────────────────────────────
@given("a registered buyer for referrer hygiene")
def registered_buyer(world: World) -> None:
    username = fake.unique_username("refhyg")
    AuthService().register(username, SETTINGS.seed_password, "buyer")
    _x(world).update(rh_user=username, rh_pass=SETTINGS.seed_password)


@given("the referrer-hygiene buyer is signed in in the browser")
def buyer_signed_in(world: World) -> None:
    x = _x(world)
    login_via_api(world, User(x["rh_user"], x["rh_pass"], "buyer"))


@when(parsers.parse('the admin page "{path}" is opened by the signed-in buyer'))
def buyer_opens_admin(world: World, path: str) -> None:
    world.page.goto(f"{_host(path)}{path}", wait_until="load")


@then(parsers.parse('the 403 view "{text}" is rendered without a redirect to login'))
def forbidden_view(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    assert urlsplit(world.page.url).path == "/admin/users", world.page.url


# ── outbound Referer ─────────────────────────────────────────────────────────
_OPEN_XORIGIN = """
(url) => {
  const a = document.createElement("a");
  a.href = url; a.target = "_blank"; a.textContent = "x";
  document.body.appendChild(a);
  a.click();
}
"""


def _open_and_probe(world: World, path: str) -> None:
    seen: list[dict] = []

    def handle(route) -> None:  # noqa: ANN001
        seen.append(route.request.all_headers())
        route.fulfill(status=200, content_type="text/html", body="<html>ok</html>")

    world.context.route(re.compile(r"^http://xorigin\.e2e\.invalid/"), handle)
    world.page.goto(f"{BASE}{path}", wait_until="load")
    assert urlsplit(world.page.url).path == urlsplit(path).path, world.page.url
    world.page.evaluate(_OPEN_XORIGIN, f"{XORIGIN}/probe")
    deadline = timeouts.NAVIGATION / 1000
    import time

    end = time.monotonic() + deadline
    while not seen and time.monotonic() < end:
        world.page.wait_for_timeout(100)
    _x(world)["rh_xorigin"] = seen


@when(parsers.parse('the admin opens "{path}" and the page issues a cross-origin request'))
def admin_cross_origin(world: World, path: str) -> None:
    _open_and_probe(world, path)


@then("the cross-origin request was observed")
def xorigin_observed(world: World) -> None:
    assert _x(world)["rh_xorigin"], "no cross-origin request reached the interceptor"


@then("that request carries no Referer header")
def no_referer(world: World) -> None:
    headers = _x(world)["rh_xorigin"][-1]
    assert "referer" not in headers, f"Referer leaked: {headers.get('referer')!r}"


@then("that request carries a Referer header that is only the site origin")
def origin_only_referer(world: World) -> None:
    headers = _x(world)["rh_xorigin"][-1]
    assert headers.get("referer") == f"{BASE}/", f"Referer = {headers.get('referer')!r}"


# ── dataLayer ────────────────────────────────────────────────────────────────
_LAYER = "() => Array.from(window.dataLayer || [], (e) => JSON.parse(JSON.stringify(e)))"


def _walk(node, out: list[tuple[str, str]]) -> None:
    if isinstance(node, dict):
        for k, v in node.items():
            if k in ("page_location", "page_referrer") and isinstance(v, str):
                out.append((k, v))
            _walk(v, out)
    elif isinstance(node, list):
        for v in node:
            _walk(v, out)


def _collect(world: World, must_contain: str) -> None:
    page = world.page
    page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    found: list[tuple[str, str]] = []

    def have() -> bool:
        found.clear()
        _walk(page.evaluate(_LAYER), found)
        return any(k == "page_location" and must_contain in v for k, v in found)

    end = timeouts.LONG / 1000
    import time

    deadline = time.monotonic() + end
    while not have() and time.monotonic() < deadline:
        page.wait_for_timeout(200)
    _x(world).setdefault("rh_pushed", []).extend(found)


@when(
    "the admin opens the audit page with a target query and then navigates to the users search "
    "with an email query while recording the dataLayer"
)
def admin_visits_two(world: World) -> None:
    _start_recording(world)
    world.page.goto(f"{ADMIN_BASE}/admin/audit?target_id=u-1&actor_id=u-2", wait_until="domcontentloaded")
    _collect(world, "/admin/audit")
    # full document navigation so document.referrer is the audit URL (as the browser reports it)
    world.page.evaluate("(u) => { window.location.href = u; }", f"{ADMIN_BASE}/admin/users?query=a%40b.vn")
    world.page.wait_for_url(lambda u: urlsplit(u).path == "/admin/users", timeout=timeouts.NAVIGATION)
    _collect(world, "/admin/users")


@then("page_location entries were pushed for the audit page and for the users page")
def pushed_both(world: World) -> None:
    pushed = _x(world)["rh_pushed"]
    locs = [v for k, v in pushed if k == "page_location"]
    assert any("/admin/audit" in v for v in locs), f"no audit page_location in {pushed}"
    assert any("/admin/users" in v for v in locs), f"no users page_location in {pushed}"


@then(parsers.parse('no dataLayer page_location or page_referrer contains "{a}", "{b}" or "{c}"'))
def no_leak(world: World, a: str, b: str, c: str) -> None:
    leaks = [(k, v) for k, v in _x(world)["rh_pushed"] if any(s in v for s in (a, b, c))]
    assert not leaks, f"admin query leaked into the dataLayer: {leaks}"


# ── guest returnUrl ──────────────────────────────────────────────────────────
@then(parsers.parse('the location contains no "{a}" and no "{b}"'))
def location_clean(world: World, a: str, b: str) -> None:
    url = world.page.url
    assert a not in url and b not in url, url
