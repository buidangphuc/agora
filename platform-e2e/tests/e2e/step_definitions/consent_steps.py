"""Consent banner / Consent Mode v2 / edge consent stamping steps (tracking/consent*.feature).

Black-box: the browser is observed through the DOM, window.dataLayer, cookies and network
requests; the edge stamping through the TrackingEvent envelope on analytics.events (field 26 =
ConsentState{analytics=1, marketing=2, personalization=3, version=4}). Google hosts are mocked.
"""

from __future__ import annotations

import re
import uuid

import httpx
from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from config.settings import get_settings
from src.constants import timeouts
from tests.e2e.flows.tracking_flow import _fields, consume_tracking_events
from tests.e2e.step_definitions.consent_contract_steps import _CONSENT, _payload_fields
from tests.e2e.step_definitions.ga4_datalayer_steps import _IDLE_WAIT_MS, _start_recording
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

_CLS_OBSERVER = """
(() => {
  window.__e2eCls = 0;
  try {
    new PerformanceObserver((list) => {
      for (const e of list.getEntries()) if (!e.hadRecentInput) window.__e2eCls += e.value;
    }).observe({ type: "layout-shift", buffered: true });
  } catch (e) {}
})();
"""
_GROUPS = ("analytics", "marketing", "personali[sz]ation")


def _x(world: World) -> dict:
    return world.state.extra


def _banner(world: World):
    return world.page.get_by_role("dialog")


def _open_home(world: World, *, cls: bool = False) -> None:
    _start_recording(world)
    if cls:
        world.page.add_init_script(_CLS_OBSERVER)
    world.page.goto(world.settings.base_url + "/", wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)


@when("a visitor with no consent cookie opens the home page")
def open_home(world: World) -> None:
    _open_home(world)


@when("a visitor with no consent cookie cold-loads the home page while observing layout shift")
def open_home_cls(world: World) -> None:
    _open_home(world, cls=True)
    world.page.wait_for_timeout(3_000)


@when(parsers.parse('the visitor clicks "{label}" on the consent banner'))
def click_banner_button(world: World, label: str) -> None:
    expect(_banner(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    _banner(world).get_by_role("button", name=label).click()


@when("the visitor reloads the page and waits for the tracking queue idle window")
def reload_idle(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")
    world.page.wait_for_timeout(_IDLE_WAIT_MS)


@when("the visitor views the seeded listing and waits for the tracking queue idle window")
def view_listing(world: World) -> None:
    listing_id = world.state.listing.listing_id  # type: ignore[union-attr]
    x = _x(world)
    x["listing_id"] = listing_id
    x.setdefault("view_marks", []).append(len(x["track_batches"]))
    world.page.goto(
        f"{world.settings.base_url}/listing/{listing_id}", wait_until="domcontentloaded"
    )
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    world.page.wait_for_timeout(_IDLE_WAIT_MS)


@when("the visitor grants personalization through the Cookie settings link")
def grant_personalization(world: World) -> None:
    world.page.get_by_text(re.compile(r"cookie settings", re.I)).first.click()
    expect(_banner(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    box = _banner(world).get_by_role("checkbox", name=re.compile(_GROUPS[2], re.I))
    box.check()
    _banner(world).get_by_role("button", name=re.compile("save choices", re.I)).click()
    world.page.wait_for_timeout(500)


# ── banner / GTM gating ──────────────────────────────────────────────────
@then("the consent banner is visible with analytics, marketing and personalization groups")
def banner_visible(world: World) -> None:
    banner = _banner(world)
    expect(banner).to_be_visible(timeout=timeouts.NAVIGATION)
    for group in _GROUPS:
        expect(banner.get_by_text(re.compile(group, re.I)).first).to_be_visible()


@then("the consent banner is removed from the DOM")
def banner_removed(world: World) -> None:
    expect(_banner(world)).to_have_count(0, timeout=timeouts.NAVIGATION)


@then(parsers.parse("the DOM has {n:d} googletagmanager.com scripts"))
def gtm_scripts(world: World, n: int) -> None:
    world.page.wait_for_timeout(1_500)
    expect(world.page.locator("script[src*='googletagmanager.com']")).to_have_count(n)


@then(parsers.parse("exactly {n:d} GTM script is injected"))
def exactly_gtm_script(world: World, n: int) -> None:
    expect(world.page.locator("script[src*='googletagmanager.com']")).to_have_count(
        n, timeout=timeouts.NAVIGATION
    )


@then("the browser issued 0 requests to googletagmanager.com and google-analytics.com")
def no_google_requests(world: World) -> None:
    seen = _x(world)["gtm_requests"]
    assert not seen, f"Google-host requests before/without analytics consent: {seen}"


_NORMALISE = """
() => Array.from(window.dataLayer || []).map((e) =>
  e && typeof e === "object" && !Array.isArray(e) && typeof e.length === "number" && e.length > 0
    ? Array.from(e) : e)
"""


@then(
    "window.dataLayer holds a consent default with every signal denied followed by a consent "
    "update with analytics_storage granted"
)
def consent_default_then_update(world: World) -> None:
    world.page.wait_for_function(
        "() => (window.dataLayer || []).some(e => e && e[0] === 'consent' && e[1] === 'update')",
        timeout=timeouts.LONG,
    )
    layer = world.page.evaluate(_NORMALISE)
    consent = [e for e in layer if isinstance(e, list) and e and e[0] == "consent"]
    kinds = [e[1] for e in consent]
    assert "default" in kinds and "update" in kinds, f"consent entries: {consent}"
    default_i, update_i = kinds.index("default"), kinds.index("update")
    assert default_i < update_i, f"consent update before default: {consent}"
    default = consent[default_i][2]
    assert default and all(v == "denied" for v in default.values()), default
    assert {"analytics_storage", "ad_storage"} <= set(default), default
    assert consent[update_i][2].get("analytics_storage") == "granted", consent[update_i]


# ── first-party beacons ──────────────────────────────────────────────────
@then("at least one POST to /api/track was sent")
def beacon_sent(world: World) -> None:
    batches = _x(world)["track_batches"]
    assert any(b.get("type") == "view" for batch in batches for b in batch), (
        f"no view beacon sent after rejecting all: {batches}"
    )


@then("no beacon body has a consent key")
def no_consent_key(world: World) -> None:
    for batch in _x(world)["track_batches"]:
        for beacon in batch:
            assert "consent" not in beacon, f"consent key in beacon body: {beacon}"


# ── edge stamping, observed on analytics.events ──────────────────────────
def _envelopes(marker: str) -> list[bytes]:
    s = get_settings()
    raws = consume_tracking_events(
        s.kafka_brokers, s.kafka_analytics_topic, contains=marker, timeout_s=30.0
    )
    out = [bytes(r) for r in raws if isinstance(r, (bytes, bytearray))]
    assert out, f"no envelope published for {marker}"
    return out


def _consent(raw: bytes) -> dict[int, int] | None:
    """ConsentState of an envelope as {field: int}; None when TrackingEvent.consent is unset."""
    found = [v for n, w, v in _payload_fields(raw) if n == _CONSENT and w == 2]
    if not found:
        return None
    return {n: int(v) for n, w, v in _fields(bytes(found[0])) if w == 0}


def _assert_consent(raw: bytes, *, p: bool, version: int) -> None:
    c = _consent(raw)
    assert c is not None, "TrackingEvent.consent is not stamped by the edge"
    assert bool(c.get(3, 0)) is p, f"personalization expected {p}, consent={c}"
    assert c.get(4, 0) == version, f"version expected {version}, consent={c}"


@then(
    "every event published for the seeded listing carries consent personalization false "
    "and version 1"
)
def all_events_denied_v1(world: World) -> None:
    for raw in _envelopes(_x(world)["listing_id"]):
        _assert_consent(raw, p=False, version=1)


def _view_event_ids(world: World, lo: int, hi: int | None) -> list[str]:
    batches = _x(world)["track_batches"][lo:hi]
    return [b["eventId"] for batch in batches for b in batch if b.get("eventId")]


def _events_for(world: World, ids: list[str]) -> list[bytes]:
    assert ids, "no beacon event ids captured for this phase"
    raws = _envelopes(_x(world)["listing_id"])
    matched = [r for r in raws if any(i.encode() in r for i in ids)]
    assert matched, f"no published event matches beacon ids {ids[:3]}"
    return matched


@then("the events published for beacons sent before the change carry personalization false")
def before_change(world: World) -> None:
    marks = _x(world)["view_marks"]
    for raw in _events_for(world, _view_event_ids(world, marks[0], marks[1])):
        _assert_consent(raw, p=False, version=1)


@then(
    "the events published for beacons sent after the change carry personalization true "
    "and version 1"
)
def after_change(world: World) -> None:
    marks = _x(world)["view_marks"]
    for raw in _events_for(world, _view_event_ids(world, marks[1], None)):
        _assert_consent(raw, p=True, version=1)


# ── API-level cookie / body cases ────────────────────────────────────────
def _post_beacon(world: World, cookie: str | None, body_extra: dict | None = None) -> None:
    x = _x(world)
    x["listing_id"] = f"e2e-cs-{tii.run_id()}"
    beacon = {
        "type": "view",
        "listingId": x["listing_id"],
        "sessionId": f"e2e-cs-sess-{tii.run_id()}",
        "anonymousId": f"e2e-anon-{uuid.uuid4()}",
        "path": "/e2e/consent",
        "eventId": str(uuid.uuid4()),
        **(body_extra or {}),
    }
    headers = {"Content-Type": "application/json"}
    if cookie is not None:
        headers["Cookie"] = f"agora_consent={cookie}"
    x["cs_resp"] = httpx.post(
        f"{tii.gateway_url()}{tii.ep.TRACK}", json=[beacon], headers=headers, timeout=15
    )


@when(
    parsers.parse('a beacon whose body claims personalization true arrives with cookie "{cookie}"')
)
def forged_body(world: World, cookie: str) -> None:
    _post_beacon(world, cookie, {"consent": {"personalization": True}})


@when("a beacon arrives with no agora_consent cookie")
def no_cookie(world: World) -> None:
    _post_beacon(world, None)


@then(parsers.parse("the response is {status:d}"))
def response_status(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["cs_resp"]
    assert resp.status_code == status, f"{resp.status_code}: {resp.text[:200]}"


@then("the published event carries consent personalization false and version 1")
def published_denied_v1(world: World) -> None:
    for raw in _envelopes(_x(world)["listing_id"]):
        _assert_consent(raw, p=False, version=1)


@then("the published event carries a consent message with all flags false and version 0")
def published_version0(world: World) -> None:
    for raw in _envelopes(_x(world)["listing_id"]):
        c = _consent(raw)
        assert c is not None, "TrackingEvent.consent is not stamped (version 0 expected, unset)"
        assert not any(c.get(n) for n in (1, 2, 3, 4)), f"expected all false / version 0: {c}"


@then("the cumulative layout shift is below 0.1")
def cls_below(world: World) -> None:
    cls = world.page.evaluate("() => window.__e2eCls")
    assert cls is not None and float(cls) < 0.1, f"CLS {cls}"
