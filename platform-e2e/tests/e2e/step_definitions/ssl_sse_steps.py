"""SSE route token lifetime (sse-stream-lifetime, capability edge-stream-and-http-policy).

Black box through the gateway: a real HTTP client reads /api/events/live as a stream. An
authenticated room (`user:{sub}`) must end with a final `event: unauthenticated` and a clean close
when the token expires or its session is revoked; a public room is never cut.

Expiry uses a token signed with the LOCAL dev identity key (exp set short). Revocation uses real
logins, two sessions of one buyer, and a private gateway (stack image + env) whose
STREAM_REVOCATION_CHECK_SECONDS is 1.
"""

from __future__ import annotations

import base64
import json
import threading
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, SessionService
from src.constants import gateway_endpoints as ep
from tests.e2e.flows import session_id_of
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.edge_tokens import sign_dev_token
from tests.e2e.support.world import World

LIVE_CONSUMER_S = 30.0
EXPIRY_SLACK_S = 4.0
REVOKE_CUT_S = 4.0
PUBLIC_HOLD_S = 6.0
LISTEN_LIMIT_S = 25.0


def _x(world: World) -> dict:
    return world.state.extra


def _sub_of(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


class _Listener:
    """Reads one SSE connection in a thread, recording its lines and when it closed."""

    def __init__(self, base: str, room: str, token: str | None) -> None:
        self.lines: list[str] = []
        self.closed_at: float | None = None
        self.error: Exception | None = None
        self.opened = threading.Event()
        self.status = 0
        headers = {"Authorization": f"bearer {token}"} if token else {}
        self._thread = threading.Thread(
            target=self._run, args=(f"{base}/api/events/live?room={room}", headers), daemon=True
        )
        self._thread.start()
        assert self.opened.wait(15), "the SSE connection never opened"
        assert self.status == httpx.codes.OK, f"SSE open answered {self.status}"

    def _run(self, url: str, headers: dict) -> None:
        try:
            with httpx.stream("GET", url, headers=headers, timeout=LISTEN_LIMIT_S + 5) as resp:
                self.status = resp.status_code
                self.opened.set()
                for line in resp.iter_lines():
                    self.lines.append(line)
        except Exception as exc:  # noqa: BLE001 - recorded for the assertion
            self.error = exc
            self.opened.set()
        finally:
            self.closed_at = time.monotonic()

    def wait_closed(self, timeout: float) -> bool:
        self._thread.join(timeout)
        return self.closed_at is not None

    @property
    def text(self) -> str:
        return "\n".join(self.lines)


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("a buyer token that expires in {seconds:d} seconds for the SSE route"))
def sse_short_token(world: World, seconds: int) -> None:
    now = int(time.time())
    sub = f"e2e-ssl-{uuid.uuid4().hex[:10]}"
    _x(world)["ssl_room"] = f"user:{sub}"
    _x(world)["ssl_exp_at"] = time.monotonic() + seconds
    _x(world)["ssl_token"] = sign_dev_token(
        {
            "sub": sub,
            "typ": "user",
            "name": "e2e-ssl",
            "scopes": ["recommendations:read"],
            "iat": now,
            "exp": now + seconds,
        }
    )


@given(
    "a buyer who is signed in on two devices and a gateway that checks revocation "
    "every second for the SSE route"
)
def sse_two_devices_fast_gateway(world: World) -> None:
    x = _x(world)
    password = get_settings().seed_password
    username = f"e2e_ssl_{uuid.uuid4().hex[:10]}"
    assert AuthService().register(username, password, "buyer"), "could not register a buyer"
    other = AuthService().login(username, password)
    current = AuthService().login(username, password)
    probe = AuthService().login(username, password)
    assert len({session_id_of(t) for t in (other, current, probe)}) == 3
    base = pe.private_gateway(world, {"STREAM_REVOCATION_CHECK_SECONDS": "1"})
    # The private gateway's revocation consumer starts from the earliest offset when it boots:
    # revoke a throwaway session and wait until this gateway rejects it, so the denylist is live.
    svc = SessionService(token=current)
    try:
        svc.revoke_session(session_id_of(probe))
    finally:
        svc.close()
    deadline = time.monotonic() + LIVE_CONSUMER_S
    while time.monotonic() < deadline:
        r = httpx.post(
            f"{base}{ep.SESSION_LIST}",
            json={},
            headers={"Authorization": f"bearer {probe}", "Content-Type": "application/json"},
            timeout=10,
        )
        if r.status_code == httpx.codes.UNAUTHORIZED:
            break
        time.sleep(0.25)
    else:
        raise TimeoutError("the private gateway never applied the probe revocation")
    x.update(
        ssl_base=base,
        ssl_other=other,
        ssl_current=current,
        ssl_room=f"user:{_sub_of(other)}",
    )


@given("an anonymous client with no credential")
def sse_anonymous(world: World) -> None:
    _x(world)["ssl_token"] = None


# ── Whens ────────────────────────────────────────────────────────────────
@when("the buyer opens the SSE route for their own user room and keeps it open")
def sse_open_own_room(world: World) -> None:
    x = _x(world)
    x["ssl_listener"] = _Listener(pe.gateway_url(), x["ssl_room"], x["ssl_token"])


@when(
    "the buyer opens the SSE route for their own user room on the other device "
    "and then revokes that device's session"
)
def sse_open_then_revoke(world: World) -> None:
    x = _x(world)
    listener = _Listener(x["ssl_base"], x["ssl_room"], x["ssl_other"])
    # The handshake event proves the connection is registered at the gateway.
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline and "event: connected" not in listener.text:
        time.sleep(0.1)
    assert "event: connected" in listener.text, "no handshake before the revoke"
    svc = SessionService(token=x["ssl_current"])
    try:
        svc.revoke_session(session_id_of(x["ssl_other"]))
    finally:
        svc.close()
    x.update(ssl_listener=listener, ssl_revoked_at=time.monotonic())


@when(
    parsers.parse(
        "the client opens the SSE route for a public listing room and stays connected "
        "for {seconds:d} seconds"
    )
)
def sse_public_hold(world: World, seconds: int) -> None:
    x = _x(world)
    listener = _Listener(pe.gateway_url(), f"listing:e2e-ssl-{uuid.uuid4().hex[:8]}", None)
    x["ssl_listener"] = listener
    x["ssl_closed_early"] = listener.wait_closed(seconds)


# ── Thens ────────────────────────────────────────────────────────────────
def _assert_cut(listener: _Listener) -> None:
    assert listener.wait_closed(LISTEN_LIMIT_S), "the SSE connection was never closed"
    assert listener.error is None, f"the connection broke instead of closing: {listener.error!r}"
    lines = [ln for ln in listener.lines if ln]
    assert lines[-2:-1] == ["event: unauthenticated"], listener.text
    data = json.loads(lines[-1].removeprefix("data: "))
    assert data["code"] == "unauthenticated", data


@then(
    "the gateway sends an unauthenticated event and closes the connection "
    "about when the token expires"
)
def sse_cut_at_expiry(world: World) -> None:
    x = _x(world)
    listener: _Listener = x["ssl_listener"]
    _assert_cut(listener)
    assert listener.closed_at is not None
    late = listener.closed_at - x["ssl_exp_at"]
    assert late <= EXPIRY_SLACK_S, f"closed {late:.1f}s after the token expired"


@then(
    parsers.parse(
        "the gateway sends an unauthenticated event and closes the connection "
        "within {seconds:d} seconds of the revoke"
    )
)
def sse_cut_after_revoke(world: World, seconds: int) -> None:
    x = _x(world)
    listener: _Listener = x["ssl_listener"]
    _assert_cut(listener)
    assert listener.closed_at is not None
    took = listener.closed_at - x["ssl_revoked_at"]
    assert took <= seconds, f"closed {took:.1f}s after the revoke"


@then("the connection is still open and received no unauthenticated event")
def sse_public_untouched(world: World) -> None:
    x = _x(world)
    listener: _Listener = x["ssl_listener"]
    assert not x["ssl_closed_early"], f"the public connection closed: {listener.text!r}"
    assert "unauthenticated" not in listener.text, listener.text
    assert "event: connected" in listener.text, listener.text
