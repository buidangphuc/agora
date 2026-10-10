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


def resolve_dataset(settings: Settings) -> Dataset:
    """DATASET_PATH, else the latest complete as_of=*.parquet under DATASET_DIR."""
    return resolve_named(settings.dataset_dir, settings.dataset_path, "DATASET_DIR", "DATASET_PATH")


def resolve_rank_dataset(settings: Settings) -> Dataset:
    """RANK_DATASET_PATH, else the latest complete snapshot under RANK_DATASET_DIR (``rank_training@v1``)."""
    return resolve_named(
        settings.rank_dataset_dir, settings.rank_dataset_path, "RANK_DATASET_DIR", "RANK_DATASET_PATH"
    )
