#!/usr/bin/env python3
"""repo_doctor — static cross-repo drift checker for this polyrepo.

Catches the coherence gaps NO existing gate covers: a service that exists as a
directory but drifted out of docker-compose / the AGENTS.md §2 port table, port
collisions, and services missing the codegen/FEATURES wiring. Pure filesystem +
text — no Go/buf/docker/venv needed, so it runs anywhere and in CI.

It is deliberately NARROW. Dynamic drift (spec↔e2e, contract breaking, build) is
owned by existing gates — the repo-doctor SKILL orchestrates those; this script
only does what a grep-and-cross-reference can do deterministically.

Usage:
  python scripts/repo_doctor.py            # check the workspace at CWD
  python scripts/repo_doctor.py --root .   # explicit root
Exit: 0 = no ERRORs (WARNs allowed), 1 = at least one ERROR (drift that breaks runtime).
"""

import argparse
import re
import sys
from pathlib import Path

COMPOSE = "docker-compose.services.yaml"
AGENTS = "AGENTS.md"

errors: list[str] = []
warns: list[str] = []


def err(msg: str) -> None:
    errors.append(msg)


def warn(msg: str) -> None:
    warns.append(msg)


def load(root: Path, name: str) -> str:
    p = root / name
    return p.read_text(encoding="utf-8") if p.exists() else ""


def team_dirs(root: Path) -> list[str]:
    return sorted(
        p.name for p in root.iterdir() if p.is_dir() and p.name.startswith("team-")
    )


def compose_service_keys(text: str) -> set[str]:
    # keys at 2-space indent, e.g. "  team-order:" — good enough to test presence.
    return set(re.findall(r"(?m)^  ([a-z0-9-]+):\s*$", text))


def agents_table(text: str) -> dict[str, str | None]:
    """Map service name -> declared gRPC/HTTP port string (or None) from §2 rows."""
    rows: dict[str, str | None] = {}
    for m in re.finditer(r"\*\*(team-[a-z]+)\*\*", text):
        name = m.group(1)
        line = text[m.start() : text.find("\n", m.start())]
        # The port lives in the LAST column; earlier ':port' mentions in the
        # description (e.g. identity's JWKS :50063) must not win over :50053.
        ports = re.findall(r":(\d{2,5})", line)
        rows[name] = ports[-1] if ports else None
    return rows


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=".")
    args = ap.parse_args()
    root = Path(args.root).resolve()

    compose = load(root, COMPOSE)
    agents = load(root, AGENTS)
    if not compose:
        err(f"{COMPOSE} not found at {root} — run from the workspace root.")
    if not agents:
        err(f"{AGENTS} not found at {root} — run from the workspace root.")
    if errors:
        _report()
        return 1

    dirs = team_dirs(root)
    ckeys = compose_service_keys(compose)
    table = agents_table(agents)

    # A. Every service directory must be registered to run and documented.
    for d in dirs:
        if d not in ckeys:
            err(
                f"{d}/ exists but has no service in {COMPOSE} — it won't run in the stack."
            )
        if d not in table:
            warn(
                f"{d}/ exists but is missing from the AGENTS.md §2 port table (doc drift)."
            )

    # A2. Reverse: a documented/compose service with no directory.
    for name in table:
        if name not in dirs:
            warn(f"AGENTS.md documents {name} but there is no {name}/ directory.")

    # B. Port collisions in the documented port table.
    seen: dict[str, str] = {}
    for name, port in table.items():
        if not port:
            continue
        if port in seen:
            err(f"Port :{port} is claimed by BOTH {seen[port]} and {name} (collision).")
        else:
            seen[port] = name

    # C. Per-service wiring hygiene (Go services).
    # The gateway is the edge (no business capabilities); a worker (cmd/consumer,
    # no request-serving RPC) owns no user-facing capabilities either — neither is
    # tracked by the spec↔e2e gate, so FEATURES.yaml is not expected of them.
    EDGE = {"team-gateway"}
    for d in dirs:
        sd = root / d
        is_go = (sd / "go.mod").exists()
        if not is_go:
            continue
        is_worker = (sd / "cmd" / "consumer").exists()
        serves_capabilities = d not in EDGE and not is_worker
        if serves_capabilities and not (sd / "FEATURES.yaml").exists():
            warn(
                f"{d}/ has no FEATURES.yaml — spec↔e2e gate can't track its capabilities."
            )
        has_bufgen = (sd / "buf.gen.yaml").exists()
        has_out = (sd / "generated").exists() or (sd / "proto-vendor").exists()
        if has_bufgen and not has_out:
            warn(
                f"{d}/ has buf.gen.yaml but no generated/ or proto-vendor/ — codegen not run (contract drift)."
            )

    _report()
    return 1 if errors else 0


def _report() -> None:
    for w in warns:
        print(f"⚠️  {w}")
    for e in errors:
        print(f"❌ {e}")
    if not errors and not warns:
        print("✅ repo-doctor: no structural drift.")
    elif not errors:
        print(f"✅ repo-doctor: {len(warns)} warning(s), no runtime-breaking drift.")
    else:
        print(f"❌ repo-doctor: {len(errors)} error(s), {len(warns)} warning(s).")


if __name__ == "__main__":
    sys.exit(main())
