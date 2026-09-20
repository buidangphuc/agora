"""Online-offline feature parity validation to detect training-serving skew."""

from __future__ import annotations

from dataclasses import dataclass, field
import math
from typing import Any

from featurestore.offline import OfflineFeatureStore
from featurestore.online import OnlineFeatureStore


@dataclass
class ParityReport:
    is_consistent: bool
    total_checked: int
    mismatches: list[str] = field(default_factory=list)


def _values_match(v1: Any, v2: Any, tol: float = 1e-4) -> bool:
    if v1 is None and v2 is None:
        return True
    if v1 is None or v2 is None:
        return False
    if isinstance(v1, (int, float)) and isinstance(v2, (int, float)):
        return math.isclose(float(v1), float(v2), rel_tol=tol, abs_tol=tol)
    return bool(v1 == v2)


def validate_parity(
    online_store: OnlineFeatureStore,
    offline_store: OfflineFeatureStore,
    user_ids: list[str],
    item_ids: list[str],
    tolerance: float = 1e-4,
) -> ParityReport:
    """Validates parity across online and offline stores for given entities."""
    mismatches: list[str] = []
    total_checked = 0

    for uid in user_ids:
        total_checked += 1
        on_u = online_store.get_user_features(uid)
        off_u_list = offline_store.get_user_features([uid])
        off_u = off_u_list[0] if off_u_list else None

        if on_u is None and off_u is None:
            continue
        if on_u is None or off_u is None:
            mismatches.append(f"User {uid}: online={on_u is not None}, offline={off_u is not None}")
            continue

        for k, v_on in on_u.to_dict().items():
            v_off = getattr(off_u, k, None)
            if not _values_match(v_on, v_off, tolerance):
                mismatches.append(f"User {uid}.{k}: online={v_on} != offline={v_off}")

    for lid in item_ids:
        total_checked += 1
        on_i = online_store.get_item_features(lid)
        off_i_list = offline_store.get_item_features([lid])
        off_i = off_i_list[0] if off_i_list else None

        if on_i is None and off_i is None:
            continue
        if on_i is None or off_i is None:
            mismatches.append(f"Item {lid}: online={on_i is not None}, offline={off_i is not None}")
            continue

        for k, v_on in on_i.to_dict().items():
            v_off = getattr(off_i, k, None)
            if not _values_match(v_on, v_off, tolerance):
                mismatches.append(f"Item {lid}.{k}: online={v_on} != offline={v_off}")

    return ParityReport(
        is_consistent=len(mismatches) == 0,
        total_checked=total_checked,
        mismatches=mismatches,
    )
