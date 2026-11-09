from pathlib import Path
from typing import Any

import pytest
import yaml
from pydantic import ValidationError

from app.modules.business.recommendation.placements import (
    PLACEMENTS_FILE,
    Filters,
    LadderStep,
    Placement,
    load_placements,
)

REMOVED_KEYS = {"name", "enabled", "timeout_ms", "allow_cache", "tier"}


def _write(tmp_path: Path, placement_body: str) -> Path:
    path = tmp_path / "placements.yaml"
    path.write_text(
        "version: '1.0'\nplacements:\n  home_feed:\n"
        + placement_body
        + "    candidate_ladder:\n      - strategy: global_popular\n",
        encoding="utf-8",
    )
    return path


def _keys(node: Any) -> set[str]:
    found: set[str] = set()
    if isinstance(node, dict):
        for key, value in node.items():
            found.add(str(key))
            found |= _keys(value)
    elif isinstance(node, list):
        for value in node:
            found |= _keys(value)
    return found


def test_no_ignored_keys_remain():
    raw = yaml.safe_load(PLACEMENTS_FILE.read_text(encoding="utf-8"))
    assert _keys(raw) & REMOVED_KEYS == set()
    placements = load_placements()
    assert "home.trending" not in placements.by_id
    assert "home_feed" in placements.by_id


def test_cart_cross_sell_enables_deduplicate_sellers():
    f = load_placements().get("cart_cross_sell").filters
    assert f.deduplicate_sellers is True
    assert f.in_stock_only is True
    assert f.exclude_cart_items is True
    assert f.exclude_seed is False


def test_existing_filters_keep_their_values():
    placements = load_placements()
    assert placements.get("home_feed").filters.deduplicate_sellers is False
    assert placements.get("similar_items").filters.exclude_seed is True


def test_existing_placements_still_resolve():
    placements = load_placements()
    home = [s.strategy for s in placements.get("home_feed").candidate_ladder]
    similar = [s.strategy for s in placements.get("similar_items").candidate_ladder]
    assert home == [
        "user_precomputed",
        "seed_vector_similarity",
        "category_popular",
        "global_popular",
    ]
    assert similar == ["seed_vector_similarity", "category_popular", "global_popular"]
    assert placements.get("undefined_id") is placements.get("home_feed")


def test_unknown_filter_key_fails_at_load(tmp_path):
    path = _write(tmp_path, "    filters:\n      bogus: true\n")
    with pytest.raises(ValidationError, match="bogus"):
        load_placements(path)


def test_removed_key_is_rejected(tmp_path):
    path = _write(tmp_path, "    timeout_ms: 35\n")
    with pytest.raises(ValidationError, match="timeout_ms"):
        load_placements(path)


@pytest.mark.parametrize("key", ["name", "enabled", "allow_cache"])
def test_every_other_removed_placement_key_is_rejected(tmp_path, key):
    path = _write(tmp_path, f"    {key}: x\n")
    with pytest.raises(ValidationError, match=key):
        load_placements(path)


def test_ladder_step_with_tier_is_rejected(tmp_path):
    path = tmp_path / "placements.yaml"
    path.write_text(
        "version: '1.0'\nplacements:\n  home_feed:\n    candidate_ladder:\n"
        "      - tier: tier1\n        strategy: global_popular\n",
        encoding="utf-8",
    )
    with pytest.raises(ValidationError, match="tier"):
        load_placements(path)


def test_models_forbid_unknown_keys():
    with pytest.raises(ValidationError, match="tier"):
        LadderStep.model_validate({"strategy": "global_popular", "tier": "t"})
    with pytest.raises(ValidationError, match="bogus"):
        Filters.model_validate({"bogus": True})
    with pytest.raises(ValidationError, match="timeout_ms"):
        Placement.model_validate({"timeout_ms": 5})
