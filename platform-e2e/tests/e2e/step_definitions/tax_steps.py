"""Steps for ai/tag_classifier_taxonomy.feature, black box over team-ai's REST tag routes."""

from __future__ import annotations

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import tax_support as tax


@pytest.fixture
def tax_ctx() -> dict:
    return {}


def _ok(r) -> dict:
    assert r.status_code == 200, f"{r.request.url} -> {r.status_code}: {r.text}"
    return r.json()


# ── SPU Level Tag Classification ─────────────────────────────────────────────
@given(parsers.parse('a seller provides product title "{title}"'))
def seller_title(tax_ctx: dict, title: str) -> None:
    tax_ctx["title"] = title


@when(parsers.parse('the seller requests tag classification for category "{category_id}"'))
def classify_spu(tax_ctx: dict, category_id: str) -> None:
    body = {"title": tax_ctx["title"], "category_id": category_id}
    tax_ctx["classify"] = _ok(tax.post("/classify", body))


@then(parsers.parse('the system returns canonical tags "{t1}", "{t2}", "{t3}"'))
def canonical_tags(tax_ctx: dict, t1: str, t2: str, t3: str) -> None:
    slugs = [t["slug"] for t in tax_ctx["classify"]["canonical_tags"]]
    for t in (t1, t2, t3):
        assert t in slugs, (t, slugs)


@then(parsers.parse('the tags are mapped to facet groups "{g1}", "{g2}", "{g3}"'))
def facet_groups(tax_ctx: dict, g1: str, g2: str, g3: str) -> None:
    filters = tax_ctx["classify"]["suggested_facet_filters"]
    for g in (g1, g2, g3):
        assert g in filters, (g, filters)


# ── Granular SKU Level Hierarchical Classification ───────────────────────────
@given(parsers.parse('a parent listing "{spu_title}"'))
def parent_listing(tax_ctx: dict, spu_title: str) -> None:
    tax_ctx["spu_title"] = spu_title


@given(parsers.parse('child SKU variants "{v1}" and "{v2}"'))
def child_variants(tax_ctx: dict, v1: str, v2: str) -> None:
    variants = []
    for i, (name, price, stock) in enumerate(((v1, 29_990_000, 15), (v2, 34_990_000, 10)), 1):
        color, capacity = (p.strip() for p in name.split("/"))
        variants.append(
            {
                "variant_id": f"sku-{i}",
                "name": name,
                "price": price,
                "stock": stock,
                "options": {"color": color, "capacity": capacity},
            }
        )
    tax_ctx["variants"] = variants


@when("hierarchical SKU classification is executed")
def classify_hierarchy(tax_ctx: dict) -> None:
    body = {
        "spu_title": tax_ctx["spu_title"],
        "category_id": "cat-electronics",
        "variants": tax_ctx["variants"],
    }
    tax_ctx["sku"] = _ok(tax.post("/classify-sku-hierarchy", body))


EXPECTED_FACETS = [
    {"color": "titan-tu-nhien", "capacity": "256gb"},
    {"color": "xanh-navy", "capacity": "512gb"},
]


@then("each SKU inherits common tags and receives specific variant facets for color and capacity")
def sku_facets(tax_ctx: dict) -> None:
    res = tax_ctx["sku"]
    assert res["total_skus_processed"] == 2
    spu_slugs = {t["slug"] for t in res["spu_canonical_tags"]}
    assert "chong-nuoc-ipx7" in spu_slugs, spu_slugs
    for sku, expected in zip(res["sku_results"], EXPECTED_FACETS, strict=True):
        assert sku["variant_facets"] == expected, sku["variant_facets"]
        assert {t["slug"] for t in sku["inherited_spu_tags"]} == spu_slugs
        effective = {t["slug"] for t in sku["all_effective_tags"]}
        assert spu_slugs | set(expected.values()) <= effective, effective
        # a variant never carries the other variant's dimension values
        other = next(e for e in EXPECTED_FACETS if e is not expected)
        assert not (set(other.values()) & effective), effective


@then("an OpenSearch nested document payload is generated")
def nested_doc(tax_ctx: dict) -> None:
    doc = tax_ctx["sku"]["nested_opensearch_doc"]
    assert doc["title"] == tax_ctx["spu_title"]
    assert "chong-nuoc-ipx7" in doc["spu_tags"]
    assert doc["price_min"] == 29_990_000 and doc["price_max"] == 34_990_000
    assert doc["total_stock"] == 25 and doc["is_in_stock"] is True
    assert "spu_facets" in doc
    assert len(doc["variants"]) == 2
    for var, src, expected in zip(
        doc["variants"], tax_ctx["variants"], EXPECTED_FACETS, strict=True
    ):
        assert var["variant_id"] == src["variant_id"]
        assert var["facets"] == expected
        assert (var["price"], var["stock"], var["is_in_stock"]) == (
            src["price"],
            src["stock"],
            True,
        )
        assert set(expected.values()) <= set(var["tags"])


# ── Candidate Tag Discovery from Raw Batch ───────────────────────────────────
def _listing(i: int, title: str) -> dict:
    return {"listing_id": f"tax-{i}", "title": title, "category_id": "cat-electronics"}


def _explore_batch(watt: int, lone: int | None = None) -> list[dict]:
    items = [
        _listing(1, f"Sạc nhanh GaN công suất {watt}W Anker"),
        _listing(2, f"Củ sạc laptop công suất {watt}W Baseus"),
    ]
    if lone is not None:
        items.append(_listing(3, f"Sạc dự phòng công suất {lone}W Xiaomi"))
    return items


@given("a batch of unclassified listings with emergent specs")
def raw_batch(tax_ctx: dict) -> None:
    watt, lone = tax.fresh_watts(2)
    tax_ctx.update(watt=watt, lone=lone, batch=_explore_batch(watt, lone))
    existing = {t["slug"] for t in tax.list_tags()}
    assert tax.slug_for(watt) not in existing and tax.slug_for(lone) not in existing


@when(parsers.parse("offline exploration is executed with frequency threshold {min_freq:d}"))
def explore(tax_ctx: dict, min_freq: int) -> None:
    body = {"batch_listings": tax_ctx["batch"], "min_frequency": min_freq, "min_confidence": 0.80}
    tax_ctx["explore"] = _ok(tax.post("/explore", body))


@then("candidate tags are discovered and registered with status EXPLORING")
def candidates_registered(tax_ctx: dict) -> None:
    slug, lone = tax.slug_for(tax_ctx["watt"]), tax.slug_for(tax_ctx["lone"])
    found = {c["slug"]: c for c in tax_ctx["explore"]["discovered_candidates"]}
    assert slug in found, list(found)
    assert found[slug]["status"] == "exploring" and found[slug]["is_canonical"] is False
    assert found[slug]["occurrence_count"] >= 2
    # a spec seen once stays below the frequency threshold: not discovered, not registered
    assert lone not in found
    stored = {t["slug"]: t for t in tax.list_tags(status="exploring")}
    assert slug in stored and stored[slug]["is_canonical"] is False
    assert lone not in {t["slug"] for t in tax.list_tags()}


# ── Candidate Tag Promoted to Canonical Filter Facet ─────────────────────────
@given("an exploring candidate tag with a wattage no earlier run used")
def exploring_candidate(tax_ctx: dict) -> None:
    (watt,) = tax.fresh_watts(1)
    body = {"batch_listings": _explore_batch(watt), "min_frequency": 2, "min_confidence": 0.80}
    _ok(tax.post("/explore", body))
    slug = tax.slug_for(watt)
    assert slug in {t["slug"] for t in tax.list_tags(status="exploring")}
    tax_ctx.update(watt=watt, slug=slug)


@when('promotion is executed with target category "cat-electronics" and a bound synonym')
def promote(tax_ctx: dict) -> None:
    tax_ctx["synonym"] = f"sac {tax_ctx['watt']}w"
    body = {
        "tag_slugs": [tax_ctx["slug"]],
        "target_category_id": "cat-electronics",
        "add_synonyms": [tax_ctx["synonym"]],
    }
    tax_ctx["promote"] = _ok(tax.post("/promote", body))


@then("the tag status becomes PROMOTED and is active for search filter facets")
def promoted_active(tax_ctx: dict) -> None:
    res, slug = tax_ctx["promote"], tax_ctx["slug"]
    assert res["total_promoted"] == 1
    tag = res["promoted_tags"][0]
    assert tag["slug"] == slug and tag["status"] == "promoted" and tag["is_canonical"] is True
    assert tax_ctx["synonym"] in tag["synonyms"]
    # no longer a candidate; now in the canonical registry
    assert slug not in {t["slug"] for t in tax.list_tags(status="exploring")}
    assert slug in {t["slug"] for t in tax.list_tags(status="promoted")}
    # a listing classified after the promotion carries it, by name and by the bound synonym
    for title in (f"Củ sạc nhanh công suất {tax_ctx['watt']}W", f"Sạc {tax_ctx['watt']}W gọn nhẹ"):
        res = _ok(tax.post("/classify", {"title": title, "category_id": "cat-electronics"}))
        assert slug in [t["slug"] for t in res["canonical_tags"]], (title, res["canonical_tags"])
        assert slug in res["suggested_facet_filters"].get("power", []), res[
            "suggested_facet_filters"
        ]
