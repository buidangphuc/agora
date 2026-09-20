"""Item Tower neural feature projection for Two-Tower Retrieval."""

from __future__ import annotations

import math
from typing import Any


class ItemTower:
    """Projects item attributes into a dense semantic vector in the shared latent space."""

    def __init__(
        self,
        category_vocab: list[str] | None = None,
        embedding_dim: int = 32,
        seed: int = 42,
    ) -> None:
        self.category_vocab = category_vocab or [
            "electronics",
            "fashion",
            "home",
            "beauty",
            "sports",
            "books",
            "toys",
            "automotive",
        ]
        self.cat_to_idx = {cat: idx for idx, cat in enumerate(self.category_vocab)}
        self.num_cats = len(self.category_vocab)
        # Input dimension: category one-hot + 3 continuous (price, ctr, popularity)
        self.input_dim = self.num_cats + 3
        self.embedding_dim = embedding_dim

        import random
        rng = random.Random(seed + 100)
        self.weights = [
            [rng.uniform(-0.1, 0.1) for _ in range(self.embedding_dim)]
            for _ in range(self.input_dim)
        ]
        self.bias = [0.0] * self.embedding_dim

    def _extract_input_vector(self, item_features: dict[str, Any]) -> list[float]:
        vec = [0.0] * self.input_dim
        cat = item_features.get("category_id", "")
        if cat in self.cat_to_idx:
            vec[self.cat_to_idx[cat]] = 1.0

        price = float(item_features.get("price", 0.0))
        ctr = float(item_features.get("historical_ctr", 0.0))
        popularity = float(item_features.get("popularity_score", 0.0))

        vec[self.num_cats] = math.log1p(max(0.0, price)) / 10.0
        vec[self.num_cats + 1] = min(1.0, max(0.0, ctr))
        vec[self.num_cats + 2] = min(1.0, max(0.0, popularity))
        return vec

    def project(self, item_features: dict[str, Any]) -> list[float]:
        """Maps item feature dict to normalized D-dimensional embedding vector."""
        x = self._extract_input_vector(item_features)
        out = [0.0] * self.embedding_dim
        for j in range(self.embedding_dim):
            val = self.bias[j]
            for i in range(self.input_dim):
                val += x[i] * self.weights[i][j]
            out[j] = max(0.0, val)

        norm = math.sqrt(sum(v * v for v in out))
        if norm > 1e-9:
            out = [v / norm for v in out]
        return out
