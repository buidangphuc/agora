"""Gateway principal log fingerprint steps (gateway-principal-log-fingerprint).

Requests are made over plain Connect JSON with a fresh X-Request-Id; the matching
`edge.request` line is read back from the team-gateway container log (`docker logs`).
"""

from __future__ import annotations

import base64
import json
import re
import time
import uuid
from datetime import datetime, timezone

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

LISTING_ROUTE = "/platform.listing.v1.ListingService/ListCategories"
FP_RE = re.compile(r"^[0-9a-f]{12}$")
LOG_WAIT_S = 15.0


def _x(world: World) -> dict:
    return world.state.extra


def _jwt_subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    claims = json.loads(base64.urlsafe_b64decode(payload))
    sub = claims.get("sub") or claims.get("user_id") or claims.get("uid")
    assert sub, f"no subject claim in the token (claims: {sorted(claims)})"
    return str(sub)


def _since(world: World) -> str:
    return _x(world)["fp_started"]


def _gateway_log(since: str) -> str:
    proc = pe.docker("logs", "--since", since, pe.gateway_container(), check=False)
    return proc.stdout + proc.stderr


def _edge_line(since: str, request_id: str) -> dict | None:
    for raw in _gateway_log(since).splitlines():
        if request_id not in raw or '"edge.request"' not in raw:
            continue
        try:
            line = json.loads(raw)
        except ValueError:
            continue
        if line.get("msg") == "edge.request" and line.get("request_id") == request_id:
            return line
    return None


def _await_line(world: World, request_id: str) -> dict:
    deadline = time.monotonic() + LOG_WAIT_S
    while True:
        line = _edge_line(_since(world), request_id)
        if line is not None:
            return line
        if time.monotonic() > deadline:
            raise AssertionError(
                f"no edge.request line with request_id={request_id} in the "
                f"{pe.gateway_container()} log within {LOG_WAIT_S:.0f}s"
            )
        time.sleep(0.5)


def _call(token: str | None, request_id: str) -> httpx.Response:
    return pe.post_json(LISTING_ROUTE, {}, token=token, headers={"X-Request-Id": request_id})


@given("a freshly registered user")
def first_user(world: World) -> None:
    x = _x(world)
    x["fp_started"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")
    token = pe.register(world, "buyer")
    x["fp_users"] = [{"token": token, "id": _jwt_subject(token), "lines": []}]


@given("a second freshly registered user")
def second_user(world: World) -> None:
    token = pe.register(world, "buyer")
    _x(world)["fp_users"].append({"token": token, "id": _jwt_subject(token), "lines": []})


@given("the user calls an authenticated listing route with a fresh X-Request-Id")
@when("the user calls an authenticated listing route with a fresh X-Request-Id")
def user_call(world: World) -> None:
    x = _x(world)
    user = x["fp_users"][0]
    rid = pe.rid("e2e-fp")
    resp = _call(user["token"], rid)
    assert resp.status_code == 200, (resp.status_code, resp.text[:200])
    x["fp_rid"] = rid
    line = _await_line(world, rid)
    user["lines"].append(line)
    x["fp_user_line"] = line
    x["fp_line"] = line


@when("an anonymous caller calls a public listing route with a fresh X-Request-Id")
def anonymous_call(world: World) -> None:
    x = _x(world)
    rid = pe.rid("e2e-fp")
    resp = _call(None, rid)
    assert resp.status_code == 200, (resp.status_code, resp.text[:200])
    x["fp_rid"] = rid


@when("a caller with an invalid bearer token calls a listing route with a fresh X-Request-Id")
def invalid_token_call(world: World) -> None:
    x = _x(world)
    rid = pe.rid("e2e-fp")
    x["fp_rid"] = rid
    x["fp_resp"] = _call("not-a-valid-token-" + uuid.uuid4().hex, rid)


@when("each user calls an authenticated listing route with a fresh X-Request-Id")
def each_user_call(world: World) -> None:
    for user in _x(world)["fp_users"]:
        rid = pe.rid("e2e-fp")
        resp = _call(user["token"], rid)
        assert resp.status_code == 200, (resp.status_code, resp.text[:200])
        user["lines"].append(_await_line(world, rid))


@when("the first user calls the route again with a fresh X-Request-Id")
def first_user_again(world: World) -> None:
    user = _x(world)["fp_users"][0]
    rid = pe.rid("e2e-fp")
    resp = _call(user["token"], rid)
    assert resp.status_code == 200, (resp.status_code, resp.text[:200])
    user["lines"].append(_await_line(world, rid))


@then("the call is refused as unauthenticated")
def refused(world: World) -> None:
    resp: httpx.Response = _x(world)["fp_resp"]
    assert resp.status_code == 401, (resp.status_code, resp.text[:200])


@then("the gateway log has an edge.request line for that request id")
def line_present(world: World) -> None:
    x = _x(world)
    x["fp_line"] = _await_line(world, x["fp_rid"])


@then(parsers.parse('that line has principal_kind "{kind}" and a 12 lowercase hex principal_fp'))
def line_user_fp(world: World, kind: str) -> None:
    line = _x(world)["fp_line"]
    assert line.get("principal_kind") == kind, line
    assert FP_RE.match(str(line.get("principal_fp", ""))), line


@then(parsers.parse('that line has principal_kind "{kind}" and no principal_fp'))
def line_no_fp(world: World, kind: str) -> None:
    line = _x(world)["fp_line"]
    assert line.get("principal_kind") == kind, line
    assert "principal_fp" not in line, line


@then("that line has no principal key")
def line_no_principal(world: World) -> None:
    line = _x(world)["fp_line"]
    assert "principal" not in line, line


@then("the earlier user request line carried a principal_fp")
def control_fp(world: World) -> None:
    line = _x(world)["fp_user_line"]
    assert FP_RE.match(str(line.get("principal_fp", ""))), line
    assert line.get("principal_kind") == "user", line


@then("the gateway log since the scenario started does not contain the user's id")
def no_raw_id(world: World) -> None:
    x = _x(world)
    uid = x["fp_users"][0]["id"]
    log = _gateway_log(_since(world))
    assert x["fp_rid"] in log, "the request under test is not in the captured log"
    assert uid not in log, "the raw user id appears in the gateway log"


@then("the gateway container has a non-empty LOG_KEY_FP_SECRET")
def secret_configured(world: World) -> None:
    secret = pe.container_env(pe.gateway_container()).get("LOG_KEY_FP_SECRET", "")
    assert secret.strip(), "LOG_KEY_FP_SECRET is not set on the gateway container"
    _x(world)["fp_secret"] = secret


@then("the gateway log since the scenario started does not contain the LOG_KEY_FP_SECRET value")
def secret_not_logged(world: World) -> None:
    x = _x(world)
    log = _gateway_log(_since(world))
    assert x["fp_rid"] in log, "the request under test is not in the captured log"
    assert x["fp_secret"] not in log, "the fingerprint secret appears in the gateway log"


@then("both users' lines carry a 12 lowercase hex principal_fp")
def both_fp(world: World) -> None:
    for user in _x(world)["fp_users"]:
        line = user["lines"][0]
        assert line.get("principal_kind") == "user", line
        assert FP_RE.match(str(line.get("principal_fp", ""))), line


@then("the two users' principal_fp values differ")
def fps_differ(world: World) -> None:
    a, b = (u["lines"][0].get("principal_fp") for u in _x(world)["fp_users"])
    assert a and b and a != b, (a, b)


@then("the first user's two lines carry the same principal_fp")
def fps_same(world: World) -> None:
    lines = _x(world)["fp_users"][0]["lines"]
    assert len(lines) == 2, lines
    assert lines[0].get("principal_fp") and lines[0].get("principal_fp") == lines[1].get(
        "principal_fp"
    ), lines
