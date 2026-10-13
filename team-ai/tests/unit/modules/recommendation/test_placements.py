"""The placement schema: team-ai's placements.yaml, typed, validated at load."""

from pathlib import Path

import pytest
from pydantic import ValidationError

from app.modules.business.recommendation.placements import (
    PLACEMENTS_FILE,
    load_placements,
)


def test_the_shipped_file_declares_the_three_team_ai_placements() -> None:
    placements = load_placements()
    assert set(placements.by_id) == {"home_feed", "similar_items", "cart_cross_sell"}


def test_home_feed_runs_the_four_step_ladder_on_the_gbdt_ranker() -> None:
    home = load_placements().get("home_feed")
    assert [s.strategy for s in home.candidate_ladder] == [
        "user_precomputed",
        "seed_vector_similarity",
        "category_popular",
        "global_popular",
    ]
    assert {s.min_candidates for s in home.candidate_ladder} == {1}
    assert (home.ranking.model, home.ranking.use_featurestore) == ("gbdt", True)
    assert home.result_limit == 10


def test_similar_items_ranks_by_similarity_without_the_featurestore() -> None:
    similar = load_placements().get("similar_items")
    assert (similar.ranking.model, similar.ranking.use_featurestore) == (
        "cosine_rank",
        False,
    )
    assert similar.candidate_ladder[0].strategy == "seed_vector_similarity"


def test_cart_cross_sell_limit_is_eight() -> None:
    assert load_placements().get("cart_cross_sell").result_limit == 8


def test_an_unknown_placement_gets_the_home_feed_config() -> None:
    placements = load_placements()
    assert placements.get("nope") is placements.get("home_feed")


def test_the_yaml_is_the_team_ai_file_verbatim() -> None:
    text = PLACEMENTS_FILE.read_text(encoding="utf-8")
    assert text.startswith("# Recommendation Placements Configuration (ADR-0012)")
    assert "deduplicate_sellers: false" in text  # unenforced keys are kept, not dropped


def _write(tmp_path: Path, body: str) -> Path:
    path = tmp_path / "placements.yaml"
    path.write_text(body, encoding="utf-8")
    return path


def test_an_unbound_strategy_is_rejected_at_load(tmp_path: Path) -> None:
    path = _write(
        tmp_path,
        """
placements:
  home_feed:
    candidate_ladder:
      - strategy: telepathy
""",
    )
    with pytest.raises(ValidationError):
        load_placements(path)


def test_an_unbound_ranking_model_is_rejected_at_load(tmp_path: Path) -> None:
    path = _write(
        tmp_path,
        """
placements:
  home_feed:
    ranking:
      model: deep_transformer
""",
    )
    with pytest.raises(ValidationError):
        load_placements(path)


def test_a_file_without_home_feed_is_rejected(tmp_path: Path) -> None:
    path = _write(tmp_path, "placements:\n  other:\n    result_limit: 3\n")
    with pytest.raises(ValidationError):
        load_placements(path)


def test_defaults_for_a_minimal_placement(tmp_path: Path) -> None:
    path = _write(tmp_path, "placements:\n  home_feed: {}\n")
    home = load_placements(path).get("home_feed")
    assert home.candidate_ladder == []
    assert home.ranking.model == "cosine_rank"
    assert home.ranking.use_featurestore is False
    assert home.result_limit == 0  # 0 = use RECS_RESULT_TOP_K
