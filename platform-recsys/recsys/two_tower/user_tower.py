"""User Tower neural feature projection for Two-Tower Retrieval."""

from __future__ import annotations

import math
from typing import Any


class UserTower:
    """Projects user features and categorical preferences into a dense semantic vector."""

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
        # Input dimension: one-hot/multi-hot categories + 3 continuous features (purchases, aov, activity)
        self.input_dim = self.num_cats + 3
        self.embedding_dim = embedding_dim

        # Deterministic projection matrix
        import random
        rng = random.Random(seed)
        self.weights = [
            [rng.uniform(-0.1, 0.1) for _ in range(self.embedding_dim)]
            for _ in range(self.input_dim)
        ]
        self.bias = [0.0] * self.embedding_dim

    def _extract_input_vector(self, user_features: dict[str, Any]) -> list[float]:
        vec = [0.0] * self.input_dim
        # Categorical preferences
        cats = user_features.get("preferred_categories", [])
        for cat in cats:
            if cat in self.cat_to_idx:
                vec[self.cat_to_idx[cat]] = 1.0

        # Continuous features (normalized)
        purchases = float(user_features.get("lifetime_purchases", 0))
        aov = float(user_features.get("avg_order_value", 0.0))
        activity = float(user_features.get("activity_score", 0.0))

        vec[self.num_cats] = math.log1p(max(0.0, purchases))
        vec[self.num_cats + 1] = math.log1p(max(0.0, aov)) / 10.0
        vec[self.num_cats + 2] = min(1.0, max(0.0, activity))
        return vec

    def project(self, user_features: dict[str, Any]) -> list[float]:
        """Maps user feature dict to normalized D-dimensional embedding vector."""
        x = self._extract_input_vector(user_features)
        out = [0.0] * self.embedding_dim
        for j in range(self.embedding_dim):
            val = self.bias[j]
            for i in range(self.input_dim):
                val += x[i] * self.weights[i][j]
            # ReLU activation
            out[j] = max(0.0, val)

        # L2 Normalize
        norm = math.sqrt(sum(v * v for v in out))
        if norm > 1e-9:
            out = [v / norm for v in out]
        return out
