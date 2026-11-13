from __future__ import annotations

from pathlib import Path

from forecast.config import Settings

ROOT = Path(__file__).resolve().parents[1]


def _example_keys() -> set[str]:
    keys = set()
    for line in (ROOT / ".env.example").read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        keys.add(line.split("=", 1)[0].strip())
    return keys


def test_env_example_matches_settings():
    """FNJ-20"""
    declared = {name.upper() for name in Settings.model_fields}
    example = _example_keys()
    missing = declared - example
    extra = example - declared
    assert not missing, f"missing from .env.example: {sorted(missing)}"
    assert not extra, f"unknown in .env.example: {sorted(extra)}"
    assert {"INPUT_DIR", "AS_OF", "BACKTEST_DAYS", "HORIZON_DAYS"} <= declared
    assert not declared & {"ENV", "DUCKDB_PATH", "QUANTILES", "MODEL_VERSION"}
    assert Settings().horizon_days == 14
