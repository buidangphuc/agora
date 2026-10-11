"""Stack helpers for the c1 e2e track (delivery hardening, shipment events, cockpit).

Kafka/consumer-group inspection goes through `rpk` inside the redpanda container, and
fault injection through the docker CLI (the runner shares the host with the compose
stack), like `tests/e2e/flows/stack_flow.py`.
"""

from __future__ import annotations

import os
import subprocess
import time

_DOCKER_TIMEOUT_S = 90


def redpanda_container() -> str:
    return os.getenv("REDPANDA_CONTAINER", "agora-redpanda-1")


def identity_container() -> str:
    return os.getenv("IDENTITY_CONTAINER", "agora-team-identity-svc")


def jaeger_container() -> str:
    return os.getenv("JAEGER_CONTAINER", "agora-jaeger-1")


def notification_container() -> str:
    return os.getenv("NOTIFICATION_CONTAINER", "agora-team-notification-svc")


def docker(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", *args],
        check=check,
        capture_output=True,
        text=True,
        timeout=_DOCKER_TIMEOUT_S,
    )


def rpk(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return docker("exec", redpanda_container(), "rpk", *args, check=check)


def wait_kafka_ready(timeout_s: float = 90.0) -> None:
    """Block until redpanda reports a healthy cluster."""
    deadline = time.monotonic() + timeout_s
    last = "no response"
    while time.monotonic() < deadline:
        out = rpk("cluster", "health", check=False)
        last = (out.stdout or out.stderr).strip()
        for line in out.stdout.splitlines():
            if line.startswith("Healthy:") and "true" in line:
                return
        time.sleep(1)
    raise TimeoutError(f"redpanda not healthy within {timeout_s}s ({last})")


def group_topic_offsets(group: str, topic: str) -> tuple[int, int]:
    """(committed offset, log-end offset) of `topic` for `group`, summed over partitions."""
    out = rpk("group", "describe", group, check=False).stdout
    current = end = 0
    seen = False
    for line in out.splitlines():
        cols = line.split()
        # TOPIC PARTITION CURRENT-OFFSET LOG-END-OFFSET LAG ...
        if len(cols) >= 5 and cols[0] == topic and cols[1].isdigit():
            if cols[2].lstrip("-").isdigit() and cols[3].lstrip("-").isdigit():
                current += max(int(cols[2]), 0)
                end += max(int(cols[3]), 0)
                seen = True
    if not seen:
        raise AssertionError(f"group {group} has no committed offsets for {topic}:\n{out}")
    return current, end


def topic_end_offset(topic: str) -> int:
    """Sum of the high watermarks of `topic` (all partitions)."""
    out = rpk("topic", "describe", topic, "-p", check=False).stdout
    total = 0
    for line in out.splitlines():
        cols = line.split()
        # PARTITION LEADER EPOCH REPLICAS LOG-START-OFFSET HIGH-WATERMARK
        if len(cols) >= 6 and cols[0].isdigit() and cols[-1].isdigit():
            total += int(cols[-1])
    return total
