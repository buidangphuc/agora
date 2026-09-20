"""Feature schemas and data representations for users and items."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
import json
from typing import Any


@dataclass
class UserFeatures:
    user_id: str
    lifetime_purchases: int = 0
    preferred_categories: list[str] = field(default_factory=list)
    avg_order_value: float = 0.0
    activity_score: float = 0.0
    last_active_days: float = 0.0

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> UserFeatures:
        return cls(**data)

    @classmethod
    def from_json(cls, json_str: str) -> UserFeatures:
        return cls.from_dict(json.loads(json_str))


@dataclass
class ItemFeatures:
    listing_id: str
    category_id: str = ""
    seller_id: str = ""
    price: float = 0.0
    historical_ctr: float = 0.0
    conversion_rate: float = 0.0
    popularity_score: float = 0.0

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ItemFeatures:
        return cls(**data)

    @classmethod
    def from_json(cls, json_str: str) -> ItemFeatures:
        return cls.from_dict(json.loads(json_str))
