"""The two-tower training step and the degenerate-vector guard (no Spark)."""

from __future__ import annotations

import random

import pytest

from recsys.two_tower.model import TwoTowerModel
from recsys.two_tower.pipeline import (
    DegenerateEmbeddingError,
    is_degenerate,
    train_and_index_two_tower,
)
from recsys.two_tower.train import train_towers

CATEGORIES = ["electronics", "fashion", "books"]


def _catalog():
    return [
        {
            "listing_id": f"{c}-{i}",
            "category_id": c,
            "price": 10.0 * (i + 1),
            "historical_ctr": 0.05 * i,
            "popularity_score": 0.1 * i,
        }
        for c in CATEGORIES
        for i in range(6)
    ]


def _users():
    return [
        {
            "user_key": f"u-{c}-{i}",
            "preferred_categories": [c],
            "lifetime_purchases": i,
            "activity_score": 0.5,
        }
        for c in CATEGORIES
        for i in range(5)
    ]


def _pairs(users):
    rng = random.Random(1)
    return [
        (u["user_key"], f"{u['preferred_categories'][0]}-{rng.randrange(6)}") for u in users for _ in range(6)
    ]


def _flat(model):
    return [w for t in (model.user_tower, model.item_tower) for row in t.weights for w in row] + [
        b for t in (model.user_tower, model.item_tower) for b in t.bias
    ]


def test_training_updates_the_weights_and_lowers_the_loss():
    users = _users()
    untrained, _ = train_and_index_two_tower(_catalog(), users, 16, pairs=_pairs(users), epochs=0)
    trained, vectors = train_and_index_two_tower(
        _catalog(), users, 16, pairs=_pairs(users), epochs=30, lr=0.5
    )

    assert _flat(trained) != _flat(untrained)  # a gradient step happened
    losses = trained.report.losses
    assert len(losses) == 30
    assert losses[-1] < 0.85 * losses[0]
    assert untrained.report.losses == []
    assert len(vectors) == 18


def test_a_small_step_never_increases_the_full_batch_loss():
    users = _users()
    model = TwoTowerModel(embedding_dim=16)
    catalog = {it["listing_id"]: it for it in _catalog()}
    profiles = {u["user_key"]: u for u in users}
    pairs = _pairs(users)
    losses = train_towers(model, pairs, profiles, catalog, epochs=8, lr=0.002, batch_size=len(pairs))
    assert all(b <= a + 1e-12 for a, b in zip(losses, losses[1:], strict=False)), losses


def test_the_trained_towers_retrieve_the_users_category():
    users = _users()
    model, _ = train_and_index_two_tower(_catalog(), users, 16, pairs=_pairs(users), epochs=60, lr=0.5)
    hits = sum(model.recommend(u, top_k=1)[0].startswith(u["preferred_categories"][0]) for u in users)
    assert hits == len(users)


def test_training_is_deterministic():
    users = _users()
    a, va = train_and_index_two_tower(_catalog(), users, 8, pairs=_pairs(users), epochs=3)
    b, vb = train_and_index_two_tower(_catalog(), users, 8, pairs=_pairs(users), epochs=3)
    assert va == vb and a.report.losses == b.report.losses


def test_pairs_for_unknown_items_are_dropped_and_a_user_without_features_trains():
    users = _users()
    pairs = _pairs(users) + [("u-electronics-0", "not-in-catalogue"), ("someone-without-features", "books-1")]
    model, _ = train_and_index_two_tower(_catalog(), users, 8, pairs=pairs, epochs=2)
    assert model.report.pairs == len(pairs) - 1


def test_nothing_is_trained_below_two_pairs():
    model, _ = train_and_index_two_tower(_catalog(), _users(), 8, pairs=[("u-books-0", "books-1")], epochs=3)
    assert model.report.losses == []


def test_items_with_different_categories_embed_differently_and_non_zero():
    """Scenario: Cold-start item receives a vector that ALS cannot produce (strengthened)."""
    catalog = [
        {"listing_id": "cold-phone", "category_id": "electronics", "price": 300.0},
        {"listing_id": "cold-shirt", "category_id": "fashion", "price": 20.0},
    ]
    _, vectors = train_and_index_two_tower(catalog, embedding_dim=16)
    assert not is_degenerate(vectors["cold-phone"]) and not is_degenerate(vectors["cold-shirt"])
    assert vectors["cold-phone"] != vectors["cold-shirt"]


def test_a_zero_vector_is_refused_not_returned():
    catalog = [
        {"listing_id": "has-features", "popularity_score": 0.8, "historical_ctr": 0.1},
        {"listing_id": "no-features"},  # all-zero tower input, zero bias: embeds to zero
    ]
    model, vectors = train_and_index_two_tower(catalog, embedding_dim=8)
    assert list(vectors) == ["has-features"]
    assert model.report.refused == ["no-features"]
    assert model.report.items == 1


def test_every_vector_degenerate_fails_the_stage():
    with pytest.raises(DegenerateEmbeddingError, match="embed to zero"):
        train_and_index_two_tower([{"listing_id": "a"}, {"listing_id": "b"}], embedding_dim=8)
    with pytest.raises(DegenerateEmbeddingError, match="empty"):
        train_and_index_two_tower([], embedding_dim=8)


def test_is_degenerate():
    assert is_degenerate([0.0, 0.0])
    assert is_degenerate([float("nan"), 1.0])
    assert is_degenerate([float("inf"), 0.0])
    assert not is_degenerate([0.0, 0.6])
