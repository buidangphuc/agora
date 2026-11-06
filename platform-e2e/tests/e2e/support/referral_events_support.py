from __future__ import annotations

import os
import time
from dataclasses import dataclass

from tests.e2e.support import plp_stack as stack
from tests.e2e.support import user_enforcement_support as ue

TOPIC = "referral.events"
ATTRIBUTED = "platform.referral.v1.ReferralAttributed"
REJECTED = "platform.referral.v1.ReferralRejected"
RELEASED = "platform.referral.v1.ReferralRewardReleased"
OVERLAY = "platform-e2e/compose/referral-reward.override.yaml"
REFERRAL_SERVICE = "team-referral"
ORDER_SERVICE = "team-order"


@dataclass
class ReferralEvent:
    kind: str
    key: str
    event_id: str
    fields: dict


def _str(payload: dict, number: int) -> str:
    return payload.get(number, [b""])[0].decode()


def _int(payload: dict, number: int) -> int:
    value = payload.get(number, [0])[0]
    return value if isinstance(value, int) else 0


def decode(key: str, raw: bytes) -> ReferralEvent | None:
    env = ue._fields(raw)
    kind = env.get(2, [b""])[0].decode()
    if not kind.startswith("platform.referral.v1."):
        return None
    payload = ue._fields(env[7][0]) if 7 in env else {}
    if kind == ATTRIBUTED:
        fields = {"referee_id": _str(payload, 1), "referrer_id": _str(payload, 2)}
    elif kind == REJECTED:
        fields = {"referee_id": _str(payload, 1), "reason": _int(payload, 2)}
    elif kind == RELEASED:
        fields = {
            "reward_id": _str(payload, 1),
            "referrer_id": _str(payload, 2),
            "referee_id": _str(payload, 3),
            "amount_xu": _int(payload, 4),
        }
    else:
        fields = {}
    return ReferralEvent(kind, key, env.get(1, [b""])[0].decode(), fields)


class ReferralEventsTap:
    def __init__(self) -> None:
        from confluent_kafka import Consumer, TopicPartition

        self._consumer = Consumer(
            {
                "bootstrap.servers": os.getenv("KAFKA_BROKERS", "localhost:19092"),
                "group.id": f"e2e-referral-{time.time_ns()}",
                "enable.auto.commit": False,
            }
        )
        meta = self._consumer.list_topics(TOPIC, timeout=15)
        topic = meta.topics.get(TOPIC)
        assert topic is not None and not topic.error and topic.partitions, (
            f"topic {TOPIC} does not exist on the broker"
        )
        assigns = []
        for partition in topic.partitions:
            _, high = self._consumer.get_watermark_offsets(
                TopicPartition(TOPIC, partition), timeout=15
            )
            assigns.append(TopicPartition(TOPIC, partition, high))
        self._consumer.assign(assigns)
        self.seen: list[ReferralEvent] = []

    def drain(self, wait_s: float) -> None:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            msg = self._consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            event = decode((msg.key() or b"").decode(errors="ignore"), msg.value() or b"")
            if event is not None:
                self.seen.append(event)

    def matching(self, kind: str, referee_id: str) -> list[ReferralEvent]:
        return [e for e in self.seen if e.kind == kind and e.fields.get("referee_id") == referee_id]

    def wait_for(self, kind: str, referee_id: str, wait_s: float) -> list[ReferralEvent]:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            found = self.matching(kind, referee_id)
            if found:
                return found
            self.drain(1.0)
        return self.matching(kind, referee_id)

    def close(self) -> None:
        self._consumer.close()


def _seconds(service: str, key: str, default: float | None = None) -> float | None:
    raw = stack.container_env(service).get(key, "").strip()
    if not raw:
        return default
    return stack.parse_go_duration(raw)


def require_fast_stack(daily_cap: int | None = None) -> tuple[float, float]:
    interval = _seconds(REFERRAL_SERVICE, "REFERRAL_RELEASE_SWEEP_INTERVAL")
    window = _seconds(ORDER_SERVICE, "RETURN_WINDOW")
    problems = []
    if interval is None or interval > 1.0:
        problems.append(f"REFERRAL_RELEASE_SWEEP_INTERVAL is {interval}, want 1s")
    if window is None or window > 3.0:
        problems.append(f"team-order RETURN_WINDOW is {window}, want 3s")
    if daily_cap is not None:
        cap = stack.container_env(REFERRAL_SERVICE).get("REFERRAL_DAILY_CAP", "")
        if cap != str(daily_cap):
            problems.append(f"REFERRAL_DAILY_CAP is {cap!r}, want {daily_cap}")
    assert not problems, (
        f"the referral overlay is not active ({'; '.join(problems)}); start the stack with "
        f"`-f {OVERLAY}`"
    )
    return interval, window
