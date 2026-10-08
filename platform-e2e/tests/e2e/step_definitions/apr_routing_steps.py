"""Routing, fallback and timeout steps (spec requirements: routing, bounded calls)."""

from __future__ import annotations

from pytest_bdd import then

from tests.e2e.support import apr_support as apr
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


@then(
    'the buyer receives those two chunks from "primary", then an error status, '
    "and no chunk from any other target"
)
def two_chunks_then_error(world: World) -> None:
    x = _x(world)
    reply: apr.Reply = x["apr_reply"]
    assert reply.code != "ok", f"the stream ended without an error: {reply.describe()}"
    assert reply.norm == "[primary] xin", f"expected the two primary chunks: {reply.describe()}"
    assert "[fb" not in reply.text, reply.describe()
    models = apr.fake_models(x["apr_tag"])
    assert models == ["primary"], f"a second target was called mid-reply: {models}"


@then('the reply comes from "fb1" within the first-token timeout plus 3 seconds')
def fallback_within_timeout(world: World) -> None:
    reply: apr.Reply = _x(world)["apr_reply"]
    limit = float(apr.ai_setting("LLM_FIRST_TOKEN_TIMEOUT_SECONDS", "2")) + 3
    assert reply.ok and apr.answered_by(reply) == "fb1", reply.describe()
    assert reply.elapsed <= limit, f"took {reply.elapsed:.1f}s, limit {limit:.1f}s"


@then("the fake provider received at most LLM_MAX_ATTEMPTS requests for that message")
def attempts_bounded(world: World) -> None:
    x = _x(world)
    reply: apr.Reply = x["apr_reply"]
    limit = apr.ai_int("LLM_MAX_ATTEMPTS", 3)
    seen = apr.fake_models(x["apr_tag"])
    assert not reply.ok, f"every target answered 500 but the call succeeded: {reply.describe()}"
    assert len(seen) <= limit, f"{len(seen)} provider requests ({seen}) > LLM_MAX_ATTEMPTS={limit}"
