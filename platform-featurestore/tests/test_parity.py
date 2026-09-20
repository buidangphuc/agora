"""Unit tests for online-offline feature parity validation."""

from featurestore.definitions import ItemFeatures, UserFeatures
from featurestore.offline import OfflineFeatureStore
from featurestore.online import OnlineFeatureStore
from featurestore.parity import validate_parity


def test_parity_matching():
    online = OnlineFeatureStore()
    offline = OfflineFeatureStore()

    u = UserFeatures(user_id="u1", lifetime_purchases=3, avg_order_value=12.5)
    i = ItemFeatures(listing_id="i1", price=49.0, historical_ctr=0.03)

    online.set_user_features(u)
    online.set_item_features(i)

    offline.save_user_features([u])
    offline.save_item_features([i])

    report = validate_parity(
        online_store=online,
        offline_store=offline,
        user_ids=["u1"],
        item_ids=["i1"],
    )

    assert report.is_consistent is True
    assert len(report.mismatches) == 0
    assert report.total_checked == 2


def test_parity_skew_detection():
    online = OnlineFeatureStore()
    offline = OfflineFeatureStore()

    u_online = UserFeatures(user_id="u1", lifetime_purchases=3, avg_order_value=12.5)
    u_offline = UserFeatures(user_id="u1", lifetime_purchases=10, avg_order_value=12.5)

    online.set_user_features(u_online)
    offline.save_user_features([u_offline])

    report = validate_parity(
        online_store=online,
        offline_store=offline,
        user_ids=["u1"],
        item_ids=[],
    )

    assert report.is_consistent is False
    assert len(report.mismatches) == 1
    assert "User u1.lifetime_purchases: online=3 != offline=10" in report.mismatches[0]
