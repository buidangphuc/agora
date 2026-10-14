"""`python -m modelregistry promote|rollback`. Exit codes: 0 done (incl. rejected), 1 unavailable
or unexpected, 2 usage/config/gate errors."""
from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Sequence

from .errors import ModelRegistryError, RegistryUnavailable
from .registry import promote, rollback


def _parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="python -m modelregistry")
    sub = p.add_subparsers(dest="cmd", required=True)
    pr = sub.add_parser("promote", help="promote a version through the gate (or --force)")
    pr.add_argument("model")
    pr.add_argument("version", type=int)
    pr.add_argument("--force", action="store_true")
    pr.add_argument("--reason", default=None)
    rb = sub.add_parser("rollback", help="swap champion and previous")
    rb.add_argument("model")
    return p


def main(argv: Sequence[str] | None = None) -> int:
    try:
        args = _parser().parse_args(argv)
    except SystemExit as e:
        return int(e.code) if isinstance(e.code, int) else 2
    try:
        if args.cmd == "promote":
            d = promote(args.model, args.version, force=args.force, reason=args.reason)
        else:
            d = rollback(args.model)
        print(json.dumps({"model": args.model, "version": d.version, "outcome": d.outcome,
                          "reason": d.reason}))
        return 0
    except RegistryUnavailable as e:
        print(f"RegistryUnavailable: {e}", file=sys.stderr)
        return 1
    except (ModelRegistryError, ValueError) as e:
        print(f"{type(e).__name__}: {e}", file=sys.stderr)
        return 2
    except Exception as e:  # noqa: BLE001
        print(f"{type(e).__name__}: {e}", file=sys.stderr)
        return 1
