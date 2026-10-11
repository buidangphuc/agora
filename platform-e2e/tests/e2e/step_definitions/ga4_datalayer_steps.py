"""GA4 data layer / batched-beacon steps (tracking/ga4_datalayer.feature).

Black-box: everything is observed from the browser (window.dataLayer, the DOM, the
network requests to /api/track and googletagmanager.com), never from frontend code.
"""

from __future__ import annotations

import json
import re

from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from src.constants import timeouts
from tests.e2e.support.world import World

_IDLE_WAIT_MS = 4_000  # spec: queue flushes after 2 s idle; leave margin

# navigator.sendBeacon bodies are Blobs that Playwright cannot read from the request.
_TAP = """
(() => {
  const original = navigator.sendBeacon.bind(navigator);
  navigator.sendBeacon = (url, data) => {
    try {
      if (String(url).includes("/api/track") && window.__e2eRecordBatch) {
        Promise.resolve(data && data.text ? data.text() : String(data))
          .then((t) => window.__e2eRecordBatch(t)).catch(() => {});
      }
    } catch (e) {}
    return original(url, data);
  };
})();
"""


def _start_recording(world: World) -> None:
    batches: list[list[dict]] = []  # one entry per POST to /api/track
    gtm_requests: list[str] = []
    world.state.extra["track_batches"] = batches
    world.state.extra["gtm_requests"] = gtm_requests
    seen: set[str] = set()

    def collect(body: str | None) -> None:
        if not body or body in seen:
            return
        try:
            payload = json.loads(body)
        except ValueError:
            return
        seen.add(body)
        batches.append(payload if isinstance(payload, list) else [payload])

    def on_request(request) -> None:  # noqa: ANN001
        if "googletagmanager.com" in request.url:
            gtm_requests.append(request.url)
        if "/api/track" in request.url:
            collect(request.post_data)

    world.page.on("request", on_request)
    world.page.expose_function("__e2eRecordBatch", collect)
    world.page.add_init_script(_TAP)


def _open_home(world: World) -> None:
    _start_recording(world)
    world.page.goto(world.settings.base_url + "/", wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    # impressions are viewability-gated: scroll the whole page so every list renders in view
    for _ in range(4):
        world.page.mouse.wheel(0, 700)
        world.page.wait_for_timeout(120)
    world.page.evaluate("() => window.scrollTo(0, 0)")


@when("the buyer opens the home page while recording tracking beacons")
def buyer_opens_home(world: World) -> None:
    _open_home(world)


@when("a guest opens the home page while recording tracking beacons")
def guest_opens_home(world: World) -> None:
    _open_home(world)


@when("the tracking queue idle window elapses")
def idle_window(world: World) -> None:
    world.page.wait_for_timeout(_IDLE_WAIT_MS)


def _data_layer(world: World) -> list:
    return world.page.evaluate("() => Array.from(window.dataLayer || [])")


def _list_entry_index(layer: list) -> int:
    for i, entry in enumerate(layer):
        if isinstance(entry, dict) and entry.get("event") == "view_item_list":
            return i
    return -1


# ── dataLayer ────────────────────────────────────────────────────────────
@then(parsers.parse('window.dataLayer contains a "{event}" entry with ecommerce items'))
def datalayer_has_list_entry(world: World, event: str) -> None:
    world.page.wait_for_function(
        "(ev) => (window.dataLayer || []).some(e => e && e.event === ev)",
        arg=event,
        timeout=timeouts.LONG,
    )
    layer = _data_layer(world)
    sizes = [
        len((e.get("ecommerce") or {}).get("items") or [])
        for e in layer
        if isinstance(e, dict) and e.get("event") == event
    ]
    assert max(sizes) >= 2, f"a rendered list must push all its items in one entry; sizes={sizes}"


@then("that dataLayer entry is immediately preceded by an ecommerce null reset")
def datalayer_null_reset(world: World) -> None:
    layer = _data_layer(world)
    idx = _list_entry_index(layer)
    assert idx > 0, f"view_item_list is first/absent in dataLayer: {layer}"
    assert layer[idx - 1] == {"ecommerce": None}, f"previous entry is {layer[idx - 1]}"


# ── beacon batching / fan-out ────────────────────────────────────────────
def _list_key(beacon: dict) -> str:
    """The rendered list a beacon belongs to: its item list id, else its placement, else its page."""
    return beacon.get("itemListId") or beacon.get("placementId") or beacon.get("path") or ""


def _impressions(batch: list[dict]) -> list[dict]:
    return [b for b in batch if b.get("type") in ("impression", "view_item_list")]


@then(
    parsers.parse(
        'all impression beacons of one item list were sent in exactly {n:d} POST to "{path}"'
    )
)
def one_request_per_list(world: World, n: int, path: str) -> None:
    batches = world.state.extra["track_batches"]
    carrying = [b for b in batches if len(_impressions(b)) >= 1]
    assert carrying, f"no impression beacon was sent to {path}: {batches}"
    lists: dict[str, set[int]] = {}
    for i, batch in enumerate(carrying):
        for b in _impressions(batch):
            lists.setdefault(_list_key(b), set()).add(i)
    split = {k: v for k, v in lists.items() if len(v) != n}
    assert not split, f"item lists spread over a number of requests != {n}: {split}"
    world.state.extra["list_batch"] = carrying[0]


@then(parsers.parse("that request body is an array of at least {n:d} beacons"))
def body_is_array(world: World, n: int) -> None:
    batch = world.state.extra["list_batch"]
    assert isinstance(batch, list) and len(_impressions(batch)) >= n, batch


def _first_list(world: World) -> list[dict]:
    imps = [b for batch in world.state.extra["track_batches"] for b in _impressions(batch)]
    assert imps, "no impression beacons captured"
    key = _list_key(imps[0])
    return [b for b in imps if (_list_key(b)) == key]


@then("the impression beacons of one item list have distinct event ids and one non-empty event group id")
def distinct_ids_one_group(world: World) -> None:
    rows = _first_list(world)
    assert len(rows) >= 2, f"need a multi-item list to check fan-out, got {rows}"
    ids = [r.get("eventId") for r in rows]
    assert all(ids) and len(set(ids)) == len(ids), f"event ids not distinct: {ids}"
    groups = {r.get("eventGroupId") for r in rows}
    assert len(groups) == 1 and "" not in groups and None not in groups, f"groups: {groups}"


@then("each of those beacons carries its own position and the same item list id")
def own_position_same_list(world: World) -> None:
    rows = _first_list(world)
    positions = [r.get("position") for r in rows]
    assert all(p is not None for p in positions) and len(set(positions)) == len(positions), positions
    assert len({r.get("itemListId") for r in rows}) == 1 and rows[0].get("itemListId"), rows


# ── GTM gating ───────────────────────────────────────────────────────────
@then("the DOM has no googletagmanager.com script")
def no_gtm_script(world: World) -> None:
    expect(world.page.locator("script[src*='googletagmanager.com']")).to_have_count(0)


@then(parsers.parse("the browser issued {n:d} requests to googletagmanager.com"))
def no_gtm_requests(world: World, n: int) -> None:
    got = world.state.extra["gtm_requests"]
    assert len(got) == n, f"expected {n} requests to googletagmanager.com, got {got}"


# ── PII ──────────────────────────────────────────────────────────────────
_EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
_PHONE = re.compile(r'"\+?\d[\d\s().-]{8,}\d"')


@then("neither window.dataLayer nor any beacon body contains the buyer's email, phone or name")
def no_pii(world: World) -> None:
    username = world.state.current_user.username
    blobs = [json.dumps(_data_layer(world))]
    blobs += [json.dumps(b) for b in world.state.extra["track_batches"]]
    # the data layer must have been exercised, otherwise this is vacuous
    assert world.state.extra["track_batches"], "no beacons captured; nothing to check"
    layer = _data_layer(world)
    assert _list_entry_index(layer) >= 0, "no view_item_list in dataLayer; nothing to check"
    keys = r'"(email|phone|name|address|fullName|firstName|lastName)"\s*:'
    for blob in blobs:
        assert username not in blob, f"buyer name leaked: {blob[:300]}"
        assert not _EMAIL.search(blob), f"email-like value leaked: {blob[:300]}"
        assert not re.search(keys, blob, re.I), f"PII key present: {blob[:300]}"
        assert not _PHONE.search(blob), f"phone-like value: {blob[:300]}"
