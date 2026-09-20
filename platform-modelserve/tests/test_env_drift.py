"""Tests for environment variable drift."""

from __future__ import annotations

from pathlib import Path

from modelserve.config import Settings


def test_env_example_drift() -> None:
    env_example_path = Path(__file__).parent.parent / ".env.example"
    assert env_example_path.exists(), ".env.example file must exist"

    example_keys = set()
    for line in env_example_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key = line.split("=", 1)[0].strip()
            example_keys.add(key)

    settings_keys = {field.alias or name for name, field in Settings.model_fields.items()}

    # All keys in .env.example should correspond to known settings
    for key in example_keys:
        assert key in settings_keys, f"Key {key} in .env.example not recognized in Settings"
