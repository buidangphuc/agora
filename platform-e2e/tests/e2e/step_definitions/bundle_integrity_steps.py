"""Bundle integrity (domain-bundle-integrity): CreateBundle / GetBundle / ListBundlesBySeller
through the gateway. Listings are seeded through the real API; the legacy bundle (B12) is
inserted straight into team-domain's `bundles` table because the API now refuses it."""

from __future__ import annotations

import os
import subprocess
import uuid

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support.oic_order_support import (
    LISTING,
    Actor,
    OicWorld,
    code_of,
    ok,
    post,
    register,
)


def _psql(sql: str) -> str:
    out = subprocess.run(
        [
            "docker", "exec", os.getenv("BDL_POSTGRES_CONTAINER", "agora-postgres-1"),
            "psql", "-U", "postgres", "-d", "listing_db", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        ],
        capture_output=True, text=True, timeout=60, check=True,
    )  # fmt: skip
    return out.stdout


@pytest.fixture
def bdl():
    w = OicWorld()
    w.data.update(own=[], calls=[], legacy_ids=[])
    yield w
    for bundle_id in w.data["legacy_ids"]:
        _psql(f"DELETE FROM bundles WHERE id = '{bundle_id}'")


def _new_listing(w: OicWorld, owner: Actor, price: int, variants: bool = False) -> str:
    listing = {
        "title": f"[BDL] {uuid.uuid4().hex[:8]}",
        "categoryId": "cat-electronics",
        "price": price,
        "stock": 50,
        "status": "LISTING_STATUS_PUBLISHED",
        "currency": "VND",
        "description": "bundle integrity e2e",
    }
    if variants:
        listing["variants"] = [
            {"name": "Red", "sku": f"R-{uuid.uuid4().hex[:6]}", "price": price, "stock": 5},
            {"name": "Blue", "sku": f"B-{uuid.uuid4().hex[:6]}", "price": price, "stock": 5},
        ]
    resp = post(w, owner, LISTING, "CreateListing", {"listing": listing})
    listing_id = (ok(resp).get("listing") or {}).get("id", "")
    assert listing_id, resp.text
    return listing_id


def _seller(w: OicWorld) -> Actor:
    return w.actors["seller"]


def _bundles_of_seller(w: OicWorld) -> list[dict]:
    seller = _seller(w)
    resp = post(w, seller, LISTING, "ListBundlesBySeller", {"sellerId": seller.user_id})
    return ok(resp).get("bundles") or []


@given(parsers.parse("a seller owns published listings priced {a:d} and {b:d}"))
def seller_two_listings(bdl: OicWorld, a: int, b: int) -> None:
    seller = register(bdl, "seller", "seller")
    bdl.data["own"] = [_new_listing(bdl, seller, a), _new_listing(bdl, seller, b)]


@given(parsers.parse("a seller owns {n:d} published listings priced {price:d} each"))
def seller_n_listings(bdl: OicWorld, n: int, price: int) -> None:
    seller = register(bdl, "seller", "seller")
    bdl.data["own"] = [_new_listing(bdl, seller, price) for _ in range(n)]


@given(parsers.parse("another seller owns a published listing priced {price:d}"))
def other_seller_listing(bdl: OicWorld, price: int) -> None:
    other = register(bdl, "other", "seller")
    bdl.data["foreign"] = _new_listing(bdl, other, price)


@given(parsers.parse("the seller owns a published listing with variants priced {price:d}"))
def seller_variant_listing(bdl: OicWorld, price: int) -> None:
    bdl.data["variant"] = _new_listing(bdl, _seller(bdl), price, variants=True)


@given("a buyer is signed in")
def buyer_signed_in(bdl: OicWorld) -> None:
    register(bdl, "buyer", "buyer")


def _resolve(w: OicWorld, ref: str) -> str:
    ref = ref.strip()
    if ref == "unknown":
        return f"missing-{uuid.uuid4().hex}"
    if ref in ("foreign", "variant"):
        return w.data[ref]
    assert ref.startswith("own"), ref
    return w.data["own"][int(ref[3:]) - 1]


def _create(w: OicWorld, actor: Actor, ids: list[str], price: int) -> None:
    resp = post(
        w, actor, LISTING, "CreateBundle",
        {"title": f"bundle {uuid.uuid4().hex[:6]}", "listingIds": ids, "bundlePrice": price},
    )  # fmt: skip
    w.responses["bundle"] = resp
    w.data["calls"].append(resp)


@when(parsers.parse('the seller creates a bundle of "{refs}" priced {price:d}'))
def seller_creates_refs(bdl: OicWorld, refs: str, price: int) -> None:
    ids = [_resolve(bdl, r) for r in refs.split(",")]
    bdl.data["requested_ids"] = ids
    _create(bdl, _seller(bdl), ids, price)


@when(parsers.parse('the buyer creates a bundle of "{refs}" priced {price:d}'))
def buyer_creates_refs(bdl: OicWorld, refs: str, price: int) -> None:
    _create(bdl, bdl.actors["buyer"], [_resolve(bdl, r) for r in refs.split(",")], price)


@when(parsers.parse("the seller creates a bundle of the first {n:d} listings priced {price:d}"))
def seller_creates_first_n(bdl: OicWorld, n: int, price: int) -> None:
    _create(bdl, _seller(bdl), bdl.data["own"][:n], price)


@then("the bundle call succeeds")
def call_succeeds(bdl: OicWorld) -> None:
    resp = bdl.responses["bundle"]
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:400]}"
    bdl.data["bundle"] = resp.json().get("bundle") or {}
    assert bdl.data["bundle"].get("id"), resp.text


@then(parsers.parse("the bundle call fails with {code}"))
def call_fails(bdl: OicWorld, code: str) -> None:
    resp = bdl.responses["bundle"]
    assert resp.status_code != 200, f"expected {code}, call succeeded: {resp.text[:300]}"
    assert code_of(resp).upper() == code, f"expected {code}, got {code_of(resp)}: {resp.text[:300]}"


@then(parsers.parse('the bundle belongs to the seller and lists "{refs}" in that order'))
def bundle_owner_and_order(bdl: OicWorld, refs: str) -> None:
    bundle = bdl.data["bundle"]
    assert bundle.get("sellerId") == _seller(bdl).user_id, bundle
    assert bundle.get("listingIds") == [_resolve(bdl, r) for r in refs.split(",")], bundle


@then("GetBundle returns the created bundle")
def get_bundle_returns_created(bdl: OicWorld) -> None:
    created = bdl.data["bundle"]
    resp = post(bdl, _seller(bdl), LISTING, "GetBundle", {"id": created["id"]})
    got = ok(resp).get("bundle") or {}
    assert got.get("id") == created["id"], got
    assert got.get("listingIds") == created.get("listingIds"), got
    assert int(got.get("bundlePrice", 0)) == int(created.get("bundlePrice", 0)), got


@then(parsers.parse("the created bundle has {n:d} listing ids"))
def created_has_n_ids(bdl: OicWorld, n: int) -> None:
    assert len(bdl.data["bundle"].get("listingIds") or []) == n, bdl.data["bundle"]


@then(parsers.parse("the created bundle has price {price:d}"))
def created_has_price(bdl: OicWorld, price: int) -> None:
    assert int(bdl.data["bundle"].get("bundlePrice", 0)) == price, bdl.data["bundle"]


@then("both bundle calls carry the same message")
def same_message(bdl: OicWorld) -> None:
    first, second = bdl.data["calls"][-2:]
    assert code_of(first) == code_of(second) == "permission_denied", (first.text, second.text)
    assert first.json().get("message") == second.json().get("message"), (first.text, second.text)


@then("the seller has no stored bundle")
def seller_has_none(bdl: OicWorld) -> None:
    bundles = _bundles_of_seller(bdl)
    assert not bundles, f"a bundle was stored: {bundles}"


@then(parsers.parse("the seller has exactly {n:d} stored bundle"))
def seller_has_n(bdl: OicWorld, n: int) -> None:
    assert len(_bundles_of_seller(bdl)) == n


@given(parsers.parse('a bundle of "{refs}" priced {price:d} was stored before the rules'))
def legacy_bundle(bdl: OicWorld, refs: str, price: int) -> None:
    ids = [_resolve(bdl, r) for r in refs.split(",")]
    bundle_id = f"legacy-{uuid.uuid4().hex[:12]}"
    array = ", ".join(f"'{i}'" for i in ids)
    _psql(
        "INSERT INTO bundles (id, seller_id, title, listing_ids, bundle_price) VALUES "
        f"('{bundle_id}', '{_seller(bdl).user_id}', 'legacy bundle', ARRAY[{array}]::text[], {price})"
    )
    bdl.data["legacy_ids"].append(bundle_id)
    bdl.data["legacy"] = {"id": bundle_id, "listingIds": ids, "bundlePrice": price}


@when("the legacy bundle is requested through GetBundle")
def get_legacy(bdl: OicWorld) -> None:
    bdl.responses["get_legacy"] = post(
        bdl, _seller(bdl), LISTING, "GetBundle", {"id": bdl.data["legacy"]["id"]}
    )


@when("the seller's bundles are listed")
def list_legacy(bdl: OicWorld) -> None:
    bdl.data["listed"] = _bundles_of_seller(bdl)


def _assert_unchanged(got: dict, legacy: dict) -> None:
    assert got.get("id") == legacy["id"], got
    assert got.get("listingIds") == legacy["listingIds"], got
    assert int(got.get("bundlePrice", 0)) == legacy["bundlePrice"], got


@then("GetBundle returns the legacy bundle unchanged")
def legacy_via_get(bdl: OicWorld) -> None:
    _assert_unchanged(ok(bdl.responses["get_legacy"]).get("bundle") or {}, bdl.data["legacy"])


@then("the seller's bundle list contains the legacy bundle unchanged")
def legacy_in_list(bdl: OicWorld) -> None:
    match = [b for b in bdl.data["listed"] if b.get("id") == bdl.data["legacy"]["id"]]
    assert match, f"legacy bundle missing from {bdl.data['listed']}"
    _assert_unchanged(match[0], bdl.data["legacy"])


@when("the seller requests GetBundle for an unknown bundle id")
def get_unknown(bdl: OicWorld) -> None:
    bdl.responses["bundle"] = post(
        bdl, _seller(bdl), LISTING, "GetBundle", {"id": f"missing-{uuid.uuid4().hex}"}
    )
