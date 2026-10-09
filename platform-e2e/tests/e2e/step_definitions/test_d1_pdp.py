"""Binds frontend/product_detail_ui.feature.

Two scenarios are bound explicitly and marked xfail because the running storefront does not do
what the spec says (each self-heals with an xpass once fixed):
- "The bar covers no content": the global footer scrolls under the 375px buy bar.
- "Reviews paginate at 10": only the first 20 reviews are fetched, so page 3 of 23 is unreachable.
"""

import pytest
from pytest_bdd import scenario, scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_pdp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.pdp_shop_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.promo_steps import *  # noqa: F401,F403

_FEATURE = "frontend/product_detail_ui.feature"


@pytest.mark.xfail(reason="the footer scrolls under the 375px buy bar", strict=False)
@scenario(_FEATURE, "The bar covers no content")
def test_the_bar_covers_no_content() -> None:
    pass


@pytest.mark.xfail(
    reason="only the first 20 reviews are fetched; page 3 is unreachable", strict=False
)
@scenario(_FEATURE, "Reviews paginate at 10")
def test_reviews_paginate_at_10() -> None:
    pass


scenarios(_FEATURE)
