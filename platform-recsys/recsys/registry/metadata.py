"""Model metadata representation."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from typing import Any


@dataclass
class ModelMetadata:
    model_name: str
    model_version: str
    model_type: str  # "als", "two_tower", "gbdt"
    status: str = "candidate"  # "candidate", "champion", "rejected", "archived"
    metrics: dict[str, float] = field(default_factory=dict)
    parameters: dict[str, Any] = field(default_factory=dict)
    git_commit: str = ""
    created_at: str = field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat()
    )
    artifact_paths: dict[str, str] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ModelMetadata:
        return cls(**data)

    @classmethod
    def from_json(cls, json_str: str) -> ModelMetadata:
        return cls.from_dict(json.loads(json_str))
