"""AI access control, role-token scopes, login history and the identity boot guard
(change: port-security-hardening; capabilities ai-access-control, auth, deploy-runtime).

Everything goes through the public gateway (Connect JSON) except the boot-guard scenario,
which runs the real team-identity image as a black box with `docker run --rm`.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import uuid
from pathlib import Path
from typing import Any

import httpx
import yaml
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, BaseService
from src.constants import gateway_endpoints as ep
from src.utils import data as fake
from tests.e2e.support.world import World

SETTINGS = get_settings()
REPO_ROOT = Path(__file__).resolve().parents[4]
SERVICE_ONLY_SCOPES = {
    "inventory.write",
    "order.read",
    "promotion.reserve",
    "audit.write",
    "features.read",
    "features.dataset",
}
REDIS_CONTAINER = os.getenv("REDIS_CONTAINER", "agora-redis-1")
IDENTITY_CONTAINER = os.getenv("IDENTITY_CONTAINER", "agora-team-identity-svc")
RECS_USER_KEY = "recs:v1:user:{user_id}"
HOMEPAGE = "RECOMMENDATION_CONTEXT_HOMEPAGE"


def _claims(token: str) -> dict[str, Any]:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))


def _register(role: str, prefix: str) -> tuple[str, str, str]:
    username = fake.unique_username(prefix)
    password = SETTINGS.seed_password
    token = AuthService().register(username, password, role)
    assert token, f"registration of {role} {username} returned no token"
    return username, password, token


def _call(token: str | None, endpoint: str, body: dict[str, Any]) -> httpx.Response:
    svc = BaseService(token=token)
    try:
        return svc.send("POST", endpoint, json_body=body)
    finally:
        svc.close()


def _redis(*args: str) -> None:
    subprocess.run(
        ["docker", "exec", REDIS_CONTAINER, "redis-cli", *args],
        check=True,
        capture_output=True,
        text=True,
        timeout=30,
    )


def _extra(world: World) -> dict[str, Any]:
    return world.state.extra


# ── ai-access-control ──────────────────────────────────────────────────────
@given("a logged-in buyer")
def logged_in_buyer(world: World) -> None:
    _, _, token = _register("buyer", "aibuyer")
    _extra(world)["token"] = token


@given("a logged-in seller")
def logged_in_seller(world: World) -> None:
    _, _, token = _register("seller", "aiseller")
    _extra(world)["token"] = token
    _extra(world)["own_id"] = _claims(token)["sub"]


@given("another seller exists")
def another_seller(world: World) -> None:
    _, _, token = _register("seller", "aiother")
    _extra(world)["other_id"] = _claims(token)["sub"]
    assert _extra(world)["other_id"] != _extra(world)["own_id"]


@when("the buyer calls MagicListing through the gateway")
def buyer_magic_listing(world: World) -> None:
    _extra(world)["response"] = _call(
        _extra(world)["token"], ep.AI_MAGIC_LISTING, {"titleHint": "Laptop Dell XPS 13"}
    )


@when("the seller calls ChatCopilot through the gateway with the other seller's seller_id")
def seller_copilot_as_other(world: World) -> None:
    _extra(world)["response"] = _call(
        _extra(world)["token"],
        ep.AI_CHAT_COPILOT,
        {"sellerId": _extra(world)["other_id"], "buyerMessage": "Sản phẩm còn hàng không?"},
    )


@then(parsers.parse("the gateway answers HTTP {status:d}"))
def gateway_answers(world: World, status: int) -> None:
    resp: httpx.Response = _extra(world)["response"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then("the copilot returns no quick replies")
def no_quick_replies(world: World) -> None:
    resp: httpx.Response = _extra(world)["response"]
    assert not resp.json().get("quickReplies"), resp.text


@given("two buyers with distinct precomputed recommendation lists")
def two_buyers_with_lists(world: World) -> None:
    extra = _extra(world)
    run = uuid.uuid4().hex[:8]
    for who in ("a", "b"):
        _, _, token = _register("buyer", f"rec{who}")
        user_id = _claims(token)["sub"]
        ids = [f"e2e-{run}-{who}-{n}" for n in (1, 2, 3)]
        key = RECS_USER_KEY.format(user_id=user_id)
        _redis("set", key, json.dumps(ids), "EX", "600")
        world.add_cleanup(lambda k=key: _redis("del", k))
        extra[f"{who}_token"], extra[f"{who}_id"], extra[f"{who}_ids"] = token, user_id, ids
    assert extra["a_id"] != extra["b_id"]


def _recommend(token: str, user_id: str) -> list[str]:
    resp = _call(
        token, ep.RECOMMENDATION_RECOMMEND, {"userId": user_id, "context": HOMEPAGE, "limit": 5}
    )
    assert resp.status_code == httpx.codes.OK, resp.text
    items = sorted(resp.json().get("items") or [], key=lambda it: it.get("rank", 0))
    return [it["listingId"] for it in items]


@when("buyer A calls Recommend through the gateway naming buyer B's user_id")
def a_recommends_as_b(world: World) -> None:
    extra = _extra(world)
    extra["served"] = _recommend(extra["a_token"], extra["b_id"])


@then("the recommendations returned are buyer A's own list")
def served_is_a(world: World) -> None:
    extra = _extra(world)
    assert extra["served"] == extra["a_ids"], extra["served"]


@then("none of buyer B's recommended listings are returned")
def served_not_b(world: World) -> None:
    extra = _extra(world)
    assert not set(extra["served"]) & set(extra["b_ids"]), extra["served"]


@then("buyer B calling Recommend as themselves gets buyer B's list")
def b_gets_b(world: World) -> None:
    extra = _extra(world)
    assert _recommend(extra["b_token"], extra["b_id"]) == extra["b_ids"]


# ── auth ───────────────────────────────────────────────────────────────────
@given("a registered buyer account")
def registered_buyer_account(world: World) -> None:
    username, password, _ = _register("buyer", "authbuyer")
    _extra(world).update(username=username, password=password)


@when("the buyer logs in through the gateway")
def buyer_logs_in(world: World) -> None:
    extra = _extra(world)
    extra["token"] = AuthService().login(extra["username"], extra["password"])


@then("the token scopes include recommendations:read and ai:use")
def token_has_ai_scopes(world: World) -> None:
    scopes = set(_claims(_extra(world)["token"]).get("scopes", []))
    assert {"recommendations:read", "ai:use"} <= scopes, scopes


@then("the token scopes include none of the service-only scopes")
def token_has_no_service_scopes(world: World) -> None:
    scopes = set(_claims(_extra(world)["token"]).get("scopes", []))
    assert not scopes & SERVICE_ONLY_SCOPES, scopes & SERVICE_ONLY_SCOPES


@when("the buyer logs in once with a wrong password and then with the right one")
def wrong_then_right_login(world: World) -> None:
    extra = _extra(world)
    wrong = _call(
        None, ep.AUTH_LOGIN, {"username": extra["username"], "password": "not-the-password"}
    )
    assert wrong.status_code == httpx.codes.UNAUTHORIZED, wrong.text
    extra["token"] = AuthService().login(extra["username"], extra["password"])


@when("the buyer lists their login history through the gateway")
def list_login_history(world: World) -> None:
    resp = _call(_extra(world)["token"], ep.SESSION_LOGIN_HISTORY, {})
    assert resp.status_code == httpx.codes.OK, resp.text
    _extra(world)["events"] = resp.json().get("events") or []


@then("the history contains a failure entry followed by a success entry")
def history_failure_then_success(world: World) -> None:
    events = sorted(_extra(world)["events"], key=lambda e: e["createdAt"])
    outcomes = [bool(e.get("success")) for e in events]
    assert outcomes == [False, True], events


# ── deploy-runtime: identity boot guard ────────────────────────────────────
def _identity_image() -> str:
    """The team-identity image the running stack uses (the `team-identity:local` tag can be
    stale when the stack builds under a compose project prefix); override with IDENTITY_IMAGE."""
    if os.getenv("IDENTITY_IMAGE"):
        return os.environ["IDENTITY_IMAGE"]
    out = subprocess.run(
        ["docker", "inspect", IDENTITY_CONTAINER, "--format", "{{.Config.Image}}"],
        capture_output=True,
        text=True,
        timeout=30,
    )
    return (
        out.stdout.strip() if out.returncode == 0 and out.stdout.strip() else "team-identity:local"
    )


@when(
    "the team-identity image is started with ENV=production and the development signing key "
    "from the local compose file"
)
def start_identity_in_production(world: World) -> None:
    compose = yaml.safe_load((REPO_ROOT / "docker-compose.services.yaml").read_text())
    env = dict(
        e.split("=", 1)
        for e in compose["services"]["team-identity"]["environment"]
        if isinstance(e, str) and not e.startswith("#")
    )
    assert "JWT_PRIVATE_KEY" in env and env.get("JWT_KID"), "compose has no dev signing key env"
    env["ENV"] = "production"
    name = f"e2e-identity-boot-guard-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(
        lambda: subprocess.run(["docker", "rm", "-f", name], capture_output=True, timeout=30)
    )
    cmd = ["docker", "run", "--rm", "--name", name, "--network", SETTINGS.stack_network]
    cmd += [arg for k, v in env.items() for arg in ("-e", f"{k}={v}")]
    cmd.append(_identity_image())
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    _extra(world)["boot"] = result


@then("the process exits non-zero and its log names the signing key")
def boot_refused(world: World) -> None:
    result: subprocess.CompletedProcess[str] = _extra(world)["boot"]
    log = result.stdout + result.stderr
    assert result.returncode != 0, f"identity booted in production with the dev key:\n{log}"
    assert "refusing to start" in log and ("JWT_KID" in log or "JWT_PRIVATE_KEY" in log), log
