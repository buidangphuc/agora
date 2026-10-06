"""Pins the Qdrant point-id scheme team-ai reads by (serve-trained-recs-locally).

team-ai maps a listing id to uuid5(namespace, listing_id) to look up its vector; a
change here without the matching team-ai change silently breaks similar items.
"""

from recsys.load.qdrant import point_id

# Same pin as team-ai tests/unit/modules/recommend/test_qdrant_backend.py.
CONSUMER_PIN = "25a4b2d5-6531-5357-90f2-06e92d1e1191"


def test_point_id_matches_the_consumer_pin():
    assert point_id("listing-1") == CONSUMER_PIN
