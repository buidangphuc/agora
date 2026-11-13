"""Steps for recs-experiment-arms (area era-e2e), through the gateway and the stack's Flipt.

The flag is seeded with team-ai's seed script and driven over Flipt's REST API; teardown always
leaves it deleted and re-seeded (disabled, no rules) and Flipt running.
"""

from __future__ import annotations

import json
import re
import time
import uuid
from typing import Any

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services.recommendation_service import CONTEXT_HOMEPAGE
from src.constants import gateway_endpoints as ep
from config.settings import get_settings
from tests.e2e.flows import flipt_flow as ff
from tests.e2e.flows.stack_flow import stop_container
from tests.e2e.step_definitions import in_stock_recommendations_steps as isr
from tests.e2e.support import cart_cross_sell_support as cx
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

FLAG = ff.EXPERIMENT_FLAG
ARM_CONTROL = f"{FLAG}:control"
ARM_SIMILARITY = f"{FLAG}:similarity"
LADDER_TIERS = {"user_precomputed", "seed_vector_similarity", "popular", "global_popular", "user_vector"}
ROLLOUT_SETTLE_S = 20.0


def _x(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("era", {})


def segments(response: dict[str, Any]) -> tuple[str, str, str, str]:
    version = response.get("modelVersion", "")
    parts = version.split("/")
    assert len(parts) == 4, f"model_version {version!r} is not <generation>/<tier>/<ranker>/<arm>"
    return parts[0], parts[1], parts[2], parts[3]


def anonymous_recommend(anon: str) -> dict[str, Any]:
    resp = httpx.post(
        get_settings().gateway_url.rstrip("/") + ep.RECOMMENDATION_RECOMMEND,
        json={"anonymousId": anon, "context": CONTEXT_HOMEPAGE, "limit": 10},
        headers={"Content-Type": "application/json"},
        timeout=15.0,
    )
    resp.raise_for_status()
    return resp.json()


def _seed_flag(world: World) -> None:
    world.add_cleanup(ff.restore_experiment_flag_disabled)
    seeded = ff.run_seed_script()
    assert seeded.returncode == 0, f"seed script failed: {seeded.stdout}{seeded.stderr}"
    assert ff.get_flag() is not None, "seed script did not create the flag"


# ── Flag state ───────────────────────────────────────────────────────────
@given("the experiment flag is enabled with a 50/50 split")
def era_flag_enabled(world: World) -> None:
    _seed_flag(world)
    ff.set_even_rollout()
    ff.set_flag_enabled(True)


@given("the experiment flag exists and is disabled")
def era_flag_disabled(world: World) -> None:
    _seed_flag(world)
    ff.set_flag_enabled(False)


@given("the experiment flag does not exist")
def era_flag_absent(world: World) -> None:
    world.add_cleanup(ff.restore_experiment_flag_disabled)
    ff.delete_flag()
    assert ff.get_flag() is None


# ── Assignment ───────────────────────────────────────────────────────────
@when("one anonymous visitor requests homepage recommendations 5 times through the gateway")
def era_five_calls(world: World) -> None:
    anon = f"e2e-era-{uuid.uuid4().hex}"
    first = _await_assigned(anon)
    _x(world)["responses"] = [first] + [anonymous_recommend(anon) for _ in range(4)]


def _await_assigned(anon: str) -> dict[str, Any]:
    deadline = time.monotonic() + ROLLOUT_SETTLE_S
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        last = anonymous_recommend(anon)
        if segments(last)[3] in (ARM_CONTROL, ARM_SIMILARITY):
            return last
        time.sleep(rss.POLL_S)
    raise AssertionError(f"visitor never got an arm of {FLAG}; last {last.get('modelVersion')!r}")


@then(
    "all 5 responses carry the same arm segment, control or similarity of recs-home-feed-ranker"
)
def era_same_arm(world: World) -> None:
    arms = {segments(r)[3] for r in _x(world)["responses"]}
    assert len(arms) == 1 and arms <= {ARM_CONTROL, ARM_SIMILARITY}, arms


@when("40 distinct anonymous visitors request homepage recommendations")
def era_forty_visitors(world: World) -> None:
    _await_assigned(f"e2e-era-{uuid.uuid4().hex}")
    _x(world)["responses"] = [
        anonymous_recommend(f"e2e-era-{uuid.uuid4().hex}") for _ in range(40)
    ]


@then(
    parsers.parse(
        'the responses contain both arm "{control}" and arm "{similarity}"'
    )
)
def era_both_arms(world: World, control: str, similarity: str) -> None:
    arms = {segments(r)[3] for r in _x(world)["responses"]}
    assert {control, similarity} <= arms, arms


@when("a buyer requests homepage recommendations through the gateway")
def era_buyer_recommend(world: World) -> None:
    _x(world)["response"] = rss.recommend(world)


@then(parsers.parse('the arm segment of model_version is "{arm}"'))
def era_arm_is(world: World, arm: str) -> None:
    assert segments(_x(world)["response"])[3] == arm, _x(world)["response"]


@when("visitors are requested until one is assigned the similarity variant")
def era_until_similarity(world: World) -> None:
    deadline = time.monotonic() + ROLLOUT_SETTLE_S
    while time.monotonic() < deadline:
        response = anonymous_recommend(f"e2e-era-{uuid.uuid4().hex}")
        if segments(response)[3] == ARM_SIMILARITY:
            _x(world)["response"] = response
            return
    raise AssertionError("no visitor was assigned the similarity variant")


@then(parsers.parse('the ranker segment of that response is "{ranker}"'))
def era_ranker_is(world: World, ranker: str) -> None:
    assert segments(_x(world)["response"])[2] == ranker, _x(world)["response"]


@then("the tier segment of that response is one the default ladder produces")
def era_tier_in_ladder(world: World) -> None:
    assert segments(_x(world)["response"])[1] in LADDER_TIERS, _x(world)["response"]


# ── Flipt down ───────────────────────────────────────────────────────────
@when("Flipt is stopped and a buyer requests homepage recommendations through the gateway")
def era_flipt_down(world: World) -> None:
    def settle() -> None:
        restore()
        ff.wait_flipt_healthy()

    restore = stop_container(ff.flipt_container())
    world.add_cleanup(settle)
    _x(world)["response"] = rss.recommend(world)


@then(
    'the call succeeds, the ranker segment is fixed_weights or gbdt, and the arm segment is "recs-home-feed-ranker:unassigned"'
)
def era_unassigned(world: World) -> None:
    _, _, ranker, arm = segments(_x(world)["response"])
    assert ranker in {"fixed_weights", "gbdt"}, _x(world)["response"]
    assert arm == f"{FLAG}:unassigned", _x(world)["response"]


# ── Seeding ──────────────────────────────────────────────────────────────
@when("the seed script runs twice")
def era_seed_twice(world: World) -> None:
    first = ff.run_seed_script()
    created = ff.get_flag()
    second = ff.run_seed_script()
    _x(world).update(runs=(first, second), created=created, after=ff.get_flag())


@then("the flag exists with enabled false and the variants control and similarity")
def era_flag_shape(world: World) -> None:
    flag = _x(world)["created"]
    assert flag is not None, "the seed script did not create the flag"
    assert flag.get("enabled") is False, flag
    assert {v["key"] for v in flag.get("variants", [])} == {"control", "similarity"}, flag


@then("both runs exited 0 and the second changed nothing")
def era_seed_idempotent(world: World) -> None:
    first, second = _x(world)["runs"]
    assert (first.returncode, second.returncode) == (0, 0), (first.stderr, second.stderr)
    assert _x(world)["after"] == _x(world)["created"], (_x(world)["created"], _x(world)["after"])


# ── model_version shape ──────────────────────────────────────────────────
@when(
    "a buyer with a precomputed list requests homepage recommendations through the gateway"
)
def era_precomputed(world: World) -> None:
    serving = rss.keys(world, rss.SERVING_DB)
    pointer = serving.get(rss.SERVING_KEY)
    assert pointer, "the stack serves no generation pointer"
    prefix = rss.serving_prefix(serving)
    listed = [f"e2e-era-{uuid.uuid4().hex[:8]}-{n}" for n in range(5)]
    serving.put(
        f"{prefix}:user:{rss.buyer_id(world)}",
        json.dumps(
            [
                {"listing_id": lid, "score": 10.0 - n, "visible": True}
                for n, lid in enumerate(listed)
            ]
        ),
    )
    _x(world).update(pointer=pointer, response=rss.await_recommend(
        world,
        lambda r: rss.ids(r)[:1] == listed[:1],
        "Recommend does not serve the buyer's precomputed list",
    ))


@then("model_version matches the serving generation, user_precomputed, a ranker and an arm")
def era_user_tier(world: World) -> None:
    generation, tier, ranker, arm = segments(_x(world)["response"])
    assert generation == _x(world)["pointer"], _x(world)["response"]
    assert tier == "user_precomputed", _x(world)["response"]
    assert ranker in {"fixed_weights", "gbdt", "similarity"} and arm, _x(world)["response"]


@then(
    "model_version matches the serving generation, seed_vector_similarity, similarity and none"
)
def era_seed_tier(world: World) -> None:
    answer = isr._st(world)["answer"]
    pointer = rss.keys(world, rss.SERVING_DB).get(rss.SERVING_KEY)
    assert segments(answer) == (pointer, "seed_vector_similarity", "similarity", "none"), answer


# ── Served log line ──────────────────────────────────────────────────────
@then(
    "each recs.served log line carries the tier, ranker and arm of that response's model_version and no raw user id"
)
def era_served_log(world: World) -> None:
    responses = rss_responses(world)
    user_id = rss.buyer_id(world)
    for response in responses:
        _, tier, ranker, arm = segments(response)
        request_id = response["requestId"]
        line = _await_log_line(request_id)
        assert _field(line, "tier") == tier, line
        assert _field(line, "ranker") == ranker, line
        assert _field(line, "arm") == arm, line
        assert user_id not in line, line


def rss_responses(world: World) -> list[dict[str, Any]]:
    return world.state.extra["rss"]["responses"]


def _await_log_line(request_id: str) -> str:
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        for line in rss.ai_log_lines("recs.served"):
            if request_id in line:
                return line
        time.sleep(1)
    raise AssertionError(f"no recs.served log line for request id {request_id}")


def _field(line: str, name: str) -> str:
    match = re.search(rf"\b{name}\W+([^\s,\"'}}]+)", line)
    assert match, f"no {name} in {line}"
    return match.group(1)
