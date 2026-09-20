"""Online Feature Store adapter backed by Redis or In-Memory fallback."""

from __future__ import annotations

from typing import Any

from featurestore.definitions import ItemFeatures, UserFeatures


class OnlineFeatureStore:
    """Provides low-latency read/write access to online entity features."""

    def __init__(self, redis_client: Any = None, prefix: str = "fs:"):
        self.redis = redis_client
        self.prefix = prefix
        self._in_memory: dict[str, str] = {}

    def _u_key(self, user_id: str) -> str:
        return f"{self.prefix}u:{user_id}"

    def _i_key(self, listing_id: str) -> str:
        return f"{self.prefix}i:{listing_id}"

    def set_user_features(self, features: UserFeatures, ttl_seconds: int = 86400) -> None:
        key = self._u_key(features.user_id)
        payload = features.to_json()
        if self.redis:
            self.redis.setex(key, ttl_seconds, payload)
        else:
            self._in_memory[key] = payload

    def get_user_features(self, user_id: str) -> UserFeatures | None:
        key = self._u_key(user_id)
        if self.redis:
            val = self.redis.get(key)
            if val is None:
                return None
            if isinstance(val, bytes):
                val = val.decode("utf-8")
            return UserFeatures.from_json(val)
        else:
            val = self._in_memory.get(key)
            return UserFeatures.from_json(val) if val else None

    def set_item_features(self, features: ItemFeatures, ttl_seconds: int = 86400) -> None:
        key = self._i_key(features.listing_id)
        payload = features.to_json()
        if self.redis:
            self.redis.setex(key, ttl_seconds, payload)
        else:
            self._in_memory[key] = payload

    def get_item_features(self, listing_id: str) -> ItemFeatures | None:
        key = self._i_key(listing_id)
        if self.redis:
            val = self.redis.get(key)
            if val is None:
                return None
            if isinstance(val, bytes):
                val = val.decode("utf-8")
            return ItemFeatures.from_json(val)
        else:
            val = self._in_memory.get(key)
            return ItemFeatures.from_json(val) if val else None

    def get_item_features_batch(self, listing_ids: list[str]) -> dict[str, ItemFeatures]:
        res: dict[str, ItemFeatures] = {}
        if not listing_ids:
            return res
        if self.redis:
            keys = [self._i_key(lid) for lid in listing_ids]
            vals = self.redis.mget(keys)
            for lid, val in zip(listing_ids, vals):
                if val is not None:
                    if isinstance(val, bytes):
                        val = val.decode("utf-8")
                    res[lid] = ItemFeatures.from_json(val)
        else:
            for lid in listing_ids:
                feat = self.get_item_features(lid)
                if feat is not None:
                    res[lid] = feat
        return res
