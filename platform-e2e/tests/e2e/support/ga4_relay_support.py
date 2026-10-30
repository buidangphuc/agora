"""Support for the GA4 server-side purchase relay e2e (change add-ga4-purchase-relay).

Black box. The relay consumes `order.events` (OrderPaidEvent) and `analytics.events`
(TrackingEvent) and posts to a GA4 Measurement Protocol endpoint. The test owns that endpoint:
an in-process mock HTTP server records every request and answers per order from a script, so
nothing reaches Google. The relay under test must be deployed with

    GA4_ENDPOINT=http://host.docker.internal:<GA4_MOCK_PORT>/mp/collect
    GA4_API_SECRET / GA4_MEASUREMENT_ID set

Knobs (env): GA4_MOCK_PORT (18110), GA4_RELAY_CONTAINER (agora-ga4-relay-svc),
GA4_RELAY_GRPC_ADDR (localhost:50059, the relay's only listener: gRPC health, reflection on locally),
GA4_RELAY_DLQ_TOPIC (analytics.ga4-relay.dlq),
GA4_MAPPING_SETTLE_S (3, pipeline settle after producing mapping events),
E2E_KAFKA_INTRUDER_* (SASL creds of a non-order principal for the forged-event scenario).
Protobuf is encoded by hand (field numbers from the public protos), no generated stubs.
"""

from __future__ import annotations

import json
import os
import subprocess
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any
from urllib.parse import parse_qs, urlparse

from config.settings import get_settings
from tests.e2e.flows.tracking_flow import _fields

ORDER_PAID_TYPE = "platform.order.v1.OrderPaidEvent"
TRACKING_TYPE = "platform.analytics.v1.TrackingEvent"
MOCK_PORT = int(os.getenv("GA4_MOCK_PORT", "18110"))
MOCK_PATH = "/mp/collect"
API_SECRET_MARK = "api_secret"


def relay_container() -> str:
    return os.getenv("GA4_RELAY_CONTAINER", "agora-ga4-relay-svc")


def relay_grpc_addr() -> str:
    return os.getenv("GA4_RELAY_GRPC_ADDR", "localhost:50059")


def health_check(addr: str, service: str = "") -> str:
    """grpc.health.v1.Health/Check status ("SERVING", ...) or "UNREACHABLE: <reason>"; empty service = overall."""
    res = subprocess.run(
        ["grpcurl", "-plaintext", "-max-time", "5", "-d", json.dumps({"service": service}),
         addr, "grpc.health.v1.Health/Check"],
        capture_output=True, text=True, timeout=30,
    )  # fmt: skip
    if res.returncode != 0:
        return f"UNREACHABLE: {(res.stderr or res.stdout).strip()[:300]}"
    return str(json.loads(res.stdout).get("status", "UNKNOWN"))


def dlq_topic() -> str:
    return os.getenv("GA4_RELAY_DLQ_TOPIC", "analytics.ga4-relay.dlq")


def settle_s() -> float:
    return float(os.getenv("GA4_MAPPING_SETTLE_S", "3"))


def uid(prefix: str) -> str:
    return f"{prefix}-{uuid.uuid4().hex[:10]}"


# ── protobuf encoding ────────────────────────────────────────────────────
def _varint(n: int) -> bytes:
    n &= (1 << 64) - 1
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def _tag(number: int, wire: int) -> bytes:
    return _varint((number << 3) | wire)


def f_varint(number: int, value: int) -> bytes:
    return _tag(number, 0) + _varint(value) if value else b""


def f_bytes(number: int, value: bytes | str) -> bytes:
    raw = value.encode() if isinstance(value, str) else value
    return _tag(number, 2) + _varint(len(raw)) + raw if raw else b""


def envelope(type_name: str, payload: bytes, principal_id: str = "") -> bytes:
    principal = f_bytes(1, principal_id) + f_varint(2, 2 if principal_id else 1)
    return (
        f_bytes(1, str(uuid.uuid4()))
        + f_bytes(2, type_name)
        + f_bytes(4, principal)
        + f_bytes(7, payload)
    )


def tracking_event(ga_client_id: str, analytics: bool, session: str) -> bytes:
    consent = f_varint(1, 1 if analytics else 0) + f_varint(4, 1)
    return (
        f_varint(1, 1)  # EVENT_TYPE_VIEW
        + f_bytes(3, session)
        + f_bytes(25, ga_client_id)
        + f_bytes(26, consent)
    )


def order_paid(
    order_id: str,
    buyer_id: str,
    items: list[dict[str, Any]],
    total: int,
    currency: str = "VND",
    paid_at: int | None = None,
) -> bytes:
    body = f_bytes(1, order_id) + f_bytes(2, buyer_id)
    for it in items:
        line = (
            f_bytes(1, it["listing_id"])
            + f_bytes(2, it.get("variant_id", ""))
            + f_bytes(3, it.get("seller_id", "e2e-seller"))
            + f_varint(4, it["quantity"])
            + f_varint(5, it["unit_price"])
            + f_bytes(6, currency)
        )
        body += f_bytes(3, line)
    body += f_varint(4, total) + f_bytes(5, currency)
    if paid_at is not None:
        body += f_bytes(6, f_varint(1, paid_at))  # google.protobuf.Timestamp.seconds
    return body


# ── Kafka ────────────────────────────────────────────────────────────────
def _producer(extra: dict[str, str] | None = None):
    from confluent_kafka import Producer

    conf = {"bootstrap.servers": get_settings().kafka_brokers}
    conf.update(extra or {})
    return Producer(conf)


def produce(topic: str, key: str, value: bytes, extra: dict[str, str] | None = None) -> None:
    """Produce one record and wait for its delivery report; raises on a broker error."""
    errors: list[Any] = []
    p = _producer(extra)
    p.produce(topic, key=key, value=value, on_delivery=lambda err, _m: errors.append(err))
    p.flush(15)
    if not errors:
        raise RuntimeError(f"no delivery report for a record to {topic}")
    if errors[0] is not None:
        raise RuntimeError(f"produce to {topic} failed: {errors[0]}")


def produce_tracking(buyer_id: str, ga_client_id: str, analytics: bool) -> None:
    payload = tracking_event(ga_client_id, analytics, uid("sess"))
    produce(
        get_settings().kafka_analytics_topic,
        buyer_id,
        envelope(TRACKING_TYPE, payload, principal_id=buyer_id),
    )


def produce_order_paid(payload: bytes, key: str, type_name: str = ORDER_PAID_TYPE) -> None:
    produce(get_settings().kafka_order_topic, key, envelope(type_name, payload))


def dlq_records(contains: str, timeout_s: float = 20.0) -> list[bytes]:
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-ga4-dlq-{time.time()}",
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }
    )
    c.assign([TopicPartition(dlq_topic(), 0, OFFSET_BEGINNING)])
    found: list[bytes] = []
    deadline = time.time() + timeout_s
    try:
        while time.time() < deadline and not found:
            msg = c.poll(0.5)
            if msg is not None and not msg.error() and contains.encode() in (msg.value() or b""):
                found.append(bytes(msg.value()))
    finally:
        c.close()
    return found


def consumer_groups() -> set[str]:
    from confluent_kafka.admin import AdminClient

    admin = AdminClient({"bootstrap.servers": get_settings().kafka_brokers})
    res = admin.list_consumer_groups().result(timeout=15)
    return {g.group_id for g in res.valid}


# ── GA4 MP mock ──────────────────────────────────────────────────────────
class Ga4Mock:
    """Records MP requests; replies per order id from a script of statuses.

    A script entry is an int status, or "hang:<seconds>" (sleep, then 204), or "drop"
    (close the connection with no answer). When the script is exhausted the reply is 204.
    """

    def __init__(self) -> None:
        self.requests: list[dict[str, Any]] = []
        self._scripts: dict[str, list[Any]] = {}
        self._lock = threading.Lock()
        self._server: ThreadingHTTPServer | None = None

    def script(self, order_id: str, replies: list[Any]) -> None:
        with self._lock:
            self._scripts[order_id] = list(replies)

    def start(self) -> None:
        mock = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_a: Any) -> None:  # noqa: D401
                return

            def do_POST(self) -> None:  # noqa: N802
                length = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(length).decode("utf-8", "replace")
                url = urlparse(self.path)
                try:
                    body = json.loads(raw)
                except ValueError:
                    body = {}
                events = body.get("events") or []
                tx = ""
                for ev in events:
                    tx = (ev.get("params") or {}).get("transaction_id") or tx
                with mock._lock:
                    mock.requests.append(
                        {
                            "t": time.monotonic(),
                            "path": url.path,
                            "query": parse_qs(url.query),
                            "raw_url": self.path,
                            "raw": raw,
                            "body": body,
                            "order": tx,
                        }
                    )
                    queue = mock._scripts.get(tx) or []
                    reply: Any = queue.pop(0) if queue else 204
                if reply == "drop":
                    self.close_connection = True
                    self.connection.close()
                    return
                if isinstance(reply, str) and reply.startswith("hang:"):
                    time.sleep(float(reply.split(":", 1)[1]))
                    reply = 204
                self.send_response(int(reply))
                self.send_header("Content-Length", "0")
                self.end_headers()

        self._server = ThreadingHTTPServer(("0.0.0.0", MOCK_PORT), Handler)
        threading.Thread(target=self._server.serve_forever, daemon=True).start()

    def stop(self) -> None:
        if self._server is not None:
            self._server.shutdown()
            self._server.server_close()
            self._server = None

    def for_order(self, order_id: str) -> list[dict[str, Any]]:
        with self._lock:
            return [r for r in self.requests if r["order"] == order_id]

    def wait_for(self, order_id: str, count: int, timeout_s: float = 60.0) -> list[dict[str, Any]]:
        deadline = time.monotonic() + timeout_s
        while time.monotonic() < deadline:
            got = self.for_order(order_id)
            if len(got) >= count:
                return got
            time.sleep(0.25)
        return self.for_order(order_id)

    def quiet_for(self, order_id: str, quiet_s: float) -> list[dict[str, Any]]:
        """Requests for the order after no new one arrived for quiet_s (an 'exactly N' check)."""
        last, since = -1, time.monotonic()
        while time.monotonic() - since < quiet_s:
            n = len(self.for_order(order_id))
            if n != last:
                last, since = n, time.monotonic()
            time.sleep(0.25)
        return self.for_order(order_id)


# ── docker helpers ───────────────────────────────────────────────────────
def docker(*args: str, check: bool = True, timeout: int = 120) -> subprocess.CompletedProcess[str]:
    res = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=timeout)
    if check and res.returncode != 0:
        raise RuntimeError(f"docker {' '.join(args)} failed: {res.stderr.strip()}")
    return res


def container_env(name: str) -> dict[str, str]:
    out = docker("inspect", name, "--format", "{{json .Config.Env}}").stdout
    env = {}
    for item in json.loads(out):
        k, _, v = item.partition("=")
        env[k] = v
    return env


def decode_dlq_order(raw: bytes) -> str:
    """The order id embedded in a DLQ record (best effort: first string field of the payload)."""
    for number, wire, value in _fields(raw):
        if number == 7 and wire == 2:
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n == 1 and w == 2:
                    return bytes(v).decode()  # type: ignore[arg-type]
    return ""


def group_lag(group: str, topic: str) -> int:
    """Total lag of the relay's consumer group on `topic` (committed offsets vs high watermarks)."""
    from confluent_kafka import Consumer, TopicPartition
    from confluent_kafka.admin import AdminClient

    brokers = get_settings().kafka_brokers
    admin = AdminClient({"bootstrap.servers": brokers})
    from confluent_kafka import ConsumerGroupTopicPartitions

    res = admin.list_consumer_group_offsets([ConsumerGroupTopicPartitions(group)])[group].result(15)
    c = Consumer({"bootstrap.servers": brokers, "group.id": f"e2e-lag-{time.time()}"})
    try:
        lag = 0
        parts = c.list_topics(topic, timeout=10).topics[topic].partitions
        committed = {tp.partition: tp.offset for tp in res.topic_partitions if tp.topic == topic}
        for p in parts:
            _lo, hi = c.get_watermark_offsets(TopicPartition(topic, p), timeout=10)
            lag += max(0, hi - max(committed.get(p, 0), 0))
        return lag
    finally:
        c.close()


def wait_mapping_caught_up(timeout_s: float = 60.0) -> None:
    """Wait until the relay has consumed everything produced so far on analytics.events.

    Falls back to the fixed settle delay if the group cannot be inspected.
    """
    group = os.getenv("GA4_RELAY_GROUP", "team-analytics.ga4-relay")
    topic = get_settings().kafka_analytics_topic
    deadline = time.monotonic() + timeout_s
    time.sleep(1.0)  # let the fetch/commit cycle see the new records
    while time.monotonic() < deadline:
        try:
            if group_lag(group, topic) == 0:
                return
        except Exception:  # noqa: BLE001 - group unknown / admin API unavailable
            time.sleep(settle_s())
            return
        time.sleep(1.0)
    raise AssertionError(f"relay group {group} still lags on {topic} after {timeout_s:.0f}s")
