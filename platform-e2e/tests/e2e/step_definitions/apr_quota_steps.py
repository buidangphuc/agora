"""Usage logging, per-principal quota and the per-principal rate limit (through the gateway).

The quota and the limit are read from the running team-ai container
(`QUOTA_CHAT_REPLIES_PER_WINDOW`, `RATE_LIMIT_PRINCIPAL_PER_MINUTE`), never copied. Each
scenario registers its own buyers, so parallel scenarios do not share a quota or a bucket.
The rate-limit scenario scripts every target to answer 400: a request-caused failure does
not trip a breaker for the other scenarios and, being a failure before the first chunk,
refunds its quota, so the only thing that can stop the buyer is the limiter.
"""

from __future__ import annotations

import re
import time

from pytest_bdd import given, then, when

from tests.e2e.support import apr_support as apr
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


def _quota() -> int:
    quota = apr.ai_int("QUOTA_CHAT_REPLIES_PER_WINDOW", 15)
    limit = apr.ai_int("RATE_LIMIT_PRINCIPAL_PER_MINUTE", 20)
    assert quota < limit, (
        f"quota {quota} must be below the {limit}/min principal limit, or it cannot be "
        "exhausted inside one window"
    )
    return quota


def _spend(token: str, replies: int) -> None:
    for index in range(replies):
        reply = apr.stream(token, f"quota reply {index}", tagged=apr.tag())
        assert (
            reply.ok
        ), f"reply {index + 1}/{replies} failed before the quota was reached: {reply.describe()}"


# ── usage log ────────────────────────────────────────────────────────────
@when("the buyer's chat reply completes and the fake provider reports 11 input and 7 output tokens")
def completed_reply(world: World) -> None:
    x = _x(world)
    x["apr_since"] = apr.log_start()
    x["apr_request_id"] = pe.rid("e2e-usage")
    x["apr_reply"] = apr.stream(
        x["apr_token"], "hello", request_id=x["apr_request_id"], tagged=apr.tag()
    )
    assert x["apr_reply"].ok, x["apr_reply"].describe()


@then(
    "team-ai's log has one usage line for that request id with 11 input tokens, 7 output tokens and the target"
)
def usage_line(world: World) -> None:
    x = _x(world)
    rid = x["apr_request_id"]
    deadline = time.monotonic() + 15
    lines: list[str] = []
    while time.monotonic() < deadline:
        lines = [
            line
            for line in apr.ai_logs(x["apr_since"]).splitlines()
            if rid in line and "usage" in line.lower()
        ]
        if lines:
            break
        time.sleep(0.5)
    assert len(lines) == 1, f"{len(lines)} usage lines for {rid}: {lines}"
    line = lines[0]
    assert re.search(r"(?<!\d)11(?!\d)", line), f"no input token count of 11: {line}"
    assert re.search(r"(?<!\d)7(?!\d)", line), f"no output token count of 7: {line}"
    assert re.search(r"primary", line), f"the usage line does not name the target: {line}"


# ── quota ────────────────────────────────────────────────────────────────
@given("a buyer who has used up the per-principal chat quota")
def quota_used_up(world: World) -> None:
    x = _x(world)
    x["apr_token"] = pe.register(world, "buyer")
    _spend(x["apr_token"], _quota())


@given("a buyer with quota for exactly one more reply")
def quota_for_one_more(world: World) -> None:
    x = _x(world)
    x["apr_token"] = pe.register(world, "buyer")
    _spend(x["apr_token"], _quota() - 1)


@when("the buyer streams another chat message")
def stream_another(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    x["apr_reply"] = apr.stream(x["apr_token"], "one too many", tagged=x["apr_tag"])


@when(
    "the buyer streams a message for which every target answers 500, then streams a message with healthy targets"
)
def fail_then_succeed(world: World) -> None:
    x = _x(world)
    x["apr_failed"] = apr.stream(
        x["apr_token"], "all down", directive=apr.ALL_500, tagged=apr.tag()
    )
    x["apr_reply"] = apr.stream_when_available(x["apr_token"], "all healthy", tagged=apr.tag())


@then('the first call fails with "unavailable" and the second reply completes')
def first_failed_second_completed(world: World) -> None:
    x = _x(world)
    assert x["apr_failed"].code == "unavailable", x["apr_failed"].describe()
    assert x["apr_reply"].ok, (
        "the failed call consumed the buyer's last reply of quota: " + x["apr_reply"].describe()
    )


# ── rate limit ───────────────────────────────────────────────────────────
@when("the buyer streams more chat messages within a minute than the per-principal limit")
def stream_past_limit(world: World) -> None:
    x = _x(world)
    limit = apr.ai_int("RATE_LIMIT_PRINCIPAL_PER_MINUTE", 20)
    x["apr_limit"] = limit
    # team-ai's limiter is a sliding-window counter over fixed windows: a burst that straddles
    # a window boundary is counted with a weighted slice of the previous window, which can
    # move the cut-off by one. Start the burst well inside a window so "the first `limit`
    # pass, the rest fail" is exact.
    window = apr.ai_int("RATE_LIMIT_WINDOW_SECONDS", 60)
    if time.time() % window > window - 15:
        time.sleep(window - time.time() % window + 1)
    x["apr_burst"] = [
        apr.stream(x["apr_token"], f"limit probe {i}", directive=apr.ALL_400, tagged=apr.tag())
        for i in range(limit + 5)
    ]


@then('the calls past the limit fail with "resource_exhausted" without reaching the provider')
def calls_past_limit_refused(world: World) -> None:
    x = _x(world)
    burst: list[apr.Reply] = x["apr_burst"]
    limit: int = x["apr_limit"]
    inside, past = burst[:limit], burst[limit:]
    early = [r.describe() for r in inside if r.code == "resource_exhausted"]
    assert not early, f"refused before the limit of {limit}: {early[:2]}"
    wrong = [r.describe() for r in past if r.code != "resource_exhausted"]
    assert not wrong, f"calls past the limit of {limit} were not refused: {wrong[:2]}"
    reached = [r.sent for r in past if apr.fake_requests(r.extra["tag"])]
    assert not reached, f"refused calls still reached the provider: {reached[:2]}"


@then("a different buyer's chat message right after succeeds")
def other_buyer_succeeds(world: World) -> None:
    reply = apr.stream(_x(world)["apr_other_token"], "I am a different buyer", tagged=apr.tag())
    assert reply.ok and reply.deltas, reply.describe()
