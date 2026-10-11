#!/usr/bin/env python3
"""Load, validate and query asdlc.config.yaml (solo/team strictness profiles).

Resolved config = shared top-level keys + the active profile's keys flattened in.

Usage:
  python scripts/asdlc_config.py --check                 # exit 0 valid / 1 with error list
  python scripts/asdlc_config.py --get thresholds.noul   # print resolved value as JSON
  python scripts/asdlc_config.py --get kev_mode --profile team

Import:
  from asdlc_config import load, route
  cfg = load()                       # dict, active profile merged
  route(["team-payment/x.go"])       # -> {"payment"}
"""
import argparse
import json
import sys
from fnmatch import fnmatchcase
from pathlib import Path

import yaml

DEFAULT_PATH = Path(__file__).resolve().parent.parent / "asdlc.config.yaml"
GATES = ("spec", "task", "feature", "release")
TIERS = ("low", "medium", "high")
KEV_MODES = ("advisory", "enforce")
RISK_TAGS = ("auth", "payment", "pii", "proto", "release")
PROFILE_NAMES = ("solo", "team")


def _read(path):
    return yaml.safe_load(Path(path).read_text(encoding="utf-8"))


def _is_int(v):
    return isinstance(v, int) and not isinstance(v, bool)


def _is_num(v):
    return isinstance(v, (int, float)) and not isinstance(v, bool)


def _key_paths(d, prefix=""):
    out = set()
    for k, v in d.items():
        p = f"{prefix}{k}"
        out.add(p)
        if isinstance(v, dict):
            out |= _key_paths(v, p + ".")
    return out


def _check_profile(name, p, errs):
    where = f"profiles.{name}"
    if not isinstance(p, dict):
        errs.append(f"{where}: must be a mapping")
        return
    wip = p.get("wip_limit")
    if not _is_int(wip) or wip < 1:
        errs.append(f"{where}.wip_limit: must be an integer >= 1")
    ap = p.get("approvals")
    if not isinstance(ap, dict):
        errs.append(f"{where}.approvals: must be a mapping")
    else:
        for t in TIERS:
            v = ap.get(t)
            if not _is_int(v) or v < 0:
                errs.append(f"{where}.approvals.{t}: must be an integer >= 0")
    rv = p.get("reviewers")
    if not isinstance(rv, dict):
        errs.append(f"{where}.reviewers: must be a mapping")
    else:
        for g in GATES:
            v = rv.get(g)
            if (not isinstance(v, list) or not v
                    or not all(isinstance(x, str) and x for x in v)):
                errs.append(f"{where}.reviewers.{g}: must be a non-empty list of strings")
    if p.get("kev_mode") not in KEV_MODES:
        errs.append(f"{where}.kev_mode: must be one of {list(KEV_MODES)}")


def validate(raw):
    """Return a list of error strings for a raw (unmerged) config mapping."""
    errs = []
    if not isinstance(raw, dict):
        return ["config: top level must be a mapping"]
    profiles = raw.get("profiles")
    if not isinstance(profiles, dict):
        errs.append("profiles: must be a mapping")
        profiles = {}
    for n in PROFILE_NAMES:
        if n not in profiles:
            errs.append(f"profiles.{n}: missing")
    if raw.get("profile") not in profiles:
        errs.append(f"profile: must name a defined profile (got {raw.get('profile')!r})")

    th = raw.get("thresholds")
    if not isinstance(th, dict):
        errs.append("thresholds: must be a mapping")
    else:
        for k in ("noul", "choice"):
            v = th.get(k)
            if not _is_num(v) or not (0.5 < v < 1):
                errs.append(f"thresholds.{k}: must be a number in (0.5, 1)")

    es = raw.get("escalation")
    mr = es.get("max_rounds") if isinstance(es, dict) else None
    if not _is_int(mr) or mr < 1:
        errs.append("escalation.max_rounds: must be an integer >= 1")

    rt = raw.get("routing")
    if not isinstance(rt, dict):
        errs.append("routing: must be a mapping")
    else:
        for tag in RISK_TAGS:
            v = rt.get(tag)
            if (not isinstance(v, list) or not v
                    or not all(isinstance(x, str) and x for x in v)):
                errs.append(f"routing.{tag}: must be a non-empty list of glob strings")

    pa = raw.get("paths")
    if not isinstance(pa, dict):
        errs.append("paths: must be a mapping")
    else:
        for k in ("gates_dir", "kev_client", "kev_log"):
            if not isinstance(pa.get(k), str) or not pa.get(k):
                errs.append(f"paths.{k}: must be a non-empty string")

    for n, p in profiles.items():
        _check_profile(n, p, errs)
    dicts = {n: p for n, p in profiles.items() if isinstance(p, dict)}
    if len(dicts) > 1:
        sets = {n: _key_paths(p) for n, p in dicts.items()}
        union = set().union(*sets.values())
        for n, s in sets.items():
            if union - s:
                errs.append(f"profiles.{n}: key set differs from other profiles; "
                            f"missing {sorted(union - s)}")
    return errs


def load(profile=None, path=None):
    """Load config, validate, and merge the active (or given) profile into the top level."""
    raw = _read(path or DEFAULT_PATH)
    errs = validate(raw)
    if profile is not None and isinstance(raw, dict) and profile not in (raw.get("profiles") or {}):
        errs.append(f"profile: unknown profile {profile!r}")
    if errs:
        raise ValueError("invalid asdlc config:\n  " + "\n  ".join(errs))
    name = profile or raw["profile"]
    cfg = {k: v for k, v in raw.items() if k not in ("profiles", "profile")}
    cfg.update(raw["profiles"][name])
    cfg["profile"] = name
    return cfg


def route(paths, config=None):
    """Return the set of risk tags (auth/payment/pii/proto/release) hit by changed paths.

    Matching is fnmatch, case-insensitive on both path and glob.
    """
    cfg = config or load()
    hits = set()
    for p in paths:
        p = (p[2:] if p.startswith("./") else p).lower()
        for tag, globs in cfg["routing"].items():
            if any(fnmatchcase(p, g.lower()) for g in globs):
                hits.add(tag)
    return hits


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--check", action="store_true", help="validate; exit 1 on errors")
    ap.add_argument("--get", metavar="KEY", help="print resolved dotted key as JSON")
    ap.add_argument("--profile", help="override the active profile")
    ap.add_argument("--config", help="path to config file (default: repo asdlc.config.yaml)")
    args = ap.parse_args()
    if not args.check and not args.get:
        ap.error("give --check or --get")

    try:
        raw = _read(args.config or DEFAULT_PATH)
    except (OSError, yaml.YAMLError) as e:
        print(f"ERROR: cannot read config: {e}", file=sys.stderr)
        return 1
    errs = validate(raw)
    if args.profile and isinstance(raw, dict) and args.profile not in (raw.get("profiles") or {}):
        errs.append(f"profile: unknown profile {args.profile!r}")
    if errs:
        for e in errs:
            print(f"ERROR: {e}", file=sys.stderr)
        return 1
    if args.check:
        print(f"OK profile={args.profile or raw['profile']}")
    if args.get:
        cur = load(args.profile, args.config)
        for part in args.get.split("."):
            if not isinstance(cur, dict) or part not in cur:
                print(f"ERROR: no such key: {args.get}", file=sys.stderr)
                return 1
            cur = cur[part]
        print(json.dumps(cur))
    return 0


if __name__ == "__main__":
    sys.exit(main())
