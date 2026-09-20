"""Offline Feature Store implementation for point-in-time queries and batch dataset generation."""

from __future__ import annotations

from typing import Any

from featurestore.definitions import ItemFeatures, UserFeatures


class OfflineFeatureStore:
    """Historical feature repository for model training and dataset generation."""

    def __init__(self) -> None:
        self._users: dict[str, UserFeatures] = {}
        self._items: dict[str, ItemFeatures] = {}

    def save_user_features(self, records: list[UserFeatures]) -> None:
        for rec in records:
            self._users[rec.user_id] = rec

    def save_item_features(self, records: list[ItemFeatures]) -> None:
        for rec in records:
            self._items[rec.listing_id] = rec

    def get_user_features(self, user_ids: list[str]) -> list[UserFeatures]:
        return [self._users[uid] for uid in user_ids if uid in self._users]

    def get_item_features(self, listing_ids: list[str]) -> list[ItemFeatures]:
        return [self._items[lid] for lid in listing_ids if lid in self._items]

    def build_training_dataset(
        self,
        interactions: list[dict[str, Any]],
    ) -> list[dict[str, Any]]:
        """Joins interaction tuples (user_id, listing_id, label) with user and item features."""
        dataset: list[dict[str, Any]] = []
        for inter in interactions:
            uid = inter.get("user_id", "")
            lid = inter.get("listing_id", "")
            label = inter.get("label", 0)

            u_feat = self._users.get(uid)
            i_feat = self._items.get(lid)

            u_dict = u_feat.to_dict() if u_feat else {}
            i_dict = i_feat.to_dict() if i_feat else {}

            row = {
                "user_id": uid,
                "listing_id": lid,
                "label": label,
                **{f"u_{k}": v for k, v in u_dict.items() if k != "user_id"},
                **{f"i_{k}": v for k, v in i_dict.items() if k != "listing_id"},
            }
            dataset.append(row)
        return dataset
