"""Steps for search/search_dynamic_facets.feature and frontend/search_dynamic_facets.feature
(change add-tag-classifier-filter-enrichment, search read-model side).

Black box through the gateway (Connect JSON) and the storefront. Listings are created by a real
seller with variants; team-search's indexer classifies them through team-ai (tag classifier) and
the answers are observed only through `SearchListings` filters/facets and the /search page.
Classification is part of the async indexing path, so every observation is a bounded poll and an
absence is asserted only after a positive control proved the listing is indexed with its tags.
"""

from __future__ import annotations

from typing import Any

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import PageName, timeouts
from src.pages import SearchPage
from tests.e2e.support import srm_support as s
from tests.e2e.support.srm_support import Listed
from tests.e2e.support.world import World

TITLE = "Tai nghe Bluetooth 5.3 chống ồn ANC sạc nhanh 65W GaN"
TITAN_256 = "Titan Tự Nhiên / 256GB"
NAVY_512 = "Xanh Navy / 512GB"
NAVY_256 = "Xanh Navy / 256GB"
TITAN_512 = "Titan Tự Nhiên / 512GB"


def _variant(name: str, stock: int) -> dict[str, Any]:
    code = "".join(ch for ch in name.upper() if ch.isalnum())[:20]
    return {"name": name, "sku": f"MLS-{code}", "price": 100_000, "stock": stock}


def _create(world: World, label: str, variants: list[tuple[str, int]]) -> Listed:
    """Publish a listing whose title carries the scenario keyword and the tag-rich text."""
    c = s.ctx(world)
    seller = s.seller_of(world)
    title = f"{c.keyword} {TITLE}"
    body: dict[str, Any] = {
        "listing": {
            "title": title,
            "categoryId": "cat-laptop",
            "price": 100_000,
            "stock": 5,
            "status": "LISTING_STATUS_PUBLISHED",
            "currency": "VND",
            "description": "mls e2e",
        }
    }
    if variants:
        body["listing"]["variants"] = [_variant(n, st) for n, st in variants]
    resp = s.post(s.LISTING + "CreateListing", body, seller.token)
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    listed = Listed(resp.json()["listing"]["id"], title)
    c.listings[label] = listed
    return listed


def _ids(world: World, filters: dict[str, str]) -> set[str]:
    resp = s.search(s.ctx(world).keyword, filters=filters, page_size=50)
    return set(s.hit_ids(resp))


def _wait_tagged(world: World, label: str) -> None:
    """Positive control: the listing is searchable AND carries SPU tags (classified)."""
    listed = s.listing(world, label)
    s.eventually(
        lambda: listed.id in _ids(world, {"tag.connectivity": "bluetooth-5-3"}),
        f"listing {label} {listed.id} to be indexed with its classified tags",
        90.0,
    )


def _group(resp_json: dict[str, Any], kind: str, group: str) -> dict[str, int]:
    for g in (resp_json.get("facets") or {}).get(kind, []) or []:
        if g.get("group") == group:
            return {b["key"]: int(b["count"]) for b in g.get("buckets", [])}
    return {}


# ── givens ───────────────────────────────────────────────────────────────
@given("a seller publishes a listing titled with Bluetooth 5.3, ANC and 65W GaN fast charging")
def spu_listing(world: World) -> None:
    _create(world, "A", [])
    s.wait_indexed(world, "A")


@given(
    "a published listing with in-stock variants Titan Tự Nhiên 256GB and Xanh Navy 512GB, and a "
    "second one with Xanh Navy 256GB and Titan Tự Nhiên 512GB"
)
def cross_variant_listings(world: World) -> None:
    _create(world, "A", [(TITAN_256, 5), (NAVY_512, 5)])
    _create(world, "B", [(NAVY_256, 5), (TITAN_512, 5)])
    _wait_tagged(world, "A")
    _wait_tagged(world, "B")


@given(
    "a published listing whose Xanh Navy 512GB variant has stock 0, and another with that "
    "variant in stock"
)
def sold_out_listings(world: World) -> None:
    _create(world, "A", [(TITAN_256, 5), (NAVY_512, 0)])
    _create(world, "B", [(NAVY_512, 5)])
    _wait_tagged(world, "A")
    _wait_tagged(world, "B")


@given("published listings with classified SPU tags and variants")
def tagged_listings(world: World) -> None:
    cross_variant_listings(world)


@given("published listings with different variant colors indexed for a keyword")
def colour_listings(world: World) -> None:
    _create(world, "A", [(NAVY_512, 5)])
    _create(world, "B", [(TITAN_256, 5)])
    _wait_tagged(world, "A")
    _wait_tagged(world, "B")


# ── whens ────────────────────────────────────────────────────────────────
@when("the listing event has been indexed")
def event_indexed(world: World) -> None:
    _wait_tagged(world, "A")


@when("a buyer searches with sku.color xanh-navy and sku.capacity 512gb")
def navy_512(world: World) -> None:
    s.ctx(world).extra["filters"] = {"sku.color": "xanh-navy", "sku.capacity": "512gb"}


@when(
    "a buyer calls SearchListings with the filter sku.Colour Name x, then with tag.color "
    "den:other"
)
def malformed(world: World) -> None:
    c = s.ctx(world)
    buyer = s.buyer_of(world)
    c.resps = [
        s.search(c.keyword, token=buyer.token, filters={"sku.Colour Name": "x"}),
        s.search(c.keyword, token=buyer.token, filters={"tag.color": "den:other"}),
    ]


@when("a buyer searches for their keyword through the gateway")
def search_keyword(world: World) -> None:
    s.ctx(world).resp = None  # observed by polling in the Then


@when("a buyer opens /search for that keyword and selects the xanh-navy bucket of the color facet")
def ui_select(world: World) -> None:
    c = s.ctx(world)
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    search.navigate_query(c.keyword)
    search.page.wait_for_load_state("networkidle")
    group = search.facet_group("sku-color")
    expect(group).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(search.results_wrapper).to_be_visible(timeout=timeouts.NAVIGATION)
    search.wait_for_result_count(2, timeouts.NAVIGATION)
    group.locator('[data-testid="facet-bucket"][data-key="xanh-navy"]').click()
    search.page.wait_for_url(lambda url: "sku.color=xanh-navy" in url, timeout=timeouts.NAVIGATION)


@when("an anonymous caller and a signed-in buyer call ClassifyTags through the gateway")
def classify_at_edge(world: World) -> None:
    c = s.ctx(world)
    buyer = s.buyer_of(world)
    path = "/platform.ai.v1.AIService/ClassifyTags"
    body = {"title": "Tai nghe Bluetooth 5.3 chống ồn ANC"}
    c.resps = [s.post(path, body), s.post(path, body, buyer.token)]


# ── thens ────────────────────────────────────────────────────────────────
@then(
    "SearchListings with the filter tag.connectivity bluetooth-5-3 returns the listing, and the "
    "same search with tag.connectivity wifi-6 does not"
)
def tag_filter(world: World) -> None:
    listed = s.listing(world, "A")
    s.eventually(
        lambda: listed.id in _ids(world, {"tag.connectivity": "bluetooth-5-3"}),
        "the SPU tag filter to return the listing",
    )
    assert listed.id not in _ids(world, {"tag.connectivity": "wifi-6"})
    # positive control for the absence above: the keyword search itself returns it
    assert listed.id in _ids(world, {})


@then("only the first listing is returned")
def only_first(world: World) -> None:
    c = s.ctx(world)
    a, b = s.listing(world, "A").id, s.listing(world, "B").id
    filters = c.extra["filters"]
    s.eventually(lambda: _ids(world, filters) == {a}, "the filtered search to return only A")
    # controls: each condition alone matches both listings, so the exclusion of B is the
    # single-variant rule, not B missing from the index
    assert _ids(world, {"sku.color": "xanh-navy"}) == {a, b}
    assert _ids(world, {"sku.capacity": "512gb"}) == {a, b}


@then("only the listing whose matching variant is in stock is returned")
def only_in_stock(world: World) -> None:
    c = s.ctx(world)
    a, b = s.listing(world, "A").id, s.listing(world, "B").id
    filters = c.extra["filters"]
    s.eventually(lambda: _ids(world, filters) == {b}, "only B (in-stock navy 512GB) to match")
    # controls: A is indexed with in-stock variants (Titan 256GB) and is found by those
    assert a in _ids(world, {"sku.color": "titan-tu-nhien"})
    assert a not in _ids(world, {"sku.color": "xanh-navy"})


@then("both calls fail with unimplemented and no classification is returned")
def classify_unimplemented(world: World) -> None:
    for resp in s.ctx(world).resps:
        assert resp.status_code == 501, f"want 501, got {resp.status_code}: {resp.text}"
        assert resp.json().get("code") == "unimplemented", resp.text
        assert "tags" not in resp.json(), resp.text


@then("the call fails with invalid_argument and no search is run")
def invalid(world: World) -> None:
    for resp in s.ctx(world).resps:
        assert resp.status_code == 400, f"want 400, got {resp.status_code}: {resp.text}"
        assert resp.json().get("code") == "invalid_argument", resp.text


@then(
    "facets.tags lists a connectivity group with the bluetooth-5-3 bucket and facets.skus lists "
    "color and capacity groups, each bucket counting listings"
)
def facets_in_response(world: World) -> None:
    c = s.ctx(world)

    def seen() -> bool:
        body = s.ok_json(s.search(c.keyword, page_size=50))
        return (
            _group(body, "tags", "connectivity").get("bluetooth-5-3") == 2
            and _group(body, "skus", "color") == {"xanh-navy": 2, "titan-tu-nhien": 2}
            and _group(body, "skus", "capacity") == {"256gb": 2, "512gb": 2}
        )

    s.eventually(seen, "facets.tags/skus to carry per-listing counts for the two listings")


@then(
    "the URL contains sku.color=xanh-navy, only listings with an in-stock navy variant remain, "
    "and the selected value is shown as an active filter chip"
)
def ui_result(world: World) -> None:
    search: SearchPage = world.get_page(PageName.SEARCH)  # type: ignore[assignment]
    a, b = s.listing(world, "A"), s.listing(world, "B")
    assert "sku.color=xanh-navy" in search.page.url, search.page.url
    search.wait_for_result_count(1, timeouts.NAVIGATION)
    hrefs = search.result_hrefs()
    assert f"/listing/{a.id}" in hrefs and f"/listing/{b.id}" not in hrefs, hrefs
    expect(search.active_filters).to_contain_text("Màu sắc: Xanh navy")
    selected = search.facet_group("sku-color").locator('[data-key="xanh-navy"]')
    expect(selected).to_have_attribute("data-active", "true")
