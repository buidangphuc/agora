"""Per-target breaker steps (destructive lane).

The breakers live in the team-ai process and are shared by every scenario, so these run
serially. The primary is driven with the fake's global mode (`POST /_mode`), not a
per-request directive, and the modes are cleared again in teardown. Timings come from the
running container (`LLM_BREAKER_THRESHOLD`, `LLM_BREAKER_COOLDOWN_SECONDS`).
"""

from __future__ import annotations

import time

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import apr_support as apr
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


def _threshold() -> int:
    return apr.ai_int("LLM_BREAKER_THRESHOLD", 2)


def _cooldown() -> float:
    return float(apr.ai_setting("LLM_BREAKER_COOLDOWN_SECONDS", "5"))


def _fail_primary(world: World, status: str) -> None:
    x = _x(world)
    apr.fake_set_mode(primary=status)
    for _ in range(_threshold()):
        apr.stream(x["apr_token"], "break the primary", tagged=apr.tag())
    apr.fake_set_mode(primary="ok")
    x["apr_failed_at"] = time.monotonic()


@given("every fake target is healthy and the primary's breaker is closed")
def breaker_closed(world: World) -> None:
    x = _x(world)
    apr.fake_set_mode(primary="ok", fb1="ok", fb2="ok")
    world.add_cleanup(lambda: apr.fake_set_mode(primary="ok", fb1="ok", fb2="ok"))
    deadline = time.monotonic() + 2 * _cooldown() + 10
    last = None
    while time.monotonic() < deadline:
        last = apr.stream(x["apr_token"], "close the breaker", tagged=apr.tag())
        if apr.answered_by(last) == "primary":
            return
        time.sleep(0.5)
    raise AssertionError(f"the primary's breaker did not close: {last and last.describe()}")


@given("the primary's breaker has been opened by consecutive 500 answers")
def breaker_opened(world: World) -> None:
    _fail_primary(world, "500")


@when(
    parsers.parse(
        "the primary answers {status} for LLM_BREAKER_THRESHOLD consecutive chat requests "
        "and then becomes healthy"
    )
)
def fail_then_heal(world: World, status: str) -> None:
    _fail_primary(world, status)


@when("the cooldown elapses and the primary is healthy again")
def cooldown_elapses(world: World) -> None:
    x = _x(world)
    wait = x["apr_failed_at"] + _cooldown() + 1.0 - time.monotonic()
    if wait > 0:
        time.sleep(wait)


@then(
    "the next chat request made within the cooldown is served by the first fallback without the primary being called"
)
def served_by_fallback(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    reply = apr.stream(x["apr_token"], "during the cooldown", tagged=x["apr_tag"])
    waited = time.monotonic() - x["apr_failed_at"]
    assert (
        waited < _cooldown()
    ), f"the cooldown ({_cooldown()}s) had already elapsed ({waited:.1f}s)"
    assert apr.answered_by(reply) == "fb1", reply.describe()
    assert "primary" not in apr.fake_models(x["apr_tag"]), "the open breaker's target was called"


@then("the next chat request is served by the primary")
def served_by_primary(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    reply = apr.stream(x["apr_token"], "after the 400s", tagged=x["apr_tag"])
    assert apr.answered_by(reply) == "primary", reply.describe()


@then("the next chat request is served by the primary, and so is the one after it")
def served_by_primary_twice(world: World) -> None:
    token = _x(world)["apr_token"]
    for label in ("probe", "follow-up"):
        reply = apr.stream(token, f"the {label}", tagged=apr.tag())
        assert apr.answered_by(reply) == "primary", f"{label}: {reply.describe()}"
