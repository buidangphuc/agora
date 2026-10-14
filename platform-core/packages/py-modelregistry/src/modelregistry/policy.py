"""Policy file loading, validation and resolution (arg, env MODELREGISTRY_POLICIES, packaged)."""
from __future__ import annotations

import math
import os
from dataclasses import dataclass, field
from importlib import resources
from pathlib import Path
from typing import Any

import yaml

from .errors import PolicyError

ENV_POLICIES = "MODELREGISTRY_POLICIES"
_KEYS = {"primary", "min_relative_improvement", "guards", "floors"}


@dataclass(frozen=True)
class Policy:
    primary: str
    min_relative_improvement: float = 0.0
    guards: dict[str, float] = field(default_factory=dict)
    floors: dict[str, float] = field(default_factory=dict)


def _is_num(v: Any) -> bool:
    return isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v)


def _read(policies_path: str | os.PathLike[str] | None) -> Any:
    path = policies_path or os.environ.get(ENV_POLICIES)
    try:
        if path:
            text = Path(path).read_text(encoding="utf-8")
        else:
            text = resources.files("modelregistry").joinpath("policies.yaml").read_text("utf-8")
        return yaml.safe_load(text) or {}
    except (OSError, yaml.YAMLError) as e:
        raise PolicyError(f"cannot load policies file {path or '<packaged>'}: {e}") from e


def _parse(model: str, entry: Any) -> Policy:
    def bad(msg: str) -> PolicyError:
        return PolicyError(f"policy for {model!r}: {msg}")

    if not isinstance(entry, dict):
        raise bad("entry must be a mapping")
    for key in entry:
        if key not in _KEYS:
            raise bad(f"unknown key {key!r}")
    primary = entry.get("primary")
    if not isinstance(primary, str) or not primary:
        raise bad("'primary' is required and must be a non-empty string")
    rel = entry.get("min_relative_improvement", 0.0)
    if not _is_num(rel) or rel < 0:
        raise bad("'min_relative_improvement' must be a number >= 0")
    guards = entry.get("guards", {})
    if not isinstance(guards, dict):
        raise bad("'guards' must be a mapping metric -> ratio")
    for k, v in guards.items():
        if not _is_num(v) or not 0 < v <= 1:
            raise bad(f"guard {k!r} must be a ratio in (0, 1]")
    floors = entry.get("floors", {})
    if not isinstance(floors, dict):
        raise bad("'floors' must be a mapping metric -> number")
    for k, v in floors.items():
        if not _is_num(v):
            raise bad(f"floor {k!r} must be a finite number")
    return Policy(primary, float(rel), {str(k): float(v) for k, v in guards.items()},
                  {str(k): float(v) for k, v in floors.items()})


def load_policy(model: str, policies_path: str | os.PathLike[str] | None = None) -> Policy:
    data = _read(policies_path)
    if not isinstance(data, dict):
        raise PolicyError("policies file must be a mapping of model name -> policy")
    if model not in data:
        raise PolicyError(f"no policy for model {model!r}")
    return _parse(model, data[model])
