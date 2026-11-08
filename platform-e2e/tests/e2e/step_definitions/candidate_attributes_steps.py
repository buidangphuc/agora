"""Steps for recsys-candidate-attributes (area cattr-e2e). Every scenario is @destructive.

Black box: the real platform-recsys image publishes a live generation (recsys_job_flow, live mode)
over the rgp fixture dataset with an item_attributes@v2 snapshot named by the run key
"@item_attributes" ({listing_id: status or null}); Recommend is then called through the gateway
as the logged-in buyer with placement `cart_cross_sell` and `contextListingIds`.
"""

from __future__ import annotations

from typing import Any

from pytest_bdd import then, when

from src.api.mlflow_registry import MlflowRegistry
from src.api.recsys_job import MLFLOW_IN_STACK
from src.api.services.recommendation_service import CONTEXT_CART
from src.constants import gateway_endpoints as ep
from tests.e2e.flows.recsys_job_flow import run_recsys_job
from tests.e2e.support.world import World

CONTEXT_LISTING = "rgp-item-0-0"
PUBLISHED = "rgp-item-0-1"
BANNED = "rgp-item-0-2"
NO_STATUS = "rgp-item-0-3"
_MLFLOW_ENV = {"MLFLOW_TRACKING_URI": MLFLOW_IN_STACK}
_STATUS = {
    "published": "LISTING_STATUS_PUBLISHED",
    "banned": "LISTING_STATUS_BANNED",
    "none": None,
}


def _ctx(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("cattr", {})


def _publish(world: World, snapshot: dict[str, str | None]) -> None:
    user = world.state.current_user
    assert user is not None and user.user_id, "no logged-in buyer to put in the dataset fixtures"
    MlflowRegistry().reset_registry()
    (run,) = run_recsys_job(
        [],
        [{**_MLFLOW_ENV, "@fixture": "good_a", "@item_attributes": snapshot}],
        live=True,
        reset=True,
        buyer_id=user.user_id,
        expect_summary=True,
    )
    assert run.exit_code == 0, f"the job signalled failure: {run.summary}\n{run.log}"
    assert run.summary["decision"] == "promoted", run.summary
    _ctx(world)["run"] = run


@when(
    "the recsys job publishes a live generation whose attribute snapshot marks one similar listing "
    "published and another banned"
)
def cattr_publish_banned(world: World) -> None:
    _publish(
        world,
        {
            CONTEXT_LISTING: _STATUS["published"],
            PUBLISHED: _STATUS["published"],
            BANNED: _STATUS["banned"],
        },
    )


@when(
    "the recsys job publishes a live generation whose attribute snapshot marks one similar listing "
    "published and leaves another with no status"
)
def cattr_publish_no_status(world: World) -> None:
    _publish(
        world,
        {
            CONTEXT_LISTING: _STATUS["published"],
            PUBLISHED: _STATUS["published"],
            NO_STATUS: _STATUS["none"],
        },
    )


@when("Recommend for cart_cross_sell is called through the gateway with the cart context listing")
def cattr_recommend(world: World) -> None:
    data = world.service_factory.recommendation.post(
        ep.RECOMMENDATION_RECOMMEND,
        {
            "context": CONTEXT_CART,
            "contextListingIds": [CONTEXT_LISTING],
            "limit": 20,
        },
    )
    _ctx(world)["ids"] = [item["listingId"] for item in data.get("items", [])]


@then("the response contains the published listing")
def cattr_has_published(world: World) -> None:
    ids = _ctx(world)["ids"]
    assert PUBLISHED in ids, f"published listing missing from {ids}"


@then("the response does not contain the banned listing")
def cattr_no_banned(world: World) -> None:
    ids = _ctx(world)["ids"]
    assert BANNED not in ids, f"banned listing served in {ids}"


@then("the response does not contain the listing without visible")
def cattr_no_unknown(world: World) -> None:
    ids = _ctx(world)["ids"]
    assert NO_STATUS not in ids, f"listing without visible served in {ids}"
