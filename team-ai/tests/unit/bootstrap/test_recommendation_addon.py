from contextlib import AsyncExitStack

from fakeredis import aioredis

from app.bootstrap.addons.recommendation import RecommendationAddon
from app.bootstrap.resources import ApplicationResources
from app.modules.business.recommendation.adapters.flipt import FliptExperimentAssigner
from tests.factories import build_test_settings


async def test_clients_carry_a_flipt_assigner_when_the_url_is_set_a_visitor_always_lands_in_the_same_arm() -> (
    None
):
    resources = ApplicationResources(redis=aioredis.FakeRedis())
    settings = build_test_settings(RECS_FLIPT_URL="http://flipt:8080")

    async with AsyncExitStack() as stack:
        await RecommendationAddon().open(resources, settings, stack)

        clients = resources.recommendation_clients
        assert clients is not None
        assert isinstance(clients.assigner, FliptExperimentAssigner)


async def test_clients_carry_no_assigner_when_the_url_is_empty_a_disabled_flag_means_no_arm() -> (
    None
):
    resources = ApplicationResources(redis=aioredis.FakeRedis())
    settings = build_test_settings(RECS_FLIPT_URL="")

    async with AsyncExitStack() as stack:
        await RecommendationAddon().open(resources, settings, stack)

        clients = resources.recommendation_clients
        assert clients is not None
        assert clients.assigner is None


async def test_flipt_http_client_is_closed_with_the_exit_stack_flipt_unreachable_serves_the_default_ranker_as_unassigned() -> (
    None
):
    resources = ApplicationResources(redis=aioredis.FakeRedis())
    settings = build_test_settings(RECS_FLIPT_URL="http://flipt:8080")

    async with AsyncExitStack() as stack:
        await RecommendationAddon().open(resources, settings, stack)
        assigner = resources.recommendation_clients.assigner  # type: ignore[union-attr]
        http = assigner._client  # type: ignore[union-attr]
        assert not http.is_closed

    assert http.is_closed
