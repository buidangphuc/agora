"""Unit tests for Two-Tower candidate retrieval model and pipeline."""

from recsys.two_tower.item_tower import ItemTower
from recsys.two_tower.model import TwoTowerModel
from recsys.two_tower.pipeline import train_and_index_two_tower
from recsys.two_tower.user_tower import UserTower


def test_towers_projection_and_normalization():
    u_tower = UserTower(embedding_dim=16)
    i_tower = ItemTower(embedding_dim=16)

    u_vec = u_tower.project({
        "preferred_categories": ["electronics"],
        "lifetime_purchases": 5,
        "avg_order_value": 50.0,
        "activity_score": 0.8,
    })

    i_vec = i_tower.project({
        "category_id": "electronics",
        "price": 45.0,
        "historical_ctr": 0.05,
        "popularity_score": 0.7,
    })

    assert len(u_vec) == 16
    assert len(i_vec) == 16

    u_norm = sum(v * v for v in u_vec) ** 0.5
    i_norm = sum(v * v for v in i_vec) ** 0.5
    assert abs(u_norm - 1.0) < 1e-4
    assert abs(i_norm - 1.0) < 1e-4


def test_two_tower_retrieval_ranking():
    model = TwoTowerModel(embedding_dim=32)

    catalog = [
        {"listing_id": "item_elec_1", "category_id": "electronics", "price": 100.0, "popularity_score": 0.9},
        {"listing_id": "item_elec_2", "category_id": "electronics", "price": 50.0, "popularity_score": 0.5},
        {"listing_id": "item_fashion_1", "category_id": "fashion", "price": 30.0, "popularity_score": 0.8},
        {"listing_id": "item_books_1", "category_id": "books", "price": 15.0, "popularity_score": 0.4},
    ]

    model.index_items(catalog)

    tech_user = {
        "preferred_categories": ["electronics"],
        "lifetime_purchases": 10,
        "avg_order_value": 80.0,
        "activity_score": 0.9,
    }

    recs = model.recommend(tech_user, top_k=2)
    assert len(recs) == 2
    assert recs[0].startswith("item_elec")


def test_cold_start_item_retrieval():
    model = TwoTowerModel(embedding_dim=32)

    cold_item = {
        "listing_id": "cold_gadget_999",
        "category_id": "electronics",
        "price": 120.0,
        "historical_ctr": 0.0,
        "popularity_score": 0.0,
    }
    other_item = {
        "listing_id": "old_book_1",
        "category_id": "books",
        "price": 10.0,
        "historical_ctr": 0.02,
        "popularity_score": 0.3,
    }
    model.index_items([cold_item, other_item])

    user = {
        "preferred_categories": ["electronics"],
        "activity_score": 0.5,
    }

    recs = model.recommend(user, top_k=1)
    assert recs == ["cold_gadget_999"]


def test_two_tower_pipeline_indexing():
    catalog = [
        {"listing_id": "item-1", "category_id": "electronics", "price": 99.0},
        {"listing_id": "item-2", "category_id": "fashion", "price": 29.0},
    ]

    model, vectors = train_and_index_two_tower(catalog, embedding_dim=16)
    assert len(vectors) == 2
    assert "item-1" in vectors
    assert len(vectors["item-1"]) == 16
    assert isinstance(model, TwoTowerModel)
