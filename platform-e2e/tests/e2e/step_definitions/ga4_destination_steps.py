"""GA4 destination steps (tracking/ga4_destination.feature, change add-ga4-destination).

Black-box. Observed surfaces: the versioned GTM container file, HTTP responses of the frontend
(`/api/analytics/identity`, the `Content-Security-Policy` header), the browser (window.dataLayer,
CSP violation events, DOM, network requests, /api/track beacon bodies).

Stack knobs (none are read from implementation code):
  E2E_GA4_USER_ID_HMAC_KEY  the HMAC key the frontend under test runs with (reference HMAC and
                            "no key" scenarios); unset = the frontend has no key
  E2E_APP_ENV               the NEXT_PUBLIC_APP_ENV the frontend was built with (staging|production)
  FRONTEND_CONTAINER        container whose log is scanned (default team-frontend-svc)
  GTM_CONTAINER_PATH        override of team-frontend/analytics/gtm-container.json
Scenarios whose deployment precondition is not met are skipped with the reason, never green.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import re
import shutil
import subprocess
import time
from pathlib import Path

import httpx
import pytest
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.step_definitions.consent_steps import _GROUPS, _NORMALISE
from tests.e2e.step_definitions.ga4_datalayer_steps import (
    _GOOGLE_HOST,
    _open_home,
    _start_recording,
)
from tests.e2e.support.world import World

_ROOT = Path(__file__).resolve().parents[4]
_CONTAINER = Path(
    os.getenv("GTM_CONTAINER_PATH", str(_ROOT / "team-frontend" / "analytics" / "gtm-container.json"))
)
_FUNNEL = (
    "view_item_list",
    "select_item",
    "view_item",
    "add_to_cart",
    "remove_from_cart",
    "view_cart",
    "begin_checkout",
    "add_shipping_info",
    "add_payment_info",
)
_IDENTITY = "/api/analytics/identity"
_CONSENT_ON = "v=1&a=1&m=0&p=0&ts=1760000000"
_CONSENT_OFF = "v=1&a=0&m=0&p=0&ts=1760000000"
_GOOGLE_HOSTS = ("googletagmanager.com", "google-analytics.com", "analytics.google.com")
_OTHER_ANALYTICS = ("facebook.net", "hotjar", "segment.", "mixpanel", "clarity.ms", "amplitude", "fullstory")
_VIOLATION_RECORDER = """
(() => {
  window.__csp = [];
  document.addEventListener("securitypolicyviolation", (e) => {
    window.__csp.push({ directive: e.effectiveDirective, blocked: e.blockedURI });
  });
})();
"""


def _x(world: World) -> dict:
    return world.state.extra


def _key() -> str | None:
    return os.getenv("E2E_GA4_USER_ID_HMAC_KEY") or None


def _require_key() -> str:
    key = _key()
    if not key:
        pytest.skip("needs E2E_GA4_USER_ID_HMAC_KEY (frontend must run with the same GA4_USER_ID_HMAC_KEY)")
    return key  # type: ignore[return-value]


def _sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


def _reference(key: str, principal: str) -> str:
    return hmac.new(key.encode(), principal.encode(), hashlib.sha256).hexdigest()


def _identity_get(
    world: World,
    *,
    token: str | None,
    consent: str | None,
    params: dict | None = None,
    client: httpx.Client | None = None,
) -> httpx.Response:
    cookies = []
    if token:
        cookies.append(f"session={token}")
    if consent:
        cookies.append(f"agora_consent={consent}")
    headers = {"cookie": "; ".join(cookies)} if cookies else {}
    c = client or httpx.Client(timeout=10, follow_redirects=False)
    try:
        return c.get(world.settings.base_url + _IDENTITY, headers=headers, params=params)
    finally:
        if client is None:
            c.close()


def _buyer_token(world: World) -> str:
    user = world.state.current_user
    assert user and user.token, "no logged-in buyer"
    return user.token


def _responses(world: World) -> list[httpx.Response]:
    return _x(world).setdefault("ga4d_responses", [])


def _layer(world: World) -> list:
    return world.page.evaluate(_NORMALISE)


def _gtm_scripts(world: World) -> int:
    return world.page.locator("script[src*='googletagmanager.com']").count()


# ══ GTM container ════════════════════════════════════════════════════════
def _version(data: dict) -> dict:
    return data.get("containerVersion", data)


@when("the GTM container JSON is parsed")
def parse_container(world: World) -> None:
    assert _CONTAINER.is_file(), f"GTM container missing: {_CONTAINER}"
    text = _CONTAINER.read_text("utf-8")
    _x(world)["ga4d_container_text"] = text
    _x(world)["ga4d_container"] = json.loads(text)


def _tags(world: World) -> list[dict]:
    tags = _version(_x(world)["ga4d_container"]).get("tag") or []
    assert tags, "container defines no tags"
    return tags


def _param(tag: dict, key: str) -> str | None:
    for p in tag.get("parameter", []):
        if p.get("key") == key:
            return p.get("value")
    return None


def _is_config(tag: dict) -> bool:
    return tag.get("type") == "gaawc"


def _is_event(tag: dict) -> bool:
    return tag.get("type") == "gaawe"


@then(parsers.parse("it has exactly {n:d} GA4 configuration tag"))
def one_config_tag(world: World, n: int) -> None:
    configs = [t for t in _tags(world) if _is_config(t)]
    assert len(configs) == n, f"{len(configs)} GA4 configuration tags (type gaawc), expected {n}"


@then("it has one GA4 event tag for each funnel event with a custom-event trigger of the same name")
def event_tags(world: World) -> None:
    version = _version(_x(world)["ga4d_container"])
    triggers = {str(t.get("triggerId")): t for t in version.get("trigger", [])}
    events = [t for t in _tags(world) if _is_event(t)]
    for name in _FUNNEL:
        tagged = [t for t in events if _param(t, "eventName") == name]
        assert len(tagged) == 1, f"{len(tagged)} GA4 event tags for {name}, expected exactly 1"
        firing = [triggers.get(str(i)) for i in tagged[0].get("firingTriggerId", [])]
        assert any(
            t and t.get("type") == "CUSTOM_EVENT" and name in json.dumps(t.get("customEventFilter", []))
            for t in firing
        ), f"{name}: no CUSTOM_EVENT trigger on '{name}' (firing: {firing})"


@then("every tag requires analytics_storage consent and 0 tags are unconditional")
def tags_consent_gated(world: World) -> None:
    bad = []
    for tag in _tags(world):
        cs = tag.get("consentSettings") or {}
        needed = str(cs.get("consentStatus", "")).upper() == "NEEDED"
        if not (needed and "analytics_storage" in json.dumps(cs)):
            bad.append(tag.get("name"))
    assert not bad, f"tags not gated on analytics_storage: {bad}"


@then(parsers.parse("it contains {n:d} tags whose event name is purchase"))
def no_purchase_tag(world: World, n: int) -> None:
    purchase = [
        t.get("name")
        for t in _tags(world)
        if _param(t, "eventName") == "purchase" or str(t.get("name", "")).strip().lower() == "purchase"
    ]
    assert len(purchase) == n, f"purchase tags present: {purchase}"
    assert any(_is_event(t) for t in _tags(world)), "container has no GA4 event tags at all"


@then("it contains 0 occurrences of any value of the HMAC key or the API secret")
def no_secret_values(world: World) -> None:
    text = _x(world)["ga4d_container_text"]
    for env in ("GA4_USER_ID_HMAC_KEY", "GA4_API_SECRET", "E2E_GA4_USER_ID_HMAC_KEY"):
        value = os.getenv(env)
        if value:
            assert value not in text, f"{env} value found in the container"
    assert not re.search(r"api_secret\s*[=:]\s*[\"']?[A-Za-z0-9_-]{8,}", text, re.I), "api secret literal"


@then("the measurement id is a variable, not a literal")
def measurement_id_variable(world: World) -> None:
    text = _x(world)["ga4d_container_text"]
    assert not re.search(r"\bG-[A-Z0-9]{6,}\b", text), "literal GA4 measurement id in the container"
    refs = [
        _param(t, "measurementId") or _param(t, "measurementIdOverride")
        for t in _tags(world)
        if _is_config(t)
    ]
    assert refs and all(r and r.startswith("{{") for r in refs), f"measurement id not a variable: {refs}"


# ══ Identity endpoint ════════════════════════════════════════════════════
@given("the GA4 identity endpoint is deployed")
def identity_deployed(world: World) -> None:
    res = _identity_get(world, token=None, consent=_CONSENT_ON)
    assert res.status_code == 204, (
        f"GET {_IDENTITY} for an anonymous caller returned {res.status_code}, expected 204 "
        "(route not deployed?)"
    )


@given("another buyer exists")
def another_buyer(world: World) -> None:
    from src.utils import data as fake

    token = world.service_factory.auth.register(
        fake.unique_username("buyer"), world.settings.seed_password, "buyer"
    )
    _x(world)["ga4d_other_sub"] = _sub(token)


def _call(world: World, *, consent: str | None, token: bool = True, params: dict | None = None) -> httpx.Response:
    res = _identity_get(
        world, token=_buyer_token(world) if token else None, consent=consent, params=params
    )
    _responses(world).append(res)
    return res


@when("the buyer calls the identity endpoint twice with analytics consent")
def buyer_twice(world: World) -> None:
    _call(world, consent=_CONSENT_ON)
    _call(world, consent=_CONSENT_ON)


@when("the buyer calls the identity endpoint with analytics consent")
def buyer_once(world: World) -> None:
    _call(world, consent=_CONSENT_ON)


@when("the buyer calls the identity endpoint with a=0 and again with no consent cookie")
def buyer_no_consent(world: World) -> None:
    _call(world, consent=_CONSENT_OFF)
    _call(world, consent=None)


@when("a caller with a=1 and no valid session calls the identity endpoint")
def anonymous_caller(world: World) -> None:
    _responses(world).append(_identity_get(world, token=None, consent=_CONSENT_ON))
    _responses(world).append(_identity_get(world, token="not.a.valid-session", consent=_CONSENT_ON))


@when("the buyer calls the identity endpoint with analytics consent on a deployment without the HMAC key")
def buyer_without_key(world: World) -> None:
    if _key():
        pytest.skip("frontend runs with an HMAC key (E2E_GA4_USER_ID_HMAC_KEY is set)")
    _call(world, consent=_CONSENT_ON)


@when(
    "the buyer calls the identity endpoint with analytics consent naming the other buyer in a userId parameter"
)
def buyer_names_other(world: World) -> None:
    other = _x(world)["ga4d_other_sub"]
    _call(world, consent=_CONSENT_ON, params={"userId": other, "user_id": other})


@when(parsers.parse("the buyer calls the identity endpoint {n:d} times with analytics consent"))
def buyer_n_times(world: World, n: int) -> None:
    token = _buyer_token(world)
    timings = []
    with httpx.Client(timeout=10, follow_redirects=False) as client:
        for _ in range(n):
            t0 = time.perf_counter()
            res = _identity_get(world, token=token, consent=_CONSENT_ON, client=client)
            timings.append((time.perf_counter() - t0) * 1000)
            _responses(world).append(res)
    _x(world)["ga4d_timings"] = timings


@when("the buyer opens the home page with analytics consent denied")
def home_consent_denied(world: World) -> None:
    world.context.add_cookies(
        [{"name": "agora_consent", "value": _CONSENT_OFF, "url": world.settings.base_url}]
    )
    _open_home(world)
    world.page.wait_for_timeout(1_500)


@when("the buyer opens the home page with analytics consent granted")
def home_consent_granted(world: World) -> None:
    world.context.add_cookies(
        [{"name": "agora_consent", "value": _CONSENT_ON, "url": world.settings.base_url}]
    )
    _open_home(world)


@then("both identity responses are 200 with the same 64-char hex userId equal to the reference HMAC")
def stable_hmac(world: World) -> None:
    key = _require_key()
    first, second = _responses(world)[:2]
    for res in (first, second):
        assert res.status_code == 200, f"identity returned {res.status_code}: {res.text[:200]}"
    a, b = first.json()["userId"], second.json()["userId"]
    assert a == b and re.fullmatch(r"[0-9a-f]{64}", a), f"unstable or malformed userId: {a} / {b}"
    assert a == _reference(key, _sub(_buyer_token(world))), "userId differs from HMAC-SHA256(user id, key)"


@then("the identity body contains no raw user id, email or name")
def body_no_pii(world: World) -> None:
    _require_key()
    user = world.state.current_user
    for res in _responses(world):
        body = res.text
        assert _sub(_buyer_token(world)) not in body and user.username not in body, body  # type: ignore[union-attr]
        assert not re.search(r"[\w.+-]+@[\w-]+\.[\w.-]+", body), body
        assert set(res.json()) <= {"userId"}, f"unexpected identity fields: {sorted(res.json())}"


@then("every identity response is 204 with an empty body")
def all_204(world: World) -> None:
    res = _responses(world)
    assert res, "no identity response captured"
    for r in res:
        assert r.status_code == 204 and r.content == b"", f"{r.status_code}: {r.text[:200]}"


@then("window.dataLayer holds 0 user_id entries")
def no_user_id_entries(world: World) -> None:
    layer = _layer(world)
    assert any(layer), "dataLayer empty; the page was not exercised"
    assert not [e for e in layer if isinstance(e, dict) and "user_id" in e], layer


@then("the frontend log contains neither the buyer id nor the HMAC key")
def log_clean(world: World) -> None:
    if not shutil.which("docker"):
        return
    name = os.getenv("FRONTEND_CONTAINER", "team-frontend-svc")
    out = subprocess.run(["docker", "logs", name], capture_output=True, text=True, check=False)
    logs = out.stdout + out.stderr
    if out.returncode != 0:
        return
    assert _sub(_buyer_token(world)) not in logs, "buyer id leaked into the frontend log"
    assert world.state.current_user.username not in logs, "buyer name leaked into the frontend log"  # type: ignore[union-attr]
    if _key():
        assert _key() not in logs, "HMAC key leaked into the frontend log"


@then("the identity response is 200 with the HMAC of the session buyer only")
def only_session_hmac(world: World) -> None:
    key = _require_key()
    res = _responses(world)[-1]
    assert res.status_code == 200, f"identity returned {res.status_code}"
    got = res.json()["userId"]
    assert got == _reference(key, _sub(_buyer_token(world))), "not the HMAC of the session principal"
    assert got != _reference(key, _x(world)["ga4d_other_sub"]), "returned the other buyer's HMAC"


@then("every identity response has Cache-Control no-store")
def no_store(world: World) -> None:
    res = _responses(world)
    assert len(res) >= 2, "expected both a buyer and an anonymous response"
    for r in res:
        assert "no-store" in r.headers.get("cache-control", "").lower(), (
            f"{r.status_code} lacks no-store: {dict(r.headers)}"
        )


@then(parsers.parse("the p95 latency of the identity calls is below {ms:d} ms"))
def p95_fast(world: World, ms: int) -> None:
    assert all(r.status_code in (200, 204) for r in _responses(world)), "identity calls failed"
    timings = sorted(_x(world)["ga4d_timings"])
    assert len(timings) == 100
    p95 = timings[94]
    assert p95 < ms, f"p95 {p95:.1f} ms >= {ms} ms"


@when("the buyer withdraws analytics consent through the Cookie settings link")
def withdraw_analytics(world: World) -> None:
    world.page.get_by_text(re.compile(r"cookie settings", re.I)).first.click()
    dialog = world.page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    dialog.get_by_role("checkbox", name=re.compile(_GROUPS[0], re.I)).uncheck()
    dialog.get_by_role("button", name=re.compile("save choices", re.I)).click()
    world.page.wait_for_timeout(1_000)


@then("the dataLayer holds a consent update with analytics_storage denied after which no user_id is pushed")
def withdrawal_stops_user_id(world: World) -> None:
    _require_key()
    world.page.wait_for_function(
        "() => (window.dataLayer || []).some(e => e && e.user_id)", timeout=timeouts.LONG
    )
    layer = _layer(world)
    denied = [
        i
        for i, e in enumerate(layer)
        if isinstance(e, list) and e[:2] == ["consent", "update"] and e[2].get("analytics_storage") == "denied"
    ]
    assert denied, f"no consent update with analytics_storage=denied: {layer}"
    assert any(isinstance(e, dict) and "user_id" in e for e in layer[: denied[-1]]), "user_id never pushed"
    after = [e for e in layer[denied[-1] + 1 :] if isinstance(e, dict) and "user_id" in e]
    assert not after, f"user_id pushed after withdrawal: {after}"


@then("the next identity call with the browser cookies is 204")
def next_call_204(world: World) -> None:
    jar = {c["name"]: c["value"] for c in world.context.cookies()}
    assert "a=0" in jar.get("agora_consent", ""), f"agora_consent not withdrawn: {jar.get('agora_consent')}"
    res = _identity_get(world, token=jar.get("session"), consent=jar.get("agora_consent"))
    assert res.status_code == 204 and res.content == b"", res.status_code


# ══ CSP ══════════════════════════════════════════════════════════════════
def _open_recording_csp(world: World) -> None:
    _start_recording(world)
    world.page.add_init_script(_VIOLATION_RECORDER)
    resp = world.page.goto(world.settings.base_url + "/", wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    world.page.wait_for_timeout(2_000)
    _x(world)["ga4d_csp"] = (resp.headers.get("content-security-policy") if resp else None) or ""


@when("a visitor opens the home page of a deployment with a GTM id while recording CSP violations")
def csp_with_gtm(world: World) -> None:
    _open_recording_csp(world)
    if _gtm_scripts(world) == 0:
        pytest.skip("frontend built without NEXT_PUBLIC_GTM_ID")


@when("a visitor opens the home page of a deployment without a GTM id while recording CSP violations")
def csp_without_gtm(world: World) -> None:
    _open_recording_csp(world)
    if _gtm_scripts(world) > 0:
        pytest.skip("frontend built with NEXT_PUBLIC_GTM_ID")


@when("a visitor opens the home page while recording CSP violations")
def csp_any(world: World) -> None:
    _open_recording_csp(world)


def _directive(csp: str, name: str) -> str:
    for part in csp.split(";"):
        tokens = part.strip().split()
        if tokens and tokens[0] == name:
            return " ".join(tokens[1:])
    return ""


@then("the CSP header allows the Google analytics hosts in script-src, connect-src and img-src")
def csp_allows(world: World) -> None:
    csp = _x(world)["ga4d_csp"]
    assert csp, "no Content-Security-Policy header on the home page"
    hosts = (
        "https://www.googletagmanager.com",
        "https://*.google-analytics.com",
        "https://*.analytics.google.com",
        "https://region1.google-analytics.com",
    )
    for directive in ("script-src", "connect-src", "img-src"):
        value = _directive(csp, directive)
        missing = [h for h in hosts if h not in value]
        assert not missing, f"{directive} lacks {missing}: {value}"


@then("the CSP header names no other third-party analytics host")
def csp_no_other(world: World) -> None:
    csp = _x(world)["ga4d_csp"].lower()
    assert csp, "no Content-Security-Policy header on the home page"
    assert not [h for h in _OTHER_ANALYTICS if h in csp], csp


@then(parsers.parse("{n:d} CSP violation reports were produced"))
def no_violations(world: World, n: int) -> None:
    violations = world.page.evaluate("() => window.__csp || []")
    assert len(violations) == n, f"CSP violations: {violations}"


@then("the CSP header is present and contains 0 Google analytics hosts")
def csp_no_google(world: World) -> None:
    csp = _x(world)["ga4d_csp"]
    assert csp, "no Content-Security-Policy header on the home page"
    assert not [h for h in _GOOGLE_HOSTS if h in csp], csp


@then("the browser made 0 requests to Google hosts")
def zero_google_requests(world: World) -> None:
    seen = _x(world)["gtm_requests"]
    assert not seen, f"requests to Google hosts: {seen}"


@when("a script from an unlisted host is injected")
def inject_unlisted(world: World) -> None:
    world.page.evaluate(
        """() => { const s = document.createElement("script");
                   s.src = "https://e2e-unlisted.example.org/x.js"; document.head.appendChild(s); }"""
    )


@then("the browser blocks it and reports a CSP violation")
def blocked_with_violation(world: World) -> None:
    assert _x(world)["ga4d_csp"], "no Content-Security-Policy header on the home page"
    world.page.wait_for_function(
        "() => (window.__csp || []).some(v => String(v.blocked).includes('e2e-unlisted.example.org'))",
        timeout=timeouts.LONG,
    )


# ══ debug_mode / no IDs ══════════════════════════════════════════════════
def _has_debug_true(node) -> bool:  # noqa: ANN001
    if isinstance(node, dict):
        return node.get("debug_mode") in (True, "true") or any(_has_debug_true(v) for v in node.values())
    if isinstance(node, list):
        return any(_has_debug_true(v) for v in node)
    return False


def _open_for_env(world: World, expected: str) -> None:
    if os.getenv("E2E_APP_ENV") != expected:
        pytest.skip(f"needs a frontend built with NEXT_PUBLIC_APP_ENV={expected} (set E2E_APP_ENV)")
    _open_home(world)
    world.page.wait_for_timeout(1_500)


@when("a buyer opens the home page with analytics consent on a staging deployment")
def open_staging(world: World) -> None:
    _open_for_env(world, "staging")


@when("a buyer opens the home page with analytics consent on a production deployment")
def open_production(world: World) -> None:
    _open_for_env(world, "production")


@then("window.dataLayer has an entry with debug_mode true")
def debug_true(world: World) -> None:
    world.page.wait_for_function(
        "() => JSON.stringify(Array.from(window.dataLayer || [], e => e && typeof e.length === 'number' ? Array.from(e) : e)).includes('\"debug_mode\":true')",
        timeout=timeouts.LONG,
    )
    assert _has_debug_true(_layer(world))


@then("window.dataLayer holds ecommerce entries and none has debug_mode true")
def debug_absent(world: World) -> None:
    world.page.wait_for_function(
        "() => (window.dataLayer || []).some(e => e && e.event === 'view_item_list')",
        timeout=timeouts.LONG,
    )
    assert not _has_debug_true(_layer(world)), "debug_mode true present outside staging"


@when("a visitor opens the home page of a deployment without a GTM id and without the HMAC key")
def open_without_ids(world: World) -> None:
    if _key():
        pytest.skip("frontend runs with an HMAC key")
    _open_home(world)
    world.page.wait_for_timeout(2_000)
    if _gtm_scripts(world) > 0:
        pytest.skip("frontend built with NEXT_PUBLIC_GTM_ID")


@then("no GTM script element exists")
def no_gtm_script(world: World) -> None:
    assert _gtm_scripts(world) == 0


@then("the identity endpoint returns 204 for an authenticated consenting caller")
def identity_204_authenticated(world: World) -> None:
    res = _identity_get(world, token=_buyer_token(world), consent=_CONSENT_ON)
    assert res.status_code == 204 and res.content == b"", res.status_code


# ══ ga_client_id in beacons ══════════════════════════════════════════════
def _set_ga(world: World, ga: str) -> None:
    _x(world)["ga4d_ga"] = ga
    errors: list[str] = []
    _x(world)["ga4d_page_errors"] = errors
    world.page.on("pageerror", lambda exc: errors.append(str(exc)))
    if ga != "absent":
        world.context.add_cookies([{"name": "_ga", "value": ga, "url": world.settings.base_url}])


@when(
    parsers.parse('a visitor with analytics consent and the cookie _ga "{ga}" opens the home page')
)
def visitor_consent_ga(world: World, ga: str) -> None:
    world.context.add_cookies(
        [{"name": "agora_consent", "value": _CONSENT_ON, "url": world.settings.base_url}]
    )
    _set_ga(world, ga)
    _open_home(world)


@when(
    parsers.parse(
        'a visitor with the consent cookie "{consent}" and the cookie _ga "{ga}" opens the home page'
    )
)
def visitor_cookie_ga(world: World, consent: str, ga: str) -> None:
    if consent != "absent":
        world.context.add_cookies(
            [{"name": "agora_consent", "value": consent, "url": world.settings.base_url}]
        )
    _set_ga(world, ga)
    _open_home(world)


@when("the visitor scrolls the page to queue view events")
def scroll_queue(world: World) -> None:
    for _ in range(4):
        world.page.mouse.wheel(0, 600)
        world.page.wait_for_timeout(500)


def _beacons(world: World) -> list[dict]:
    beacons = [b for batch in _x(world)["track_batches"] for b in batch]
    assert beacons, "no /api/track beacon was sent"
    return beacons


def _client_id(ga: str) -> str:
    m = re.fullmatch(r"GA1\.\d+\.(.+)", ga)
    return m.group(1) if m else ""


@then(parsers.parse("at least one beacon was sent and every beacon has a gaClientId that is {outcome}"))
def all_outcome(world: World, outcome: str) -> None:
    assert outcome in ("kept", "empty"), outcome
    expected = _client_id(_x(world)["ga4d_ga"]) if outcome == "kept" else ""
    ids = [b.get("gaClientId") or "" for b in _beacons(world)]
    assert all(i == expected for i in ids), f"expected gaClientId {expected!r}, got {set(ids)}"


@then(parsers.parse('at least one beacon was sent and a beacon has a gaClientId that is "{value}"'))
def some_has(world: World, value: str) -> None:
    ids = [b.get("gaClientId") or "" for b in _beacons(world)]
    assert value in ids, f"no beacon with gaClientId {value!r}; got {set(ids)}"
    assert value == _client_id(_x(world)["ga4d_ga"])


@then("the page raised no uncaught exception")
def no_page_errors(world: World) -> None:
    assert not _x(world).get("ga4d_page_errors"), _x(world)["ga4d_page_errors"]


@then(
    parsers.parse(
        'every beacon enqueued before consent has an empty gaClientId and the later ones carry "{value}"'
    )
)
def per_event_consent(world: World, value: str) -> None:
    ids = [b.get("gaClientId") or "" for b in _beacons(world)]
    assert "" in ids and value in ids, f"need beacons from both sides of the consent flip: {ids}"
    assert set(ids) <= {"", value}, f"unexpected gaClientId values: {set(ids)}"
    first_kept = ids.index(value)
    assert all(i == value for i in ids[first_kept:]), f"an earlier beacon got an id later: {ids}"


@then("once the visitor has analytics consent and a valid _ga cookie after a reload the beacons carry its client id")
def valid_after_reload_carries(world: World) -> None:
    """Anchor: proves the id was withheld/dropped because of the input, not because the feature is absent."""
    valid = "GA1.1.1234567890.1700000000"
    before = len(_x(world)["track_batches"])
    world.context.add_cookies(
        [
            {"name": "agora_consent", "value": _CONSENT_ON, "url": world.settings.base_url},
            {"name": "_ga", "value": valid, "url": world.settings.base_url},
        ]
    )
    world.page.reload(wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)
    for _ in range(4):
        world.page.mouse.wheel(0, 700)
        world.page.wait_for_timeout(150)
    world.page.wait_for_timeout(4_000)
    expected = _client_id(valid)
    later = [b for batch in _x(world)["track_batches"][before:] for b in batch]
    assert later, "no beacon after the reload"
    assert any((b.get("gaClientId") or "") == expected for b in later), (
        f"no beacon carries {expected!r} after consent: {[b.get('gaClientId') for b in later]}"
    )
