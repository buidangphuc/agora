"""Model-version provenance.

Every artifact (Qdrant point payload + the Redis version key) is stamped with the
same `model_version`. It is the value the serving RPC echoes back
(add-recommendation-contract) and the debugging anchor for offline/online skew.
An explicit MODEL_VERSION wins; otherwise it is derived from the run clock so a
re-run of the same window is reproducible within the minute.
"""

from __future__ import annotations

from datetime import datetime, timezone

from .config import Settings


def resolve_model_version(settings: Settings, now: datetime | None = None) -> str:
    if settings.model_version.strip():
        return settings.model_version.strip()
    ts = (now or datetime.now(timezone.utc)).strftime("%Y%m%dT%H%M%SZ")
    return f"als-{ts}"
