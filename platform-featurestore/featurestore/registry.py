"""The versioned feature registry and its hash lock."""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import yaml

from featurestore.settings import ConfigError


class RegistryDrift(Exception):
    """A definition changed without a version bump (exit code 4)."""


@dataclass(frozen=True)
class View:
    name: str
    version: int
    entity: str
    sql_text: str
    features: dict[str, str]
    sha256: str

    @property
    def key(self) -> str:
        return f"{self.name}@v{self.version}"


@dataclass(frozen=True)
class Dataset:
    name: str
    version: int
    sql_text: str
    sha256: str

    @property
    def key(self) -> str:
        return f"{self.name}@v{self.version}"


def _entry_hash(entry: dict, sql_text: str) -> str:
    canonical = json.dumps(entry, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256((sql_text + "\n" + canonical).encode()).hexdigest()


def load_registry(registry_dir: Path) -> list[View]:
    try:
        doc = yaml.safe_load((registry_dir / "features.yaml").read_text())
    except OSError as exc:
        raise ConfigError(f"cannot read registry: {exc}") from exc
    views = []
    for entry in doc.get("views", []):
        sql_text = (registry_dir / entry["sql"]).read_text()
        views.append(
            View(
                name=entry["name"],
                version=int(entry["version"]),
                entity=entry["entity"],
                sql_text=sql_text,
                features=dict(entry["features"]),
                sha256=_entry_hash(entry, sql_text),
            )
        )
    return views


def load_datasets(registry_dir: Path) -> list[Dataset]:
    try:
        doc = yaml.safe_load((registry_dir / "features.yaml").read_text())
    except OSError as exc:
        raise ConfigError(f"cannot read registry: {exc}") from exc
    out = []
    for entry in doc.get("datasets", []) or []:
        sql_text = (registry_dir / entry["sql"]).read_text()
        out.append(
            Dataset(
                name=entry["name"],
                version=int(entry["version"]),
                sql_text=sql_text,
                sha256=_entry_hash(entry, sql_text),
            )
        )
    return out


def read_lock(registry_dir: Path) -> dict[str, str]:
    p = registry_dir / "features.lock"
    return json.loads(p.read_text()) if p.exists() else {}


def write_lock(registry_dir: Path, views: list[View], datasets: list[Dataset] | None = None) -> None:
    lock = {v.key: v.sha256 for v in [*views, *(datasets or [])]}
    (registry_dir / "features.lock").write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n")


def check_lock(registry_dir: Path, views: list[View], datasets: list[Dataset] | None = None) -> None:
    lock = read_lock(registry_dir)
    for v in [*views, *(datasets or [])]:
        want = lock.get(v.key)
        if want is None:
            raise RegistryDrift(f"{v.key} is not in features.lock (run `python -m featurestore lock`)")
        if want != v.sha256:
            raise RegistryDrift(f"{v.key} definition changed without a version bump")
