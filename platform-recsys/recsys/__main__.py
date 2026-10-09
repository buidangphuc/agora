"""Batch entrypoint.

Runnable via ``python -m recsys`` (in-process local[*] SparkSession) or under
``spark-submit recsys/__main__.py``. No request-serving surface — this is a
scheduled batch job (a platform-gitops CronJob), not a long-running server.

``python -m recsys rollback`` restores the previous generation instead of training: exit 0 on
success, 2 (nothing changed) when there is no previous generation to restore.
"""

from __future__ import annotations

import logging
import sys

from .config import ConfigError, load_settings
from .pipeline import run


def _rollback(settings) -> int:
    from .load import redis_cache  # noqa: PLC0415
    from .publish import RollbackRefused, rollback  # noqa: PLC0415
    from .registry.registry import ModelRegistry  # noqa: PLC0415

    log = logging.getLogger("recsys")
    try:
        client = redis_cache.connect(settings)
        client.ping()
    except Exception as exc:  # Redis holds the pointers and the registry: nothing to roll back without it
        log.error("rollback needs Redis: %s", exc)
        return 1
    try:
        result = rollback(settings, ModelRegistry(redis_client=client), redis_client=client)
    except RollbackRefused as exc:
        log.error("rollback refused: %s", exc)
        return 2
    log.info("rolled back: %s", result)
    return 0


def main(argv: list[str] | None = None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    settings = load_settings()
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    if argv and argv[0] == "rollback":
        return _rollback(settings)
    if argv:
        logging.getLogger("recsys").error("unknown command %r (only 'rollback' is supported)", argv[0])
        return 2
    try:
        summary = run(settings)
    except ConfigError as exc:
        logging.getLogger("recsys").error("cannot start: %s", exc)
        return 2
    except Exception:  # e.g. a failed publish: the candidate was recorded as rejected
        logging.getLogger("recsys").exception("batch failed")
        return 1
    logging.getLogger("recsys").info("done: %s", summary)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
