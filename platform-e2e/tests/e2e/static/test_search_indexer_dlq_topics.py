from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[4]
SERVICES = ROOT / "docker-compose.services.yaml"
INDEXER_TOPICS = ["listing.events", "order.events", "engagement.events"]


def _created_topics() -> set[str]:
    text = SERVICES.read_text()
    match = re.search(r"rpk topic create (.+?) -X brokers", text)
    assert match, "redpanda-init topic list not found"
    return set(match.group(1).split())


def test_C17_redpanda_init_creates_search_indexer_dlq_topics():
    created = _created_topics()
    missing = [f"search-indexer.{t}.dlq" for t in INDEXER_TOPICS if f"search-indexer.{t}.dlq" not in created]
    assert not missing, f"redpanda-init must create {missing}"
