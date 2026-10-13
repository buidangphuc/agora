"""The GBDT stage: gate, registry, artifact and its place in the generation (no Spark).

Ported to the MLflow registry (model ``recsys-gbdt``).

INTERFACE ASSUMPTION: ``stage.run_stage(settings, inputs, model_version) -> RankerResult`` starts its own
MLflow run, calls ``registry_client.evaluate`` (or ``register_rejected`` for a structural rejection) on
``recsys-gbdt`` and returns
  .decision : the modelregistry.Decision (outcome ``promotable`` | ``rejected``; a stage never applies it)
  .reason   : the decision's reason
  .artifact : the ``agora-gbdt/1`` dict when promotable, else None
  .summary  : metrics (incl. ``baseline_ndcg@10``, ``holdout_lists_with_positive``), ``rows``, ``ctr_source``,
              ``holdout`` as before
"""

from __future__ import annotations

import json

import pandas as pd
import pytest
from modelregistry import apply

from recsys.config import ConfigError, Settings
from recsys.load import redis_cache
from recsys.publish import publish_generation
from recsys.ranker import contract, gbdt, stage, training
from tests import ranker_fixture as fx
from tests.fakes import FakeQdrantClient, FakeRedis
from tests.pipeline_fixture import GBDT, newest, outcome, registered_models, seed_strong, versions


def _settings(tmp_path, reg, signal=True, **kw) -> Settings:
    values = fx.write_all(tmp_path, signal=signal)
    # No item has this many impressions: ctr stays the snapshot's constant, so only the price carries signal.
    values.update(gbdt_trees=25, gbdt_min_leaf=10, enable_gbdt=True, gbdt_ctr_min_impressions=10**6)
    values.update(mlflow_tracking_uri=reg.tracking_uri)
    values.update(kw)
    return Settings(**values)


def _run(settings, version="gen-1"):
    return stage.run_stage(settings, stage.resolve_inputs(settings), version)


def _recorded(reg, version) -> str:
    """Everything the registry holds about a version: tags and the params of its run."""
    mv = reg.client.get_model_version(GBDT, str(version))
    return json.dumps([dict(mv.tags), dict(reg.client.get_run(mv.run_id).data.params)])


def test_the_trainer_learns_the_price_signal_and_beats_the_fixed_weight_baseline(tmp_path, reg):
    result = _run(_settings(tmp_path, reg))
    m = result.summary["metrics"]
    assert result.decision.outcome == "promotable", result.reason
    assert m["ndcg@10"] > m["baseline_ndcg@10"] * 1.05, m  # the baseline cannot tell the items apart
    assert m["holdout_lists_with_positive"] > 20
    assert reg.tags(GBDT, result.decision.version)["lineage.eval_protocol"] == contract.EVAL_PROTOCOL


def test_the_artifact_scores_a_cheap_item_above_an_expensive_one_the_way_the_trainer_does(tmp_path, reg):
    result = _run(_settings(tmp_path, reg))
    art = json.loads(json.dumps(result.artifact))
    assert art["features"] == list(contract.RANKING_FEATURES) and art["generation"] == "gen-1"
    assert art["model_version"] == "gbdt-gen-1" and art["format"] == "agora-gbdt/1"
    base = [100, 5, 1, 1, 3, 4.0, 0.05, 0.0]
    cheap, dear = list(base), list(base)
    cheap[-1], dear[-1] = 20_000.0, 480_000.0
    assert gbdt.score_vector(art, cheap) > gbdt.score_vector(art, dear)


def test_a_promotable_candidate_is_recorded_as_recsys_gbdt_and_applied_only_after_the_publish(tmp_path, reg):
    result = _run(_settings(tmp_path, reg))
    v = result.decision.version
    assert registered_models(reg) == {GBDT}  # its own model: never the ALS one
    assert versions(reg, GBDT) == [v] and outcome(reg, GBDT, v) == "promotable"
    assert reg.aliases(GBDT) == {}  # the stage moves no alias
    apply(GBDT, v)  # what the pipeline does once the generation is published
    assert reg.aliases(GBDT) == {"champion": v} and outcome(reg, GBDT, v) == "promoted"


def test_the_model_records_its_lineage_rows_and_ctr_sources(tmp_path, reg):
    rows = tmp_path / "rows.parquet"
    result = _run(_settings(tmp_path, reg, gbdt_rows_path=str(rows)))
    blob = _recorded(reg, result.decision.version)
    ds = (next((tmp_path / "rank").glob("as_of=*.manifest.json"))).read_text()
    assert json.loads(ds)["file_sha256"] in blob  # the dataset it trained on
    assert fx.SNAPSHOT_AS_OF in blob  # and the feature snapshots in force
    s = result.summary
    assert s["rows"] == fx.LISTS * fx.SHOWN
    # every training row records its CTR source
    written = pd.read_parquet(rows)
    assert len(written) == s["rows"]
    assert set(written["ctr_source"]) <= {"debiased", "fallback"} and written["ctr_source"].notna().all()
    counts = written["ctr_source"].value_counts().to_dict()
    assert {k: counts.get(k, 0) for k in ("debiased", "fallback")} == s["ctr_source"]
    assert set(written["split"]) == {"train", "holdout"}


def test_both_ctr_sources_occur_when_some_items_are_rare(tmp_path, reg):
    s = _settings(tmp_path, reg, gbdt_rows_path=str(tmp_path / "rows.parquet"), gbdt_ctr_min_impressions=100)
    counts = _run(s).summary["ctr_source"]
    assert counts["debiased"] > 0 and counts["fallback"] > 0  # ~100 impressions per item: about half pass


def test_the_holdout_is_strictly_later_than_training(tmp_path, reg):
    h = _run(_settings(tmp_path, reg)).summary["holdout"]
    assert h["train_last_at"] < h["test_first_at"] == h["cutoff"]
    assert h["train_lists"] + h["test_lists"] == fx.LISTS and h["test_lists"] == fx.LISTS // 5


def test_a_candidate_below_its_champion_is_rejected_and_has_no_artifact(tmp_path, reg):
    old = seed_strong(reg, GBDT)
    result = _run(_settings(tmp_path, reg))
    v = newest(reg, GBDT)
    assert result.decision.outcome == "rejected" and result.artifact is None
    assert v != old[GBDT] and outcome(reg, GBDT, v) == "rejected"
    assert reg.tags(GBDT, v)["gate.reason"] == result.reason and "ndcg@10" in result.reason
    assert reg.aliases(GBDT) == {"champion": old[GBDT]}


def test_a_signalless_dataset_does_not_pass_a_strict_champion(tmp_path, reg):
    seed_strong(reg, GBDT)
    result = _run(_settings(tmp_path, reg, signal=False))
    assert result.decision.outcome == "rejected" and result.artifact is None


def test_the_stage_cannot_force_the_gate(tmp_path, reg, monkeypatch):
    # was: test_force_skips_the_gate. The removed PROMOTION_FORCE changes nothing (RAT-35).
    monkeypatch.setenv("PROMOTION_FORCE", "true")
    old = seed_strong(reg, GBDT)
    result = _run(_settings(tmp_path, reg))
    v = newest(reg, GBDT)
    assert result.decision.outcome == "rejected" and result.artifact is None
    assert not any("forced" in k for k in reg.tags(GBDT, v))
    assert reg.aliases(GBDT) == {"champion": old[GBDT]}


def test_a_second_run_on_the_same_data_is_gated_against_the_champion(tmp_path, reg):
    s = _settings(tmp_path, reg)
    first = _run(s, "gen-1")
    apply(GBDT, first.decision.version)
    second = _run(s, "gen-2")
    # equal metrics: below the +0.5 % improvement, so the champion is not churned
    assert second.decision.outcome == "rejected" and "ndcg@10" in second.reason
    assert reg.aliases(GBDT) == {"champion": first.decision.version}


def test_rows_before_every_snapshot_are_dropped_and_a_run_with_none_left_is_rejected(tmp_path, reg):
    s = _settings(tmp_path, reg)
    fx.write_popularity(tmp_path / "late", as_of="20261101T000000Z")  # a snapshot after every impression
    result = _run(Settings(**{**s.__dict__, "item_features_dir": str(tmp_path / "late")}))
    assert result.decision.outcome == "rejected" and result.artifact is None
    assert "no usable rows" in result.reason and f"{fx.LISTS * fx.SHOWN} dropped" in result.reason
    # a structural rejection is still recorded (register_rejected), with its reason
    v = newest(reg, GBDT)
    assert outcome(reg, GBDT, v) == "rejected" and "no usable rows" in reg.tags(GBDT, v)["gate.reason"]
    assert reg.aliases(GBDT) == {}


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


def test_the_training_columns_follow_the_contract(tmp_path, reg):
    result = _run(_settings(tmp_path, reg, gbdt_rows_path=str(tmp_path / "r.parquet")))
    assert result.decision.outcome == "promotable"
    written = pd.read_parquet(tmp_path / "r.parquet")
    assert list(contract.RANKING_FEATURES) == [c for c in written.columns if c in contract.RANKING_FEATURES]
    assert (written["item_attributes.price"] > 0).all() and training.CTR_COL == 6
