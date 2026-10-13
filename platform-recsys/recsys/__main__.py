"""Batch entrypoint.

Runnable via ``python -m recsys`` (in-process local[*] SparkSession) or under
``spark-submit recsys/__main__.py``. No request-serving surface — this is a
scheduled batch job (a platform-gitops CronJob), not a long-running server.

Exit codes (round-2 decision D4): 0 = completed, including a ``rejected`` decision; 1 = the registry is
unavailable or an unexpected failure; 2 = usage or configuration error, policy, gate or rollback error,
stale or missing dataset.

``python -m recsys rollback`` restores the previous generation instead of training: exit 0 on
success, 2 (nothing changed) when there is no previous generation to restore.
"""

from __future__ import annotations

import logging
import sys

from modelregistry import GateError, PolicyError, RegistryUnavailable, RollbackError

from .config import ConfigError, load_settings
from .pipeline import run

EXIT_FAILED = 1
EXIT_REFUSED = 2

_REFUSALS = (ConfigError, PolicyError, GateError, RollbackError)


def _rollback(settings) -> int:
    from .load import redis_cache  # noqa: PLC0415
    from .publish import RollbackRefused, rollback  # noqa: PLC0415

    log = logging.getLogger("recsys")
    try:
        client = redis_cache.connect(settings)
        client.ping()
    except Exception as exc:  # Redis holds the pointers: nothing to roll back without it
        log.error("rollback needs Redis: %s", exc)
        return EXIT_FAILED
    try:
        result = rollback(settings, redis_client=client)
    except (RollbackRefused, *_REFUSALS) as exc:
        log.error("rollback refused: %s", exc)
        return EXIT_REFUSED
    except RegistryUnavailable as exc:
        log.error("rollback failed, the registry is unavailable: %s", exc)
        return EXIT_FAILED
    except Exception:  # e.g. Redis or Qdrant failed mid-way: the registry was put back
        log.exception("rollback failed, nothing was switched")
        return EXIT_FAILED
    log.info("rolled back: %s", result)
    return 0


def _migrate_registry(settings) -> int:
    from .load import redis_cache  # noqa: PLC0415
    from .migrate import migrate  # noqa: PLC0415

    log = logging.getLogger("recsys")
    try:
        client = redis_cache.connect(settings)
        client.ping()
    except Exception as exc:  # the champions to seed live in Redis
        log.error("migrate-registry needs Redis: %s", exc)
        return EXIT_FAILED
    try:
        outcome = migrate(settings, redis_client=client)
    except RegistryUnavailable as exc:
        log.error("migrate-registry failed, the registry is unavailable: %s", exc)
        return EXIT_FAILED
    log.info("migrate-registry done: %s", outcome)
    return 0


def main(argv: list[str] | None = None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    try:
        settings = load_settings()
    except ValueError as exc:  # a malformed or out-of-range setting (ConfigError is one)
        logging.basicConfig(level=logging.INFO)
        logging.getLogger("recsys").error("cannot start: %s", exc)
        return EXIT_REFUSED
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    if argv and argv[0] == "rollback":
        return _rollback(settings)
    if argv and argv[0] == "migrate-registry":
        return _migrate_registry(settings)
    if argv:
        logging.getLogger("recsys").error(
            "unknown command %r (only 'rollback' and 'migrate-registry' are supported)", argv[0]
        )
        return EXIT_REFUSED
    try:
        summary = run(settings)
    except _REFUSALS as exc:
        logging.getLogger("recsys").error("cannot start: %s", exc)
        return EXIT_REFUSED
    except RegistryUnavailable as exc:
        logging.getLogger("recsys").error("registry unavailable: %s", exc)
        return EXIT_FAILED
    except Exception:  # e.g. a failed stage or publish: the candidates were recorded as rejected
        logging.getLogger("recsys").exception("batch failed")
        return EXIT_FAILED
    logging.getLogger("recsys").info("done: %s", summary)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
