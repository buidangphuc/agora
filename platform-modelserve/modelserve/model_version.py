"""Model versioning and cache key calculation utilities."""

from __future__ import annotations

import hashlib


def make_embedding_cache_key(model_version: str, text: str) -> str:
    """Generate deterministic Redis cache key for text embedding.

    Key format: modelserve:embed:{model_version}:{sha256(text)}
    """
    text_hash = hashlib.sha256(text.encode("utf-8")).hexdigest()
    return f"modelserve:embed:{model_version}:{text_hash}"
