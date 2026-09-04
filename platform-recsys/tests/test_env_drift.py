"""`.env.example` drift gate.

The env var names declared in recsys.config (_FIELDS) must match `.env.example`
exactly — both directions — mirroring team-analytics' `make check-env`. A new
setting without its `.env.example` line (or a stale line) fails here.
"""

import pathlib

from recsys import config

ROOT = pathlib.Path(__file__).resolve().parent.parent
ENV_EXAMPLE = ROOT / ".env.example"


def _env_example_keys() -> set[str]:
    keys = set()
    for line in ENV_EXAMPLE.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        keys.add(line.split("=", 1)[0].strip())
    return keys


def test_env_example_matches_config_declarations():
    declared = set(config.env_names())
    documented = _env_example_keys()
    missing = declared - documented
    extra = documented - declared
    assert not missing, f".env.example is missing declared vars: {sorted(missing)}"
    assert not extra, f".env.example has undeclared vars: {sorted(extra)}"


def test_no_duplicate_declarations():
    names = config.env_names()
    assert len(names) == len(set(names)), "duplicate env var in config._FIELDS"
