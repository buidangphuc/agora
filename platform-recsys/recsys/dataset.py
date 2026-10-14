"""Governed training dataset resolution (written by platform-featurestore).

Files live at ``<DATASET_DIR>/as_of=<YYYYMMDDTHHMMSSZ>.parquet`` with an
``as_of=<stamp>.manifest.json`` beside each. ``DATASET_PATH`` (an explicit file)
overrides ``DATASET_DIR``; its manifest is the same name with ``.parquet`` replaced
by ``.manifest.json``. There is no fallback to raw events: with no dataset the job
refuses to start (ConfigError → exit 2, nothing registered).

Pure stdlib so it unit-tests without PySpark.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from .config import ConfigError, Settings

MANIFEST_SUFFIX = ".manifest.json"
_REQUIRED_MANIFEST_KEYS = ("name", "version", "as_of", "file_sha256")


@dataclass(frozen=True)
class Dataset:
    path: str
    manifest: dict

    @property
    def lineage(self) -> dict:
        """What a registered model records in parameters["dataset"]."""
        m = self.manifest
        return {
            "name": m["name"],
            "version": m["version"],
            "as_of": m["as_of"],
            "sha256": m["file_sha256"],
        }


def manifest_path_for(parquet: Path) -> Path:
    return parquet.with_name(parquet.name[: -len(".parquet")] + MANIFEST_SUFFIX)


def _load(parquet: Path, setting: str) -> Dataset:
    manifest_file = manifest_path_for(parquet)
    try:
        manifest = json.loads(manifest_file.read_text())
    except FileNotFoundError:
        raise ConfigError(f"{setting}: manifest {manifest_file} not found beside {parquet}") from None
    except ValueError as exc:
        raise ConfigError(f"{setting}: manifest {manifest_file} is not valid JSON: {exc}") from None
    missing = [k for k in _REQUIRED_MANIFEST_KEYS if k not in manifest]
    if missing:
        raise ConfigError(f"{setting}: manifest {manifest_file} is missing keys {missing}")
    return Dataset(path=str(parquet), manifest=manifest)


def resolve_named(directory: str, explicit: str, dir_setting: str, path_setting: str) -> Dataset:
    """``explicit`` (a file), else the lexically latest complete ``as_of=*.parquet`` under ``directory``.
    The settings' names are used in the errors so the operator knows what to set."""
    explicit = explicit.strip()
    if explicit:
        parquet = Path(explicit)
        if not parquet.is_file():
            raise ConfigError(f"no governed dataset at {path_setting}={explicit}")
        if parquet.suffix != ".parquet":
            raise ConfigError(f"{path_setting}={explicit} must be a .parquet file")
        return _load(parquet, path_setting)

    # A snapshot without its manifest is not a governed dataset (and may be mid-write).
    candidates = sorted(
        p for p in Path(directory).glob("as_of=*.parquet") if p.is_file() and manifest_path_for(p).is_file()
    )
    if not candidates:
        raise ConfigError(f"no governed dataset under {dir_setting}={directory}")
    return _load(candidates[-1], dir_setting)


def _check_fresh(dataset: Dataset, max_age_minutes: int, now: datetime | None) -> None:
    """Refuse a snapshot whose manifest ``as_of`` is older than ``max_age_minutes`` (inclusive; 0 = off)."""
    if max_age_minutes <= 0:
        return
    as_of = dataset.manifest["as_of"]
    try:
        stamp = datetime.strptime(str(as_of), "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc)
    except ValueError:
        raise ConfigError(
            f"dataset {dataset.path}: manifest as_of {as_of!r} is not a UTC timestamp"
        ) from None
    age = ((now or datetime.now(timezone.utc)) - stamp).total_seconds() / 60.0
    if age > max_age_minutes:
        raise ConfigError(
            f"dataset {dataset.path} (as_of {as_of}) is {age:.0f} minutes old, "
            f"older than MAX_DATASET_AGE_MINUTES={max_age_minutes}"
        )


def resolve_dataset(settings: Settings, now: datetime | None = None) -> Dataset:
    """DATASET_PATH, else the latest complete as_of=*.parquet under DATASET_DIR, if fresh enough
    (MAX_DATASET_AGE_MINUTES; ``now`` is injectable for tests)."""
    dataset = resolve_named(settings.dataset_dir, settings.dataset_path, "DATASET_DIR", "DATASET_PATH")
    _check_fresh(dataset, settings.max_dataset_age_minutes, now)
    return dataset


def resolve_rank_dataset(settings: Settings) -> Dataset:
    """RANK_DATASET_PATH, else the latest complete snapshot under RANK_DATASET_DIR (``rank_training@v1``)."""
    return resolve_named(
        settings.rank_dataset_dir, settings.rank_dataset_path, "RANK_DATASET_DIR", "RANK_DATASET_PATH"
    )
