"""Online store: versioned Redis keys plus freshness meta (design D4)."""

from __future__ import annotations

import json


def row_key(view: str, version: int, entity_id: str) -> str:
    return f"fs:{view}:v{version}:{entity_id}"


def current_key(view: str) -> str:
    return f"fs:{view}:current"


def meta_key(view: str) -> str:
    return f"fs:{view}:meta"


def write_view(r, view: str, version: int, rows: list[dict], ttl: int, meta: dict, chunk: int = 1000) -> None:
    """Entities first, then the `current` pointer and `meta`, so readers never see a half-written version."""
    for i in range(0, len(rows), chunk):
        pipe = r.pipeline()
        for row in rows[i : i + chunk]:
            feats = {k: v for k, v in row.items() if k != "entity_id"}
            pipe.set(row_key(view, version, row["entity_id"]), json.dumps(feats), ex=ttl)
        pipe.execute()
    pipe = r.pipeline()
    pipe.set(current_key(view), str(version))
    pipe.set(meta_key(view), json.dumps(meta))
    pipe.execute()


def reconcile(r, view: str, version: int, keep: set[str]) -> None:
    """Delete this version's entity keys that are not in the freshly computed set (erased or stale users)."""
    prefix = f"fs:{view}:v{version}:"
    stale = [k for k in r.scan_iter(prefix + "*") if k[len(prefix) :] not in keep]
    for i in range(0, len(stale), 1000):
        r.delete(*stale[i : i + 1000])


def read_row(r, view: str, version: int, entity_id: str) -> dict | None:
    raw = r.get(row_key(view, version, entity_id))
    return None if raw is None else json.loads(raw)
