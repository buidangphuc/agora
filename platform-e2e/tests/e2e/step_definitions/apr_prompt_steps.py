"""System prompt, session history, redaction and assistant logs (spec requirements 6 and 7).

What the model actually received is read from the fake provider's record of the request
(`GET /_requests?contains=<tag>`), so the assertions are on the real wire payload.
"""

from __future__ import annotations

import re
import time

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import apr_support as apr
from tests.e2e.support.world import World

# Any of the usual shapes of a redaction marker: [REDACTED_PHONE], <PHONE>, ***, "redacted"...
MARKER = re.compile(r"(?i)redact|masked|\*{2,}|\[[A-Z_]{3,}\]|<[A-Z_]{3,}>|█")


def _x(world: World) -> dict:
    return world.state.extra


def _first_request(tag: str) -> dict:
    rows = apr.fake_requests(tag)
    assert rows, f"the fake provider received no request containing {tag!r}"
    return rows[0]


def _messages(row: dict) -> list[dict]:
    return row["body"]["messages"]


def _request_for(tag: str, marker: str) -> dict:
    """The fake's request whose last user message contains `marker`."""
    for row in apr.fake_requests(tag):
        users = [m for m in _messages(row) if m["role"] == "user"]
        if users and marker in users[-1]["content"]:
            return row
    raise AssertionError(f"no provider request ends with a user message containing {marker!r}")


# ── system prompt ────────────────────────────────────────────────────────
@then(
    "the request the fake provider received starts with a system message equal to the configured system prompt"
)
def starts_with_system(world: World) -> None:
    configured = apr.ai_setting("CHAT_SYSTEM_PROMPT", apr.DEFAULT_SYSTEM_PROMPT)
    row = _first_request(_x(world)["apr_tag"])
    first = _messages(row)[0]
    assert first == {"role": "system", "content": configured}, f"first message: {first!r}"


# ── history ──────────────────────────────────────────────────────────────
@given("a chat session id for the history checks")
def history_session(world: World) -> None:
    _x(world)["apr_session"] = f"e2e-apr-session-{apr.tag()}"


@when('the buyer streams "first question" and then "second question" with the same session_id')
def two_questions(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    kw = {"session_id": x["apr_session"], "tagged": x["apr_tag"]}
    x["apr_first"] = apr.stream(x["apr_token"], "first question", **kw)
    assert x["apr_first"].ok, x["apr_first"].describe()
    x["apr_reply"] = apr.stream(x["apr_token"], "second question", **kw)
    assert x["apr_reply"].ok, x["apr_reply"].describe()


@then(
    "the second request the fake provider received contains, after the system message, the first user turn, the first assistant reply and the second user turn, in that order"
)
def history_in_order(world: World) -> None:
    x = _x(world)
    row = _request_for(x["apr_tag"], "second question")
    after_system = _messages(row)[1:]
    assert [m["role"] for m in after_system] == ["user", "assistant", "user"], after_system
    assert after_system[0]["content"] == x["apr_first"].sent
    assert " ".join(after_system[1]["content"].split()) == x["apr_first"].norm
    assert after_system[2]["content"] == x["apr_reply"].sent


@when("the buyer streams more messages in one session than CHAT_HISTORY_MAX_TURNS")
def many_messages(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    x["apr_turns"] = apr.ai_int("CHAT_HISTORY_MAX_TURNS", 4)
    total = x["apr_turns"] + 3
    x["apr_total"] = total
    for index in range(1, total + 1):
        reply = apr.stream_when_available(
            x["apr_token"],
            f"message {index}",
            session_id=x["apr_session"],
            tagged=x["apr_tag"],
        )
        assert reply.ok, f"message {index}: {reply.describe()}"
        x["apr_reply"] = reply


@then(
    "the last request the fake provider received contains only the most recent CHAT_HISTORY_MAX_TURNS earlier turns"
)
def history_is_bounded(world: World) -> None:
    x = _x(world)
    row = _request_for(x["apr_tag"], f"message {x['apr_total']} ")
    body = _messages(row)
    history = body[1:-1]
    limit = x["apr_turns"]
    texts = [m["content"] for m in history]
    # A turn is a user message or an exchange (user + assistant): allow either reading, but
    # the unbounded history would hold 2 * (total - 1) messages.
    assert (
        len(history) <= 2 * limit
    ), f"{len(history)} history messages for CHAT_HISTORY_MAX_TURNS={limit}"
    assert len(history) < 2 * (x["apr_total"] - 1), "the history was not trimmed at all"
    assert not any("message 1 " in t for t in texts), "the oldest turn was kept"
    assert any(
        f"message {x['apr_total'] - 1} " in t for t in texts
    ), "the newest earlier turn is missing"
    assert history[0]["role"] in ("user", "assistant") and history[-1]["role"] == "assistant"
    assert body[-1]["role"] == "user"


@when(
    "the buyer streams a message for which every target answers 500, then streams a follow-up with the same session_id"
)
def failed_then_follow_up(world: World) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    kw = {"session_id": x["apr_session"], "tagged": x["apr_tag"]}
    x["apr_failed"] = apr.stream(x["apr_token"], "doomed question", directive=apr.ALL_500, **kw)
    x["apr_reply"] = apr.stream_when_available(x["apr_token"], "follow-up question", **kw)


@then("the follow-up request the fake provider received contains no turn from the failed message")
def no_failed_turn(world: World) -> None:
    x = _x(world)
    assert x["apr_failed"].code == "unavailable", x["apr_failed"].describe()
    assert x["apr_reply"].ok, x["apr_reply"].describe()
    row = _request_for(x["apr_tag"], "follow-up question")
    messages = _messages(row)
    assert "doomed question" not in str(messages), messages
    assert [m["role"] for m in messages] == ["system", "user"], messages


# ── redaction ────────────────────────────────────────────────────────────
@when(parsers.parse('the buyer streams "{text}"'))
def stream_text(world: World, text: str) -> None:
    x = _x(world)
    x["apr_tag"] = apr.tag()
    x["apr_reply"] = apr.stream(x["apr_token"], text, tagged=x["apr_tag"])
    assert x["apr_reply"].ok, x["apr_reply"].describe()


def _user_text(world: World) -> str:
    row = _first_request(_x(world)["apr_tag"])
    users = [m["content"] for m in _messages(row) if m["role"] == "user"]
    assert users, _messages(row)
    return users[-1]


@then(
    'the request the fake provider received contains redaction markers and neither "0912 345 678" nor "079123456789"'
)
def phone_and_id_masked(world: World) -> None:
    row = _first_request(_x(world)["apr_tag"])
    wire = str(_messages(row))
    user = _user_text(world)
    assert "0912 345 678" not in wire and "079123456789" not in wire, user
    assert MARKER.search(user), f"no redaction marker in the model input: {user!r}"


@then('the request the fake provider received contains "850000000" and "123456789"')
def numbers_kept(world: World) -> None:
    user = _user_text(world)
    assert "850000000" in user and "123456789" in user, user
    assert not MARKER.search(
        user.replace(_x(world)["apr_tag"], "")
    ), f"digits were masked: {user!r}"


# ── assistant logs ───────────────────────────────────────────────────────
@when(
    'the buyer asks the Shopping Assistant through the gateway a question containing "a.b@example.com"'
)
def ask_assistant(world: World) -> None:
    x = _x(world)
    x["apr_since"] = apr.log_start()
    resp = apr.assistant_call(x["apr_token"], "Please email me at a.b@example.com about a laptop")
    assert resp.status_code == 200, (resp.status_code, resp.text[:300])
    time.sleep(2)  # let team-ai flush its log line


@then('no team-ai log line contains "a.b@example.com"')
def log_has_no_email(world: World) -> None:
    logs = apr.ai_logs(_x(world)["apr_since"])
    offending = [line for line in logs.splitlines() if "a.b@example.com" in line]
    assert logs.strip(), "team-ai wrote no log at all since the call, so the check proves nothing"
    assert not offending, offending[:3]
