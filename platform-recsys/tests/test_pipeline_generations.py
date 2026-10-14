"""The pipeline publishes generations and applies the structural gate (needs PySpark: runs in the image).

Ported to the MLflow registry: the champion is the ``recsys-als@champion`` alias; every run that is meant to
promote starts from a weak seeded champion (the shared gate would reject a repeat of an equal model).
"""

from __future__ import annotations

import pytest

pytest.importorskip("pyspark")

from recsys.pipeline import run  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    als_rows,
    gens,
    newest,
    one_size_rows,
    outcome,
    pointers,
    requires_spark,
    seed_weak,
    settings,
)

pytestmark = requires_spark


def test_successive_promotions_keep_two_generations_and_move_the_pointers(tmp_path, reg):
    write_dataset(tmp_path, als_rows())
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()

    summaries = []
    for v in ("g1", "g2", "g3"):
        seed_weak(reg, ALS)
        summaries.append(run(settings(tmp_path, v, reg), redis_client=fake_r, qdrant_client=fake_q))

    assert [s["decision"] for s in summaries] == ["promoted"] * 3
    assert summaries[1]["serving"] == "g2" and summaries[1]["previous"] == "g1"
    assert fake_r.store["recs:v1:serving"] == "g3"
    assert fake_r.store["recs:v1:previous"] == "g2"
    assert fake_r.store["recs:v1:model_version"] == "g3"
    assert gens(fake_r) == {"g2", "g3"}
    assert fake_q.aliases["item_als_vectors"] == "item_als_vectors__g3"
    assert set(fake_q.collections) == {
        "item_als_vectors__g2",
        "item_als_vectors__g3",
        "user_als_vectors__g2",
        "user_als_vectors__g3",
    }
    # the registry follows the publishes: the last candidate is champion, the seeded holder is `previous`
    champion = reg.aliases(ALS)["champion"]
    assert champion == newest(reg, ALS) and outcome(reg, ALS, champion) == "promoted"


def test_one_size_fits_all_candidate_is_rejected_and_serving_is_unchanged(tmp_path, reg):
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()
    good = tmp_path / "good"
    write_dataset(good, als_rows())
    run(settings(good, "g1", reg), redis_client=fake_r, qdrant_client=fake_q)
    g1 = reg.aliases(ALS)["champion"]
    before = (dict(fake_r.store), dict(fake_q.aliases), set(fake_q.collections))

    bad = tmp_path / "bad"
    write_dataset(bad, one_size_rows())
    cfg = settings(bad, "g2-degenerate", reg, top_n=5)
    summary = run(cfg, redis_client=fake_r, qdrant_client=fake_q)

    assert summary["decision"] == "rejected" and summary["gate"] == "structural"
    assert "list overlap" in summary["reason"] or "item coverage" in summary["reason"]
    rejected = newest(reg, ALS)
    assert rejected != g1
    assert outcome(reg, ALS, rejected) == "rejected"
    assert reg.tags(ALS, rejected)["gate.reason"] == summary["reason"]
    assert reg.aliases(ALS) == {"champion": g1}
    assert (dict(fake_r.store), dict(fake_q.aliases), set(fake_q.collections)) == before


class _AliasFailingQdrant(FakeQdrantClient):
    def update_collection_aliases(self, change_aliases_operations):
        raise RuntimeError("qdrant unreachable")


def test_failed_publish_keeps_the_previous_champion_and_pointers(tmp_path, reg):
    write_dataset(tmp_path, als_rows())
    fake_r, fake_q = FakeRedis(), FakeQdrantClient()
    run(settings(tmp_path, "g1", reg), redis_client=fake_r, qdrant_client=fake_q)
    g1 = reg.aliases(ALS)["champion"]
    seed_weak(reg, ALS)  # so that g2 is a promotable candidate
    champion_before = reg.aliases(ALS)["champion"]
    pointers_before = pointers(fake_r)
    aliases = dict(fake_q.aliases)

    broken = _AliasFailingQdrant()
    broken.collections, broken.aliases, broken.dims = fake_q.collections, fake_q.aliases, fake_q.dims
    with pytest.raises(RuntimeError, match="qdrant unreachable"):
        run(settings(tmp_path, "g2", reg), redis_client=fake_r, qdrant_client=broken)

    assert reg.aliases(ALS)["champion"] == champion_before != newest(reg, ALS)
    assert outcome(reg, ALS, g1) == "promoted"
    failed = newest(reg, ALS)
    assert outcome(reg, ALS, failed) == "rejected"
    assert reg.tags(ALS, failed)["gate.reason"] == "publish failed: RuntimeError"
    assert pointers(fake_r) == pointers_before
    assert fake_q.aliases == aliases
