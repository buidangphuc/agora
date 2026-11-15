"""Steps for seller/seller_terms_consent.feature (change frontend-legal-pages).

Reuses the seller onboarding browser steps and the user-enforcement account steps through the shared
`ueui` state bucket; only the link check and the ledger read-back are new.
"""

from __future__ import annotations

import time
from urllib.parse import urlsplit

import httpx
from playwright.sync_api import expect
from pytest_bdd import parsers, then

from src.constants import timeouts
from tests.e2e.step_definitions.legal_consent_steps import (  # noqa: F401
    _all_records,
)
from tests.e2e.support import seller_onboarding_support as so
from tests.e2e.support.world import World

SELLER_TERMS = "CONSENT_PURPOSE_SELLER_TERMS"


def _buyer(world: World) -> dict:
    return world.state.extra["ueui"]["buyer"]


def _terms_link(world: World):
    return world.page.locator('a[href$="/dieu-khoan-nguoi-ban"]')


@then(parsers.parse('the seller terms link targets the buyer app "{path}" in a new tab'))
def link_target(world: World, path: str) -> None:
    link = _terms_link(world)
    expect(link.first).to_be_visible(timeout=timeouts.NAVIGATION)
    href = link.first.get_attribute("href") or ""
    assert href.rstrip("/") == world.settings.base_url.rstrip("/") + path, href
    assert link.first.get_attribute("target") == "_blank", href
    world.state.extra["seller_terms_href"] = href


@then("the seller terms link target answers 200")
def link_200(world: World) -> None:
    resp = httpx.get(world.state.extra["seller_terms_href"], follow_redirects=False, timeout=30)
    assert resp.status_code == 200, (urlsplit(resp.url.__str__()).path, resp.status_code)


@then(
    parsers.parse(
        'the ledger holds a SELLER_TERMS row with the seller terms version and source "{source}"'
    )
)
def seller_row(world: World, source: str) -> None:
    deadline = time.monotonic() + 15
    rows: list[dict] = []
    while time.monotonic() < deadline:
        rows = [r for r in _all_records(_buyer(world)["token"]) if r["purpose"] == SELLER_TERMS]
        if rows:
            break
        time.sleep(0.5)
    assert rows, "no SELLER_TERMS row after BecomeSeller succeeded"
    assert rows[0]["version"] == so.current_terms() and rows[0]["source"] == source, rows[0]
    assert rows[0].get("granted") is True, rows[0]


@then("the ledger holds no SELLER_TERMS row for the buyer")
def no_seller_row(world: World) -> None:
    rows = [r for r in _all_records(_buyer(world)["token"]) if r["purpose"] == SELLER_TERMS]
    assert rows == [], rows
