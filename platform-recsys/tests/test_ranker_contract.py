"""The GBDT ranker's feature list is the registry's, in order, and team-ai's (recsys-gbdt-trainer)."""

from __future__ import annotations

import ast
import pathlib

import pytest
import yaml

from recsys.ranker import contract, gbdt

REPO = pathlib.Path(__file__).resolve().parents[2]
REGISTRY = REPO / "platform-featurestore" / "registry" / "features.yaml"
TEAM_AI = REPO / "team-ai" / "app" / "modules" / "business" / "recommend" / "features.py"


def test_the_list_is_the_item_popularity_features_in_registry_order_then_price():
    names = contract.RANKING_FEATURES
    assert len(names) == len(set(names)) == 8
    registry_order = (
        "views_7d",
        "clicks_7d",
        "add_to_cart_7d",
        "favorites_current",
        "review_count",
        "avg_rating",
        "ctr_7d",
    )
    assert names[:7] == tuple(f"item_popularity.{f}" for f in registry_order)
    assert names[7] == "item_attributes.price" and contract.CTR_FEATURE in names


@pytest.mark.skipif(not REGISTRY.exists(), reason="platform-featurestore is not checked out")
def test_the_list_matches_the_featurestore_registry():
    views = {(v["name"], v["version"]): v for v in yaml.safe_load(REGISTRY.read_text())["views"]}
    for view, version in contract.FEATURE_VIEWS.items():
        assert (view, version) in views, f"{view}@v{version} is not in the registry"
    for view in contract.FEATURE_VIEWS:
        declared = list(views[(view, contract.FEATURE_VIEWS[view])]["features"])
        wanted = [contract.split_name(n)[1] for n in contract.RANKING_FEATURES if n.startswith(view + ".")]
        assert wanted == [f for f in declared if f in wanted], f"{view}: order differs from the registry"
        assert set(wanted) <= set(declared)
    assert views[("item_attributes", 1)]["features"]["price"] == "int"


@pytest.mark.skipif(not TEAM_AI.exists(), reason="team-ai is not checked out")
def test_the_list_matches_team_ais():
    tree = ast.parse(TEAM_AI.read_text())
    values = [
        ast.literal_eval(node.value)
        for node in ast.walk(tree)
        if isinstance(node, ast.AnnAssign) and getattr(node.target, "id", "") == "RANKING_FEATURES"
    ]
    assert values, "team-ai recommend/features.py has no RANKING_FEATURES"
    assert tuple(values[0]) == contract.RANKING_FEATURES


def test_the_artifact_carries_the_contract():
    model = gbdt.Model([], 0.1)
    art = gbdt.to_artifact(model, model_version="gbdt-x", generation="x", metrics={})
    assert art["features"] == list(contract.RANKING_FEATURES)
    assert art["feature_views"] == contract.FEATURE_VIEWS and art["format"] == "agora-gbdt/1"
    assert set(art["defaults"]) == set(contract.RANKING_FEATURES)
