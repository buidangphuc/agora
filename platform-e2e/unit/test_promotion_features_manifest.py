"""Unit tests (no stack): team-promotion/FEATURES.yaml after promotion-go-sdk-v1 task 2.1."""

from __future__ import annotations

import json
import re
from pathlib import Path

import jsonschema
import pytest
import yaml

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / "team-promotion" / "FEATURES.yaml"
SCHEMA = ROOT / "platform-e2e" / "schemas" / "features.schema.json"
OLD_PACKAGE = re.compile(r"internal/(service|handler|repository)\b")


@pytest.fixture(scope="module")
def doc() -> dict:
    return yaml.safe_load(MANIFEST.read_text())


@pytest.fixture(scope="module")
def by_id(doc: dict) -> dict:
    return {f["id"]: f for f in doc["features"]}


def test_manifest_validates_against_schema_scenario_concurrent_reserves_are_capped(doc):
    jsonschema.validate(doc, json.loads(SCHEMA.read_text()))


def test_runtime_hardening_entry_scenario_concurrent_reserves_are_capped(by_id):
    f = by_id["promotion.service-runtime-hardening"]
    assert f["persona"] == "admin"
    assert f["priority"] == "P1"
    assert f["kind"] == "async_event"
    assert f["services"] == ["team-promotion"]
    assert f["api"]["exercised"] == [
        "promotion.v1.CreateVoucher",
        "promotion.v1.ReserveFlashSaleStock",
    ]
    assert f["status"] == "planned"
    assert not f.get("covered_by")
    text = " ".join(f["acceptance"]).lower()
    for word in ("postgres", "kafka", "flipt", "voucherchanged"):
        assert word in text


def test_duplicate_voucher_code_entry_scenario_concurrent_reserves_are_capped(by_id):
    f = by_id["promotion.duplicate-voucher-code-conflict"]
    assert f["persona"] == "admin"
    assert f["status"] == "planned"
    assert not f.get("covered_by")
    assert "promotion.v1.CreateVoucher" in f["api"]["exercised"]
    assert "AlreadyExists" in " ".join(f["acceptance"])


def test_no_note_cites_a_deleted_package_scenario_concurrent_reserves_are_capped(doc):
    stale = [f["id"] for f in doc["features"] if OLD_PACKAGE.search(f.get("notes", ""))]
    assert stale == []


def test_no_note_says_skipped_without_a_database_scenario_concurrent_reserves_are_capped(doc):
    bad = [f["id"] for f in doc["features"] if "skipped without a database" in f.get("notes", "")]
    assert bad == []


def test_concurrent_cap_entry_cites_new_packages_scenario_concurrent_reserves_are_capped(by_id):
    f = by_id["promotion.flash-stock-hold-concurrent-reserves-are-capped"] if (
        "promotion.flash-stock-hold-concurrent-reserves-are-capped" in by_id
    ) else next(x for x in by_id.values() if x.get("covered_by", "").endswith("::Concurrent reserves are capped"))
    assert f["covered_by"] == "promo/flash_sale_stock_hold.feature::Concurrent reserves are capped"
    assert "`internal/flashsale` `TestReserveFlashSaleStockConcurrentCap`" in f["notes"]
    assert "`internal/store` `TestPostgresFlashHoldConcurrentCap`" in f["notes"]


def test_every_cited_test_uses_the_mapped_package_scenario_concurrent_reserves_are_capped(doc):
    mapping = {}
    for line in (ROOT / "openspec/changes/promotion-go-sdk-v1/test-package-map.txt").read_text().split("\n"):
        parts = line.split()
        if len(parts) == 3:
            mapping[parts[1]] = parts[2]
    wrong = []
    for f in doc["features"]:
        for pkg, name in re.findall(r"`(internal/[a-z]+)` `(Test\w+)`", f.get("notes", "")):
            if name in mapping and mapping[name] != pkg:
                wrong.append((f["id"], name, pkg))
    assert wrong == []


def test_existing_entries_kept_scenario_concurrent_reserves_are_capped(doc):
    assert len(doc["features"]) >= 114
    ids = [f["id"] for f in doc["features"]]
    assert len(ids) == len(set(ids))
