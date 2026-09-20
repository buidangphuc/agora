"""Two-Tower Retrieval Model combining UserTower and ItemTower for ANN candidate retrieval."""

from __future__ import annotations

from typing import Any

from recsys.two_tower.item_tower import ItemTower
from recsys.two_tower.user_tower import UserTower


class TwoTowerModel:
    """End-to-end Two-Tower neural candidate retrieval system."""

    def __init__(
        self,
        embedding_dim: int = 32,
        category_vocab: list[str] | None = None,
        seed: int = 42,
    ) -> None:
        self.embedding_dim = embedding_dim
        self.user_tower = UserTower(
            category_vocab=category_vocab,
            embedding_dim=embedding_dim,
            seed=seed,
        )
        self.item_tower = ItemTower(
            category_vocab=category_vocab,
            embedding_dim=embedding_dim,
            seed=seed,
        )
        # In-memory candidate index: listing_id -> embedding_vector
        self._item_vectors: dict[str, list[float]] = {}
        self._item_metadata: dict[str, dict[str, Any]] = {}

    def index_item(self, item: dict[str, Any]) -> None:
        lid = str(item.get("listing_id") or item.get("id", ""))
        if not lid:
            return
        vec = self.item_tower.project(item)
        self._item_vectors[lid] = vec
        self._item_metadata[lid] = item

    def index_items(self, items: list[dict[str, Any]]) -> None:
        for it in items:
            self.index_item(it)

    def score(self, user_features: dict[str, Any], item_features: dict[str, Any]) -> float:
        """Calculates inner product similarity score between user and item feature representations."""
        u_vec = self.user_tower.project(user_features)
        i_vec = self.item_tower.project(item_features)
        return sum(u * v for u, v in zip(u_vec, i_vec))

    def retrieve(
        self,
        user_features: dict[str, Any],
        top_k: int = 10,
        exclude_item_ids: set[str] | None = None,
    ) -> list[tuple[str, float]]:
        """Retrieves top-K candidates from item index by cosine similarity."""
        if not self._item_vectors:
            return []

        u_vec = self.user_tower.project(user_features)
        excludes = exclude_item_ids or set()

        scores: list[tuple[str, float]] = []
        for lid, i_vec in self._item_vectors.items():
            if lid in excludes:
                continue
            sim = sum(u * v for u, v in zip(u_vec, i_vec))
            scores.append((lid, sim))

        scores.sort(key=lambda x: x[1], reverse=True)
        return scores[:top_k]

    def recommend(
        self,
        user_features: dict[str, Any],
        top_k: int = 10,
        exclude_item_ids: set[str] | None = None,
    ) -> list[str]:
        """Convenience method returning ordered list of candidate item IDs."""
        ranked = self.retrieve(user_features, top_k=top_k, exclude_item_ids=exclude_item_ids)
        return [lid for lid, _ in ranked]
