"""The GBDT stage: gate, registry, artifact and its place in the generation (no Spark)."""

from __future__ import annotations

import json

import pandas as pd
import pytest

from recsys.config import ConfigError, Settings
from recsys.load import redis_cache
from recsys.publish import publish_generation
from recsys.ranker import contract, gbdt, stage, training
from recsys.registry.registry import CHAMPION_KEY, GBDT_CHAMPION_KEY, ModelRegistry
from tests import ranker_fixture as fx
from tests.fakes import FakeQdrantClient, FakeRedis


def _settings(tmp_path, signal=True, **kw) -> Settings:
    values = fx.write_all(tmp_path, signal=signal)
    # No item has this many impressions: ctr stays the snapshot's constant, so only the price carries signal.
    values.update(gbdt_trees=25, gbdt_min_leaf=10, enable_gbdt=True, gbdt_ctr_min_impressions=10**6)
    values.update(kw)
    return Settings(**values)


def _run(settings, version="gen-1", registry=None):
    registry = registry or ModelRegistry()
    return stage.run_stage(settings, stage.resolve_inputs(settings), version, registry), registry


def test_the_trainer_learns_the_price_signal_and_beats_the_fixed_weight_baseline(tmp_path):
    result, _ = _run(_settings(tmp_path))
    m = result.summary["metrics"]
    assert result.promoted and result.summary["decision"] == "promoted", result.reason
    assert m["ndcg@10"] > m["baseline_ndcg@10"] * 1.05, m  # the baseline cannot tell the items apart
    assert m["eval_protocol"] == contract.EVAL_PROTOCOL and m["holdout_lists_with_positive"] > 20


def test_the_artifact_scores_a_cheap_item_above_an_expensive_one_the_way_the_trainer_does(tmp_path):
    result, _ = _run(_settings(tmp_path))
    art = json.loads(json.dumps(result.artifact))
    assert art["features"] == list(contract.RANKING_FEATURES) and art["generation"] == "gen-1"
    assert art["model_version"] == "gbdt-gen-1" and art["format"] == "agora-gbdt/1"
    base = [100, 5, 1, 1, 3, 4.0, 0.05, 0.0]
    cheap, dear = list(base), list(base)
    cheap[-1], dear[-1] = 20_000.0, 480_000.0
    assert gbdt.score_vector(art, cheap) > gbdt.score_vector(art, dear)


def test_a_promoted_candidate_is_recorded_and_gated_against_its_own_champion(tmp_path):
    result, registry = _run(_settings(tmp_path))
    meta = registry.get_model("gbdt-gen-1")
    assert meta.model_type == "gbdt" and meta.status == "candidate"  # promoted only after the publish
    assert registry.get_champion_version() is None  # the ALS champion pointer is not touched
    result.registry.apply_promotion(result.decision)
    assert registry.scoped(GBDT_CHAMPION_KEY).get_champion_version() == "gbdt-gen-1"
    assert registry.get_champion_version() is None
    assert registry.get_model("gbdt-gen-1").status == "champion"
    assert registry.champion_key == CHAMPION_KEY


def test_the_model_records_its_lineage_rows_and_ctr_sources(tmp_path):
    rows = tmp_path / "rows.parquet"
    result, registry = _run(_settings(tmp_path, gbdt_rows_path=str(rows)))
    p = registry.get_model("gbdt-gen-1").parameters
    assert p["dataset"]["name"] == "rank_training" and len(p["dataset"]["sha256"]) == 64
    assert p["feature_list"] == list(contract.RANKING_FEATURES)
    assert [s["snapshot"] for s in p["features"]["item_popularity"]["snapshots"]] == [
        f"as_of={fx.SNAPSHOT_AS_OF}.parquet"
    ]
    assert p["features"]["item_attributes"]["view"] == "item_attributes"
    assert p["rows"]["rows"] == fx.LISTS * fx.SHOWN and p["rows"]["rows_dropped_no_snapshot"] == 0
    # every training row records its CTR source
    written = pd.read_parquet(rows)
    assert len(written) == p["rows"]["rows"]
    assert set(written["ctr_source"]) <= {"debiased", "fallback"} and written["ctr_source"].notna().all()
    counts = written["ctr_source"].value_counts().to_dict()
    assert {k: counts.get(k, 0) for k in ("debiased", "fallback")} == p["ctr_source_counts"]
    assert set(written["split"]) == {"train", "holdout"}


def test_both_ctr_sources_occur_when_some_items_are_rare(tmp_path):
    s = _settings(tmp_path, gbdt_rows_path=str(tmp_path / "rows.parquet"), gbdt_ctr_min_impressions=100)
    result, registry = _run(s)
    counts = registry.get_model("gbdt-gen-1").parameters["ctr_source_counts"]
    assert counts["debiased"] > 0 and counts["fallback"] > 0  # ~100 impressions per item: about half pass


def test_the_holdout_is_strictly_later_than_training(tmp_path):
    _, registry = _run(_settings(tmp_path))
    h = registry.get_model("gbdt-gen-1").parameters["holdout"]
    assert h["train_last_at"] < h["test_first_at"] == h["cutoff"]
    assert h["train_lists"] + h["test_lists"] == fx.LISTS and h["test_lists"] == fx.LISTS // 5


def test_a_candidate_below_the_threshold_is_rejected_and_has_no_artifact(tmp_path):
    result, registry = _run(_settings(tmp_path, promotion_min_relative_improvement=10.0))
    assert not result.promoted and result.artifact is None and "baseline" in result.reason
    assert registry.get_model("gbdt-gen-1").status == "rejected"
    assert registry.get_model("gbdt-gen-1").parameters["gate_reason"] == result.reason


def test_a_signalless_dataset_does_not_pass_a_strict_gate(tmp_path):
    result, _ = _run(_settings(tmp_path, signal=False, promotion_min_relative_improvement=0.5))
    assert not result.promoted and result.artifact is None


def test_force_skips_the_gate(tmp_path):
    result, _ = _run(_settings(tmp_path, promotion_min_relative_improvement=10.0, promotion_force=True))
    assert result.promoted and result.artifact is not None


def test_a_second_run_on_the_same_data_is_gated_against_the_champion(tmp_path):
    s = _settings(tmp_path)
    first, registry = _run(s, "gen-1")
    first.registry.apply_promotion(first.decision)
    second, _ = _run(s, "gen-2", registry)
    assert not second.promoted and "improvement" in second.reason  # equal metrics: below the 1% threshold
    assert registry.scoped(GBDT_CHAMPION_KEY).get_champion_version() == "gbdt-gen-1"


def test_rows_before_every_snapshot_are_dropped_and_a_run_with_none_left_is_rejected(tmp_path):
    s = _settings(tmp_path)
    fx.write_popularity(tmp_path / "late", as_of="20261101T000000Z")  # a snapshot after every impression
    result, registry = _run(Settings(**{**s.__dict__, "item_features_dir": str(tmp_path / "late")}))
    assert not result.promoted and "no usable rows" in result.reason
    assert (
        registry.get_model("gbdt-gen-1").parameters["rows"]["rows_dropped_no_snapshot"] == fx.LISTS * fx.SHOWN
    )


def test_missing_inputs_refuse_to_start_naming_the_setting(tmp_path):
    with pytest.raises(ConfigError, match="RANK_DATASET_DIR="):
        stage.resolve_inputs(Settings(rank_dataset_dir=str(tmp_path / "none")))
    fx.write_rank_dataset(tmp_path / "rank", fx.rank_rows())
    with pytest.raises(ConfigError, match="ITEM_FEATURES_DIR="):
        stage.resolve_inputs(
            Settings(rank_dataset_dir=str(tmp_path / "rank"), item_features_dir=str(tmp_path / "x"))
        )
    with pytest.raises(ConfigError, match="RANK_DATASET_PATH="):
        stage.resolve_inputs(Settings(rank_dataset_path=str(tmp_path / "nope.parquet")))


def test_the_ranker_artifact_belongs_to_its_generation(tmp_path):
    """Written with the generation, TTL-refreshed and deleted with it (only serving and previous stay)."""
    settings = Settings(write_legacy_keys=False, als_rank=2)
    redis, qdrant = FakeRedis(), FakeQdrantClient()
    art = gbdt.to_artifact(gbdt.Model([], 0.1), model_version="gbdt-g", generation="g", metrics={})
    for gen in ("g1", "g2", "g3"):
        out = publish_generation(
            settings, gen, {"u": [("a", 1.0)]}, {"a": [("b", 1.0)]}, [("a", 1.0)],
            item_rows=[("a", [1.0, 0.0]), ("b", [0.0, 1.0])], user_rows=[("u", [1.0, 0.0])],
            redis_client=redis, qdrant_client=qdrant, ranker_artifact={**art, "generation": gen},
        )  # fmt: skip
        assert out["ranker_key"] == settings.gen_ranker_key(gen)
    keys = {g: settings.gen_ranker_key(g) for g in ("g1", "g2", "g3")}
    assert redis.get(keys["g1"]) is None  # retired with its generation
    for g in ("g2", "g3"):
        assert json.loads(redis.get(keys[g]))["generation"] == g and redis.ttl(keys[g]) > 0
    redis.ttl_set[keys["g2"]] = 5
    redis_cache.refresh_ttl(settings, {"g2"}, client=redis)
    assert redis.ttl(keys["g2"]) == settings.cache_ttl_seconds  # the TTL refresh reaches the ranker key
    publish_generation(settings, "g4", {"u": []}, {"a": []}, [], item_rows=[("a", [1.0, 0.0])],
                       user_rows=[("u", [1.0, 0.0])], redis_client=redis, qdrant_client=qdrant)  # fmt: skip
    assert redis.get(settings.gen_ranker_key("g4")) is None  # no artifact written when none is promoted


def test_the_default_run_does_not_read_the_ranking_settings():
    s = Settings()
    assert s.enable_gbdt is False and s.rank_dataset_dir.endswith("rank_training/v1")


def test_the_training_columns_follow_the_contract(tmp_path):
    result, _ = _run(_settings(tmp_path, gbdt_rows_path=str(tmp_path / "r.parquet")))
    assert result.promoted
    written = pd.read_parquet(tmp_path / "r.parquet")
    assert list(contract.RANKING_FEATURES) == [c for c in written.columns if c in contract.RANKING_FEATURES]
    assert (written["item_attributes.price"] > 0).all() and training.CTR_COL == 6
