"""Candidate attributes (``seller_id``, ``category_id``, ``visible``) from ``item_attributes@v2``.

The snapshot is read from ``ITEM_ATTRIBUTES_V2_PATH`` (an explicit file, wins) or the latest
``as_of=<YYYYMMDDTHHMMSSZ>.parquet`` under ``ITEM_ATTRIBUTES_V2_DIR``. ``visible`` is
``status == LISTING_STATUS_PUBLISHED``; an unset or unknown status leaves ``visible`` absent. A value that is
empty or longer than 128 characters is treated as unknown and left out. ``in_stock`` is never carried.

A snapshot older than 24 hours is used anyway, with one warning naming its ``as_of``. A missing or
corrupt snapshot yields ``None``: a training run refuses (exit 2) instead of publishing the old shape.
Logs carry counts and never an id.
"""

from __future__ import annotations

import logging
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path

log = logging.getLogger("recsys.attributes")

ATTRIBUTE_KEYS = ("seller_id", "category_id", "visible")
MAX_VALUE_LENGTH = 128
STALE_AFTER = timedelta(hours=24)
PUBLISHED = "LISTING_STATUS_PUBLISHED"
KNOWN_STATUSES = frozenset(
    {
        PUBLISHED,
        "LISTING_STATUS_DRAFT",
        "LISTING_STATUS_REJECTED",
        "LISTING_STATUS_PENDING_REVIEW",
        "LISTING_STATUS_BANNED",
        "LISTING_STATUS_HELD",
        "LISTING_STATUS_DELETED",
    }
)

_STAMP = re.compile(r"^as_of=(\d{8}T\d{6}Z)\.parquet$")
_STAMP_FORMAT = "%Y%m%dT%H%M%SZ"


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


class AttributeSnapshot(dict):
    """``{listing_id: {seller_id?, category_id?, visible?}}`` plus the snapshot's ``as_of``."""

    as_of: datetime | None = None


def _stamp_of(path: Path) -> datetime | None:
    m = _STAMP.match(path.name)
    if not m:
        return None
    return datetime.strptime(m.group(1), _STAMP_FORMAT).replace(tzinfo=timezone.utc)


def _resolve(settings) -> Path | None:
    if settings.item_attributes_v2_path:
        path = Path(settings.item_attributes_v2_path)
        return path if path.is_file() else None
    directory = Path(settings.item_attributes_v2_dir)
    if not directory.is_dir():
        return None
    stamped = [(stamp, p) for p in directory.iterdir() if (stamp := _stamp_of(p)) is not None]
    return max(stamped, key=lambda item: item[0])[1] if stamped else None


def _text(value) -> str | None:
    if value is None:
        return None
    text = value if isinstance(value, str) else str(value)
    if not text or len(text) > MAX_VALUE_LENGTH:
        return None
    return text


def _row_attributes(seller, category, status) -> dict:
    attrs: dict = {}
    seller_id = _text(seller)
    if seller_id is not None:
        attrs["seller_id"] = seller_id
    category_id = _text(category)
    if category_id is not None:
        attrs["category_id"] = category_id
    if status in KNOWN_STATUSES:
        attrs["visible"] = status == PUBLISHED
    return attrs


def _read(path: Path) -> AttributeSnapshot:
    import pyarrow.parquet as pq  # noqa: PLC0415

    names = pq.ParquetFile(path).schema_arrow.names
    if "listing_id" not in names:
        raise ValueError("no listing_id column")
    wanted = [c for c in ("listing_id", "seller_id", "category_id", "status") if c in names]
    table = pq.read_table(path, columns=wanted).to_pydict()
    n = len(table["listing_id"])
    empty = [None] * n
    snapshot = AttributeSnapshot()
    for lid, seller, category, status in zip(
        table["listing_id"],
        table.get("seller_id", empty),
        table.get("category_id", empty),
        table.get("status", empty),
        strict=True,
    ):
        if not lid:
            continue
        snapshot[str(lid)] = _row_attributes(seller, category, status)
    return snapshot


def load_attributes(settings) -> AttributeSnapshot | None:
    """The attribute map of the latest snapshot, or ``None`` (logged ``attributes=missing``)."""
    path = _resolve(settings)
    if path is None:
        log.warning("attributes=missing: no item_attributes v2 snapshot")
        return None
    try:
        snapshot = _read(path)
    except Exception as exc:  # noqa: BLE001
        log.warning("attributes=missing: item_attributes v2 snapshot unreadable (%s)", type(exc).__name__)
        return None
    snapshot.as_of = _stamp_of(path)
    if snapshot.as_of is not None and utcnow() - snapshot.as_of > STALE_AFTER:
        log.warning(
            "item_attributes v2 snapshot is stale: as_of=%s is more than 24h old",
            snapshot.as_of.strftime("%Y-%m-%dT%H:%M:%SZ"),
        )
    log.info("attributes loaded: %d listings", len(snapshot))
    return snapshot
