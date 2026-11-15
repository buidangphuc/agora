"""``python -m recsys.erasure``: the long-running identity.events UserErased consumer.

Exit 0 on SIGTERM/SIGINT; exit 1 when Redis, Qdrant or Kafka fails (uncommitted offsets are replayed).
"""

from __future__ import annotations

import logging
import signal
import sys

from recsys.config import load_settings
from recsys.erasure.consumer import ErasureConsumer, make_kafka_producer, make_kafka_source
from recsys.load import redis_cache


def main() -> int:
    settings = load_settings()
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    log = logging.getLogger("recsys.erasure")
    stop = {"now": False}

    def _stop(_signum, _frame) -> None:
        stop["now"] = True

    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)

    try:
        from qdrant_client import QdrantClient  # noqa: PLC0415

        redis = redis_cache.connect(settings)
        redis.ping()
        qdrant = QdrantClient(url=settings.qdrant_url)
        consumer = ErasureConsumer(
            make_kafka_source(settings), redis, qdrant, make_kafka_producer(settings), settings
        )
        log.info(
            "consuming %s as group %s from %s",
            settings.identity_events_topic,
            settings.erasure_consumer_group,
            settings.kafka_brokers,
        )
        stats = consumer.run(lambda: stop["now"])
    except Exception:
        log.exception("erasure consumer failed")
        return 1
    log.info("erasure consumer stopped: %s", stats)
    return 0


if __name__ == "__main__":
    sys.exit(main())
