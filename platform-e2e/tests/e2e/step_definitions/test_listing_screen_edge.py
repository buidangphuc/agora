"""Binds ai/listing_screen_edge.feature (change ai-listing-screen)."""

from pytest_bdd import scenarios, then, when

from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

SCREEN = "/platform.ai.v1.AIService/ScreenListing"
# An RPC already known to be internal-only: the control proving that the gateway is reachable
# and answers 501 `unimplemented` for service-only AIService methods, so a non-501 on
# ScreenListing is about the missing registration and not an unreachable gateway.
CONTROL = "/platform.ai.v1.AIService/ClassifyTags"


@when("an anonymous caller and a signed-in buyer call ScreenListing through the gateway")
def screen_at_edge(world: World) -> None:
    c = s.ctx(world)
    buyer = s.buyer_of(world)
    body = {"title": "x"}
    c.extra["screen_ctrl"] = [
        s.post(CONTROL, {"title": "x"}),
        s.post(CONTROL, {"title": "x"}, buyer.token),
    ]
    c.resps = [s.post(SCREEN, body), s.post(SCREEN, body, buyer.token)]


@then("both calls fail with unimplemented and no verdict is returned")
def screen_unimplemented(world: World) -> None:
    c = s.ctx(world)
    for resp in c.extra["screen_ctrl"]:
        assert resp.status_code == 501, f"control: want 501, got {resp.status_code}: {resp.text}"
    for resp in c.resps:
        assert resp.status_code == 501, f"want 501, got {resp.status_code}: {resp.text}"
        assert resp.json().get("code") == "unimplemented", resp.text
        assert "verdict" not in resp.json(), resp.text


scenarios("ai/listing_screen_edge.feature")
