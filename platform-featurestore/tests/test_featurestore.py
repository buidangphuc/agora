"""Unit tests for online and offline feature store functionality."""

from featurestore.definitions import ItemFeatures, UserFeatures
from featurestore.offline import OfflineFeatureStore
from featurestore.online import OnlineFeatureStore


def test_online_store_user_and_item():
    store = OnlineFeatureStore()

    u = UserFeatures(
        user_id="user_123",
        lifetime_purchases=5,
        preferred_categories=["electronics", "fashion"],
        avg_order_value=45.5,
        activity_score=0.85,
    )
    store.set_user_features(u)

    fetched_u = store.get_user_features("user_123")
    assert fetched_u is not None
    assert fetched_u.user_id == "user_123"
    assert fetched_u.lifetime_purchases == 5
    assert fetched_u.preferred_categories == ["electronics", "fashion"]
    assert fetched_u.avg_order_value == 45.5

    i = ItemFeatures(
        listing_id="item_456",
        category_id="cat_electronics",
        price=99.9,
        historical_ctr=0.042,
        popularity_score=0.9,
    )
    store.set_item_features(i)

    fetched_i = store.get_item_features("item_456")
    assert fetched_i is not None
    assert fetched_i.listing_id == "item_456"
    assert fetched_i.price == 99.9
    assert fetched_i.historical_ctr == 0.042

    # Batch get
    batch = store.get_item_features_batch(["item_456", "non_existent"])
    assert "item_456" in batch
    assert "non_existent" not in batch


def test_offline_store_dataset_generation():
    store = OfflineFeatureStore()

    users = [
        UserFeatures(user_id="u1", lifetime_purchases=2, avg_order_value=20.0),
        UserFeatures(user_id="u2", lifetime_purchases=10, avg_order_value=150.0),
    ]
    items = [
        ItemFeatures(listing_id="i1", category_id="cat_1", price=10.0, historical_ctr=0.05),
        ItemFeatures(listing_id="i2", category_id="cat_2", price=50.0, historical_ctr=0.02),
    ]

    store.save_user_features(users)
    store.save_item_features(items)

    interactions = [
        {"user_id": "u1", "listing_id": "i1", "label": 1},
        {"user_id": "u1", "listing_id": "i2", "label": 0},
        {"user_id": "u2", "listing_id": "i2", "label": 1},
    ]

    dataset = store.build_training_dataset(interactions)
    assert len(dataset) == 3
    assert dataset[0]["user_id"] == "u1"
    assert dataset[0]["listing_id"] == "i1"
    assert dataset[0]["label"] == 1
    assert dataset[0]["u_lifetime_purchases"] == 2
    assert dataset[0]["i_price"] == 10.0
    assert dataset[1]["label"] == 0
    assert dataset[2]["u_avg_order_value"] == 150.0
