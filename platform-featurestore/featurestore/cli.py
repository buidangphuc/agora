"""`python -m featurestore {materialize,parity,lock,dataset}`. Exit codes: 0 ok, 2 config/input, 3 parity, 4 registry drift."""

from __future__ import annotations

import sys

from featurestore import dataset, job, parity, registry
from featurestore.settings import ConfigError, Settings


def _redis(settings: Settings, client):
    if client is not None:
        return client
    if not settings.redis_url:
        raise ConfigError("FEATURESTORE_REDIS_URL is not set")
    import redis

    return redis.Redis.from_url(settings.redis_url, decode_responses=True)


def main(argv: list[str] | None = None, env: dict | None = None, redis_client=None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1 or args[0] not in ("materialize", "parity", "lock", "dataset"):
        print("usage: python -m featurestore {materialize|parity|lock|dataset}", file=sys.stderr)
        return 2
    cmd = args[0]
    try:
        settings = Settings.from_env(env)
        views = registry.load_registry(settings.registry_dir)
        datasets = registry.load_datasets(settings.registry_dir)
        if cmd == "lock":
            registry.write_lock(settings.registry_dir, views, datasets)
            print(f"wrote {settings.registry_dir / 'features.lock'}")
            return 0
        try:
            registry.check_lock(settings.registry_dir, views, datasets)
        except registry.RegistryDrift as exc:
            print(f"registry drift: {exc}", file=sys.stderr)
            return 4
        if cmd == "dataset":
            for m in dataset.build_all(settings, datasets):
                print(
                    f"dataset {m['name']}@v{m['version']} rows={m['rows']} as_of={m['as_of']} file={m['file']}"
                )
            return 0
        r = _redis(settings, redis_client)
        if cmd == "materialize":
            manifest = job.materialize(settings, views, r)
            # Compare online values with the snapshot files this run wrote (not the
            # in-memory rows), so a Parquet write or type problem fails the run too.
            manifest.pop("_rows", None)
            mismatches = parity.check_manifest(r, settings.offline_dir, manifest, settings.parity_sample)
            for v in manifest["views"]:
                print(f"materialized {v['name']}@v{v['version']} rows={v['rows']} as_of={manifest['as_of']}")
        else:
            manifest = parity.latest_manifest(settings.offline_dir)
            mismatches = parity.check_manifest(r, settings.offline_dir, manifest, settings.parity_sample)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    if mismatches:
        for m in mismatches:
            print(m.line())
        return 3
    print("parity ok")
    return 0
