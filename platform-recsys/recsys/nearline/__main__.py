"""``python -m recsys.nearline``: the long-running analytics.events → Redis nearline consumer.

Exit 0 on SIGTERM/SIGINT or (drain mode, ``NEARLINE_IDLE_EXIT_SECONDS``) when the topic has no more
messages; exit 1 when Redis or Kafka fails (uncommitted offsets are replayed on restart).
"""

from __future__ import annotations

import logging
import signal
import sys

from recsys.config import load_settings
from recsys.load import redis_cache
from recsys.nearline.consumer import NearlineConsumer, make_kafka_source
from recsys.nearline.signals import NearlineSignalAggregator, NearlineSignalStore


def main() -> int:
    settings = load_settings()
    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    log = logging.getLogger("recsys.nearline")
    stop = {"now": False}

    def _stop(_signum, _frame) -> None:
        stop["now"] = True

    signal.signal(signal.SIGTERM, _stop)
    signal.signal(signal.SIGINT, _stop)

    try:
        redis = redis_cache.connect(settings)
        redis.ping()
        store = NearlineSignalStore(redis)
        aggregator = NearlineSignalAggregator(store, ttl_seconds=settings.nearline_ttl_seconds)
        consumer = NearlineConsumer(
            make_kafka_source(settings), aggregator, idle_exit_seconds=settings.nearline_idle_exit_seconds
        )
        log.info(
            "consuming %s as group %s from %s into redis %s:%s/%s",
            settings.kafka_analytics_topic,
            settings.nearline_consumer_group,
            settings.kafka_brokers,
            settings.redis_host,
            settings.redis_port,
            settings.redis_db,
        )
        stats = consumer.run(lambda: stop["now"])
    except Exception:
        log.exception("nearline consumer failed")
        return 1
    log.info("nearline consumer stopped: %s", stats)
    return 0


if __name__ == "__main__":
    sys.exit(main())
