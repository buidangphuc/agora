"""Steps for recs-serving-safeguards (area rss-e2e), through the gateway and the storefront.

Destructive scenarios save every Redis key they write and restore it exactly in teardown; the
Redis outage restores the container and waits until team-ai serves a real model again.
"""

from __future__ import annotations

import json
import subprocess
import time
import uuid

from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from src.constants import PageName, timeouts
from tests.e2e.step_definitions.apr_boot_steps import _docker_run, _env, _image
from tests.e2e.step_definitions.pdp_streaming_steps import _record_beacons
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

RUN_TIMEOUT_S = 60
GRPC_BOOT_S = 90.0
RECOMMEND = "/platform.recommendation.v1.RecommendationService/Recommend"
ROW = "[data-recs-request-id]"


def _x(world: World) -> dict:
    return world.state.extra.setdefault("rss", {})


# ── Redis outage ─────────────────────────────────────────────────────────
@when(
    "the Redis used by team-ai is stopped and a buyer requests homepage recommendations through the gateway"
)
def rss_redis_outage(world: World) -> None:
    def settle() -> None:
        rss.compose("start", "redis")
        pe.wait_healthy(rss.REDIS_CONTAINER, 120)
        rss.await_serving_again(world)

    world.add_cleanup(settle)
    rss.compose("stop", "redis")
    # A call may still be answered from the 5 s pointer memo; every answer must be OK, and the
    # response must settle on the fallback.
    _x(world)["response"] = rss.await_recommend(
        world,
        lambda r: r.get("modelVersion") == rss.FALLBACK_VERSION,
        "Recommend did not answer OK with serving-fallback while Redis is down",
        timeout_s=15,
    )


@then(parsers.parse('the call succeeds with model_version "{version}"'))
def rss_call_succeeds_with_version(world: World, version: str) -> None:
    assert _x(world)["response"].get("modelVersion") == version, _x(world)["response"]


# ── Cold start ───────────────────────────────────────────────────────────
@when(
    "the serving generation's popular list is L and a buyer with no recommendations of their own requests homepage recommendations"
)
def rss_cold_start(world: World) -> None:
    redis = rss.keys(world, rss.SERVING_DB)
    prefix = rss.serving_prefix(redis)
    popular = [f"e2e-rss-pop-{uuid.uuid4().hex[:8]}-{n}" for n in range(6)]
    redis.remove(f"{prefix}:user:{rss.buyer_id(world)}")
    redis.put(
        f"{prefix}:popular",
        json.dumps([{"listing_id": lid, "score": 100.0 - n} for n, lid in enumerate(popular)]),
    )
    _x(world)["popular"] = popular
    _x(world)["response"] = rss.await_recommend(
        world,
        lambda r: rss.ids(r)[:3] == popular[:3],
        "Recommend does not serve the published popular list",
        limit=5,
    )


@then("the returned listing ids are the first items of L, in L's order")
def rss_first_items_of_popular(world: World) -> None:
    popular = _x(world)["popular"]
    got = rss.ids(_x(world)["response"])
    assert got == popular[: len(got)] and len(got) >= 3, (got, popular)


# ── Boot guard ───────────────────────────────────────────────────────────
@when(
    "the team-ai image is started with ENVIRONMENT=production, RECS_ENABLED=true and RECS_BACKEND=memory"
)
def rss_boot_memory(world: World) -> None:
    env = _env("production")
    env.update(RECS_ENABLED="true", RECS_BACKEND="memory")
    name, cmd = _docker_run(world, env, "--rm")
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    except subprocess.TimeoutExpired as exc:
        pe.docker("rm", "-f", name, check=False)
        raise AssertionError(
            f"team-ai kept running with RECS_BACKEND=memory in production: {exc.stdout}"
        ) from exc
    _x(world)["boot"] = proc


@then("the process exits non-zero and its log names RECS_BACKEND")
def rss_boot_refused(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["boot"]
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0, f"exit 0, booted the memory catalogue in production:\n{log}"
    assert "RECS_BACKEND" in log, log[-2000:]


# ── run_grpc entrypoint ──────────────────────────────────────────────────
@when(
    "team-ai is started with scripts/run_grpc.py and a buyer requests recommendations through a gateway pointed at it"
)
def rss_run_grpc(world: World) -> None:
    name = f"e2e-rss-grpc-{uuid.uuid4().hex[:8]}"
    env = {
        k: v
        for k, v in pe.container_env(pe.ai_container()).items()
        if k not in pe._PRIVATE_SKIP_ENV
    }
    port = env.get("GRPC_PORT", "50060")
    args = ["run", "-d", "--name", name, "--network", pe.stack_network()]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    pe.docker(*args, _image(), "python", "scripts/run_grpc.py")
    addr = f"{name}:{port}"
    base = pe.private_gateway(
        world, {"UPSTREAM_RECOMMENDATION_ADDR": addr, "UPSTREAM_AI_ADDR": addr}
    )
    token = world.state.current_user.token
    body = {"context": "RECOMMENDATION_CONTEXT_HOMEPAGE", "limit": 5}
    deadline = time.monotonic() + GRPC_BOOT_S
    while True:
        resp = pe.post_json(RECOMMEND, body, token, base_url=base, timeout=10)
        # Not up yet: the gateway reports the upstream as unavailable until the port answers.
        if resp.status_code != 503 and pe.connect_code(resp) != "unavailable":
            break
        assert time.monotonic() < deadline, f"standalone team-ai never answered: {resp.text}"
        time.sleep(1)
    _x(world)["grpc_resp"] = resp
    _x(world)["grpc_logs"] = pe.docker("logs", name, check=False)


@then("the call succeeds instead of answering unimplemented")
def rss_grpc_ok(world: World) -> None:
    resp = _x(world)["grpc_resp"]
    code = pe.connect_code(resp)
    assert code != "unimplemented", f"Recommend is not registered: {resp.status_code} {resp.text}"
    assert resp.status_code == 200 and code == "ok", f"{resp.status_code} {resp.text}"


# ── Attribution through the gateway ──────────────────────────────────────
@when("a buyer requests homepage recommendations twice through the gateway")
def rss_two_calls(world: World) -> None:
    _x(world)["responses"] = [rss.recommend(world), rss.recommend(world)]


@then(
    'both responses have placement_id "home_feed" and two different non-empty request_ids, and team-ai logged a recs.served line for each'
)
def rss_distinct_request_ids(world: World) -> None:
    responses = _x(world)["responses"]
    request_ids = [r.get("requestId") for r in responses]
    assert all(r.get("placementId") == "home_feed" for r in responses), responses
    assert all(request_ids) and request_ids[0] != request_ids[1], request_ids
    deadline = time.monotonic() + 15
    missing = list(request_ids)
    while missing and time.monotonic() < deadline:
        lines = rss.ai_log_lines("recs.served")
        missing = [rid for rid in request_ids if not any(rid in line for line in lines)]
        if missing:
            time.sleep(1)
    assert not missing, f"no recs.served log line for request id(s) {missing}"


# ── Online features ──────────────────────────────────────────────────────
@when(
    "two candidates have equal model scores and only the second has item_popularity features with ctr_7d 0.5"
)
def rss_tie_break(world: World) -> None:
    serving = rss.keys(world, rss.SERVING_DB)
    features = rss.keys(world, rss.FEATURES_DB)
    first, second = (f"e2e-rss-tie-{uuid.uuid4().hex[:8]}-{n}" for n in ("a", "b"))
    version = features.get("fs:item_popularity:current")
    if not version:
        version = "1"
        features.put("fs:item_popularity:current", version)
    features.put(
        f"fs:item_popularity:v{version}:{second}",
        json.dumps({"ctr_7d": 0.5, "favorites_current": 3}),
    )
    features.remove(f"fs:item_popularity:v{version}:{first}")
    prefix = rss.serving_prefix(serving)
    serving.put(
        f"{prefix}:user:{rss.buyer_id(world)}",
        json.dumps([{"listing_id": first, "score": 0.5}, {"listing_id": second, "score": 0.5}]),
    )
    _x(world)["pair"] = (first, second)
    _x(world)["response"] = rss.await_recommend(
        world,
        lambda r: set(rss.ids(r)) >= {first, second},
        "Recommend does not return the seeded pair",
    )


@then("the second ranks above the first in the response")
def rss_second_above_first(world: World) -> None:
    first, second = _x(world)["pair"]
    got = rss.ids(_x(world)["response"])
    assert got.index(second) < got.index(first), got


# ── Storefront attribution ───────────────────────────────────────────────
@when("a buyer opens the homepage and its recommendation row is shown")
def rss_open_home_with_row(world: World) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id, "no seeded listing in state"
    serving = rss.keys(world, rss.SERVING_DB)
    prefix = rss.serving_prefix(serving)
    own = rss.ids(rss.recommend(world, 10))
    wanted = [listing.listing_id, *[i for i in own if i != listing.listing_id]][:10]
    serving.put(
        f"{prefix}:user:{rss.buyer_id(world)}",
        json.dumps([{"listing_id": i, "score": 10.0 - n} for n, i in enumerate(wanted)]),
    )
    # The pointer is memoised by team-ai: wait until the buyer's own list is what is served.
    rss.await_recommend(world, lambda r: rss.ids(r)[:1] == wanted[:1], "seeded list not served")
    beacons = _record_beacons(world)
    world.navigate_to(PageName.HOME)
    row = world.page.locator(ROW).first
    expect(row).to_be_visible(timeout=timeouts.LONG)
    _x(world)["request_id"] = row.get_attribute("data-recs-request-id")
    row.scroll_into_view_if_needed()
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline and not any(
        b.get("type") == "impression" and b.get("placementId") == "home_feed" for b in beacons
    ):
        world.page.wait_for_timeout(500)


@then(
    'the impression beacons of that row carry an impressionId equal to the request_id the storefront received, and placementId "home_feed"'
)
def rss_beacons_carry_request_id(world: World) -> None:
    request_id = _x(world)["request_id"]
    assert request_id, "the recommendation row carries no data-recs-request-id"
    beacons = [
        b
        for b in world.state.extra["beacons"]
        if b.get("type") == "impression" and b.get("placementId") == "home_feed"
    ]
    assert beacons, f"no home_feed impression beacon in {world.state.extra['beacons']}"
    wrong = [b for b in beacons if b.get("impressionId") != request_id]
    assert not wrong, f"impressionId is not the server request id {request_id}: {wrong}"
