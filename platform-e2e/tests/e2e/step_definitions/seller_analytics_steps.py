"""Step definitions for seller analytics dashboard."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages.seller_analytics_page import SellerAnalyticsPage
from tests.e2e.support.world import World


@when("the seller opens the analytics page")
def seller_opens_analytics(world: World) -> None:
    world.navigate_to(PageName.SELLER_ANALYTICS)


@then("the seller analytics dashboard is displayed")
def seller_analytics_displayed(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/seller/analytics.*"), timeout=timeouts.DEFAULT)


@then("the revenue metrics summary is visible")
def revenue_metrics_visible(world: World) -> None:
    analytics_page: SellerAnalyticsPage = world.get_page(PageName.SELLER_ANALYTICS)  # type: ignore[assignment]
    expect(analytics_page.revenue_metric_card).to_be_visible(timeout=timeouts.DEFAULT)
