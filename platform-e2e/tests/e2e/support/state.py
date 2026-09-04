"""Per-scenario shared state (mirrors bds `scenario-state.ts`, kept intentionally small).

Steps read/write cross-step data here via `world.state`. Add fields as scenarios
need them rather than starting from a ~90-field god-object.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from src.models import Listing, User


@dataclass
class ScenarioState:
    current_user: User | None = None
    seeded_seller: User | None = None
    listing: Listing | None = None
    search_term: str | None = None
    extra: dict[str, Any] = field(default_factory=dict)
