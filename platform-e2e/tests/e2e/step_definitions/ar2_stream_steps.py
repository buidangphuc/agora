"""StreamChat token lifetime (authz-residuals-2, capability edge-stream-and-http-policy).

Black box through the gateway with the Connect streaming envelope (see
tests/e2e/support/pear_edge_support.py). The model provider is the LLM fake: the directive
`[[fake primary=hang fb1=hang fb2=hang]]` makes every provider call hang, so the stream is still
open when the token expires or its session is revoked. team-ai's own first-token timeout is 2 s
per target with 3 attempts (llm-fake overlay), so without the gateway's cut the stream would
end after about 6 s with `unavailable`, never `unauthenticated`.

Expiry uses a token signed with the LOCAL dev identity key (exp set short). Revocation uses real
logins, two sessions of one buyer, and a private gateway (stack image + env) whose
STREAM_REVOCATION_CHECK_SECONDS is 1; the stack's own gateway keeps the default of 5.
"""

from __future__ import annotations

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

HOLD = "[[fake primary=hang fb1=hang fb2=hang]]"
# team-ai's chain (2 s first-token timeout x 3 attempts) ends a held stream after about 6 s.
PROVIDER_GIVES_UP_S = 5.0
REVOKE_CUT_S = 4.0
LIVE_CONSUMER_S = 30.0


def _x(world: World) -> dict:
    return world.state.extra


def _short_token(seconds: int) -> str:
    now = int(time.time())
    return sign_dev_token(
        {
            "sub": f"e2e-ar2-{uuid.uuid4().hex[:10]}",
            "typ": "user",
            "name": "e2e-ar2",
            "scopes": ["ai:use", "recommendations:read"],
            "iat": now,
            "exp": now + seconds,
        }
    )


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("a buyer token that expires in {seconds:d} seconds"))
def short_lived_token(world: World, seconds: int) -> None:
    _x(world)["ar2_token"] = _short_token(seconds)


@given("a buyer who is signed in on two devices and a gateway that checks revocation every second")
def two_devices_fast_gateway(world: World) -> None:
    x = _x(world)
    password = get_settings().seed_password
    username = f"e2e_ar2_{uuid.uuid4().hex[:10]}"
    assert AuthService().register(username, password, "buyer"), "could not register a buyer"
    other = AuthService().login(username, password)
    current = AuthService().login(username, password)
    probe = AuthService().login(username, password)
    assert len({session_id_of(t) for t in (other, current, probe)}) == 3
    base = pe.private_gateway(world, {"STREAM_REVOCATION_CHECK_SECONDS": "1"})
    # The private gateway's revocation consumer starts from the earliest offset when it boots.
    # Revoke a throwaway third session and wait until this gateway rejects it: the denylist is live.
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
    x.update(ar2_base=base, ar2_other=other, ar2_current=current)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the buyer opens a StreamChat that the model provider holds open")
def open_held_stream(world: World) -> None:
    x = _x(world)
    started = time.monotonic()
    x["ar2_result"] = pe.stream_chat(x["ar2_token"], f"hello {HOLD}")
    x["ar2_elapsed"] = time.monotonic() - started


@when(
    "the buyer opens a StreamChat on the other device that the model provider holds open "
    "and then revokes that device's session"
)
def open_then_revoke(world: World) -> None:
    x = _x(world)
    box: dict = {}

    def run() -> None:
        box["result"] = pe.stream_chat(x["ar2_other"], f"hello {HOLD}", base_url=x["ar2_base"])
        box["ended"] = time.monotonic()

    worker = threading.Thread(target=run, daemon=True)
    worker.start()
    # Revoke once the stream is open: the held stream cannot answer, so give the gateway a
    # moment to forward it (a signal-free wait is not possible, the stream has no response yet).
    time.sleep(1.0)
    svc = SessionService(token=x["ar2_current"])
    try:
        svc.revoke_session(session_id_of(x["ar2_other"]))
    finally:
        svc.close()
    revoked_at = time.monotonic()
    worker.join(timeout=60)
    assert not worker.is_alive(), "the held stream never ended"
    x.update(ar2_result=box["result"], ar2_after_revoke=box["ended"] - revoked_at)


@when("the buyer calls StreamChat and the provider answers at once")
def stream_answers(world: World) -> None:
    x = _x(world)
    x["ar2_result"] = pe.stream_chat(x["ar2_token"], "hello")


# ── Thens ────────────────────────────────────────────────────────────────
@then(
    "the stream ends with the Connect error code unauthenticated "
    "before the provider's own timeouts end it"
)
def ended_at_expiry(world: World) -> None:
    x = _x(world)
    res: pe.StreamResult = x["ar2_result"]
    assert res.code == "unauthenticated", (res.http_status, res.code, res.raw[:300])
    assert (
        x["ar2_elapsed"] < PROVIDER_GIVES_UP_S
    ), f"the stream lasted {x['ar2_elapsed']:.1f}s, the cut should come near the 3s expiry"


@then(
    parsers.parse(
        "the stream ends with the Connect error code unauthenticated "
        "within {seconds:d} seconds of the revoke"
    )
)
def ended_after_revoke(world: World, seconds: int) -> None:
    x = _x(world)
    res: pe.StreamResult = x["ar2_result"]
    assert res.code == "unauthenticated", (res.http_status, res.code, res.raw[:300])
    assert x["ar2_after_revoke"] <= seconds, f"ended {x['ar2_after_revoke']:.1f}s after the revoke"


@then("the stream completes with chat text and no error")
def completed(world: World) -> None:
    res: pe.StreamResult = _x(world)["ar2_result"]
    assert res.code == "ok" and res.streamed, (res.http_status, res.code, res.raw[:300])
