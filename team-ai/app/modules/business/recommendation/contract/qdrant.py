"""Qdrant naming written by platform-recsys (``recsys/load/qdrant.py``)."""

import uuid

# Point ids in the trained collections are uuid5(namespace, listing_id); the listing id
# itself is in the payload. MUST equal platform-recsys ``recsys/load/qdrant.py`` ``_NS``.
POINT_ID_NAMESPACE = uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00")


def point_id(listing_id: str) -> str:
    """The Qdrant point id platform-recsys stores for ``listing_id``."""
    return str(uuid.uuid5(POINT_ID_NAMESPACE, listing_id))


def generation_collection(base: str, generation: str | None) -> str:
    """``<base>__<generation>``; the bare ``base`` (the alias) when no generation is served."""
    return f"{base}__{generation}" if generation else base
