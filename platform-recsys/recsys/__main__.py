"""Batch entrypoint.

Runnable via ``python -m recsys`` (in-process local[*] SparkSession) or under
``spark-submit recsys/__main__.py``. No request-serving surface — this is a
scheduled batch job (a platform-gitops CronJob), not a long-running server.
"""

from __future__ import annotations

import logging
import sys

from .config import load_settings
from .pipeline import run


def main(argv: list[str] | None = None) -> int:
    settings = load_settings()
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    summary = run(settings)
    logging.getLogger("recsys").info("done: %s", summary)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
