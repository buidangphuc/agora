"""Pure (PySpark-free) interaction helpers.

Kept import-light on purpose: the event->weight lookup and the principal-vs-
anonymous user-key rule are the parts most worth unit-testing, and they must run
on a laptop with no Spark installed. The Spark DataFrame pipeline in
``interactions.py`` reuses these same rules (as a UDF / broadcast map) so the
logic is defined once.
"""

from __future__ import annotations

import math

# Weight applied to any event_type not present in the configured map. A tiny
# positive floor keeps an unknown-but-real interaction as weak positive signal
# rather than dropping it or (worse) treating it as zero confidence.
UNKNOWN_EVENT_WEIGHT = 0.5


def choose_user_key(principal_id: str | None, anonymous_id: str | None) -> str | None:
    """user = principal_id when authenticated, else anonymous_id.

    Mirrors how a RecommendRequest identifies a caller (user xor anonymous).
    Returns None when neither id is usable, so the row can be dropped.
    """
    if principal_id and principal_id.strip():
        return principal_id.strip()
    if anonymous_id and anonymous_id.strip():
        return anonymous_id.strip()
    return None


def event_weight(event_type: str | None, weights: dict[str, float]) -> float:
    """Confidence weight for an event_type (case-insensitive), with a floor."""
    if not event_type:
        return UNKNOWN_EVENT_WEIGHT
    return float(weights.get(event_type.strip().lower(), UNKNOWN_EVENT_WEIGHT))


def recency_multiplier(age_days: float, half_life_days: float) -> float:
    """Exponential recency decay in [0,1]; half_life_days<=0 disables decay (1.0)."""
    if half_life_days <= 0:
        return 1.0
    if age_days <= 0:
        return 1.0
    return math.pow(0.5, age_days / half_life_days)


def is_valid_listing(listing_id: str | None) -> bool:
    return bool(listing_id and listing_id.strip())
