"""Steps shared by the ai-path-resilience features (change ai-path-resilience, area apr-e2e).

State lives in `world.state.extra` under `apr_*` keys:
    apr_token / apr_other_token   bearer tokens of the two buyers
    apr_reply / apr_tag           the last StreamChat reply and the token that finds it in the fake
"""

from __future__ import annotations

import time

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import apr_support as apr
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


# ── Givens ───────────────────────────────────────────────────────────────
@given("a freshly registered buyer for the LLM checks")
def llm_buyer(world: World) -> None:
    _x(world)["apr_token"] = pe.register(world, "buyer")


@given("a second freshly registered buyer for the LLM checks")
def llm_other_buyer(world: World) -> None:
    _x(world)["apr_other_token"] = pe.register(world, "buyer")


@given("the primary target is serving")
def primary_is_serving(world: World) -> None:
    """Other scenarios share team-ai's breakers; wait out a cooldown another one left open."""
    x = _x(world)
    cooldown = apr.ai_int("LLM_BREAKER_COOLDOWN_SECONDS", 5)
    deadline = time.monotonic() + 2 * cooldown + 10
    last = None
    while time.monotonic() < deadline:
        last = apr.stream(x["apr_token"], "warm up", tagged=apr.tag())
        if apr.answered_by(last) == "primary":
            return
        time.sleep(1)
    raise AssertionError(
        f"the primary target never served a plain message: {last and last.describe()}"
    )


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse('the buyer streams a chat message with the fake provider scripted "{script}"'))
def stream_scripted(world: World, script: str) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    x["apr_reply"] = apr.stream(x["apr_token"], "hello", directive=script, tagged=x["apr_tag"])


@when("the buyer streams a chat message")
def stream_plain(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    x["apr_reply"] = apr.stream(x["apr_token"], "hello", tagged=x["apr_tag"])


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('the buyer receives one complete reply from "{model}" and no error'))
def complete_reply_from(world: World, model: str) -> None:
    reply: apr.Reply = _x(world)["apr_reply"]
    assert reply.ok, reply.describe()
    assert reply.norm == apr.full_reply(model), reply.describe()


@then(parsers.parse('the fake provider received requests for that message only from "{model}"'))
def only_from(world: World, model: str) -> None:
    models = apr.fake_models(_x(world)["apr_tag"])
    assert models and set(models) == {model}, f"fake provider was called for: {models}"


@then(parsers.parse('the fake provider did not call "{model}" for that message'))
def did_not_call(world: World, model: str) -> None:
    models = apr.fake_models(_x(world)["apr_tag"])
    assert model not in models, f"fake provider was called for: {models}"


@then("the fake provider received no request for that message")
def no_request_for_message(world: World) -> None:
    rows = apr.fake_requests(_x(world)["apr_tag"])
    assert not rows, f"fake provider received {[r['model'] for r in rows]}"


@then(parsers.parse('the call fails with "{code}"'))
def call_fails_with(world: World, code: str) -> None:
    reply: apr.Reply = _x(world)["apr_reply"]
    assert reply.code == code, reply.describe()


@then("the error message contains no exception or provider text")
def message_has_no_leak(world: World) -> None:
    reply: apr.Reply = _x(world)["apr_reply"]
    assert reply.message, f"the error carries no message at all: {reply.describe()}"
    lowered = reply.message.lower()
    leaked = [m for m in apr.LEAK_MARKERS if m in lowered]
    assert not leaked and len(reply.message) < 80, f"leaked {leaked}: {reply.message!r}"


@then("no reply chunk was streamed")
def nothing_streamed(world: World) -> None:
    reply: apr.Reply = _x(world)["apr_reply"]
    assert not reply.deltas, f"chunks streamed: {reply.deltas}"
