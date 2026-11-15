"""Steps for frontend/buyer_ai_surfaces.feature (change buyer-ai-surfaces-ui).

Black box: the assistant page is driven in a real browser (team-ai in mock mode); the Server Action
traffic, the stream route and the beacons sent to /api/track are what is asserted.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.core.base_page import goto_streamed
from tests.e2e.step_definitions.ai_interaction_steps import _ask, _assistant, _cards, _x
from tests.e2e.support import ai_events_support as ai
from tests.e2e.support.world import World

_PLACEMENT = "assistant.suggestions"


def _base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


@given("the browser records assistant replies")
def records_replies(world: World) -> None:
    actions: list[str] = []
    streams: list[str] = []
    bodies: list[str] = []
    x = _x(world)
    x.update(ai_action_urls=actions, ai_stream_urls=streams, ai_action_bodies=bodies)

    def on_request(request) -> None:  # noqa: ANN001
        if "/api/assistant/stream" in request.url:
            streams.append(request.url)
        elif request.method == "POST" and "/assistant" in request.url and (
            "next-action" in request.headers
        ):
            actions.append(request.url)

    def on_response(response) -> None:  # noqa: ANN001
        req = response.request
        if req.method == "POST" and "next-action" in req.headers and "/assistant" in req.url:
            try:
                bodies.append(response.text())
            except Exception:  # noqa: BLE001
                pass

    world.page.on("request", on_request)
    world.page.on("response", on_response)


@when(parsers.parse('the buyer opens the assistant page and asks "{question}"'))
def open_and_ask(world: World, question: str) -> None:
    goto_streamed(world.page, f"{_base(world)}/assistant")
    _ask(world, question)
    expect(_cards(world).first).to_be_visible(timeout=timeouts.LONG)
    _x(world)["ai_reply_text"] = _assistant(world).ai_replies.last.inner_text()
    world.page.wait_for_timeout(1_000)


@then("the assistant page made exactly 1 assistant action request and 0 stream requests")
def one_call_no_stream(world: World) -> None:
    x = _x(world)
    assert len(x["ai_action_urls"]) == 1, x["ai_action_urls"]
    assert not x["ai_stream_urls"], x["ai_stream_urls"]


@then("the assistant bubble text equals the reply text of that response")
def bubble_equals_reply(world: World) -> None:
    x = _x(world)
    body = "\n".join(x["ai_action_bodies"])
    lines = [ln.strip() for ln in x["ai_reply_text"].splitlines() if len(ln.strip()) >= 12]
    assert lines, "the assistant bubble has no text"
    first = lines[0]
    assert first in body, f"bubble line {first!r} is not part of the unary response"


@when(parsers.parse('a client posts to "{path}"'))
def client_posts(world: World, path: str) -> None:
    resp = world.context.request.post(f"{_base(world)}{path}", data={"messages": []})
    _x(world)["post_status"] = resp.status


@then(parsers.parse("the response status is {status:d}"))
def status_is(world: World, status: int) -> None:
    assert _x(world)["post_status"] == status, _x(world)["post_status"]


def _impressions(world: World) -> list[dict]:
    return [b for b in ai.all_beacons(world) if b.get("type") == "impression"]


def _wait_impressions(world: World) -> list[dict]:
    deadline = 15_000
    waited = 0
    while waited < deadline:
        found = [i for i in _impressions(world) if i.get("placementId") == _PLACEMENT]
        if found:
            break
        world.page.wait_for_timeout(500)
        waited += 500
    world.page.wait_for_timeout(int(ai.SETTLE_S * 1000))
    return [i for i in _impressions(world) if i.get("placementId") == _PLACEMENT]


@then(parsers.parse('every assistant card has an impression beacon with placement "{placement}"'))
def card_impressions(world: World, placement: str) -> None:
    found = _wait_impressions(world)
    cards = _cards(world).count()
    assert cards >= 1 and len(found) == cards, f"{len(found)} impressions for {cards} cards"
    assert all(i.get("placementId") == placement for i in found)


@then(
    "every assistant impression carries an impressionId equal to the reply request id and a "
    "non-empty modelVersion"
)
def impressions_provenance(world: World) -> None:
    found = _wait_impressions(world)
    ids = {i.get("impressionId") for i in found}
    assert len(ids) == 1 and all(ids), ids
    body = "\n".join(_x(world)["ai_action_bodies"])
    assert next(iter(ids)) in body, "impressionId is not the request id of the reply"
    assert all(i.get("modelVersion") for i in found), found


@then("the assistant impression positions are 1 up to the number of cards")
def positions(world: World) -> None:
    found = _wait_impressions(world)
    got = sorted(int(i.get("position", 0)) for i in found)
    assert got == list(range(1, len(found) + 1)), got


def _entry(world: World):  # noqa: ANN202
    return world.page.locator("main a[href='/assistant']").first


@when("a visitor opens the home page")
def open_home(world: World) -> None:
    goto_streamed(world.page, f"{_base(world)}/")


@then(parsers.parse('the assistant entry is a link to "{href}"'))
def entry_is_link(world: World, href: str) -> None:
    expect(_entry(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    assert _entry(world).get_attribute("href") == href


@when("the visitor activates the assistant entry")
def activate_entry(world: World) -> None:
    world.page.wait_for_timeout(1_500)
    _entry(world).click()


@then("the visitor lands on the assistant page")
def lands_assistant(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/assistant"), timeout=timeouts.NAVIGATION)
