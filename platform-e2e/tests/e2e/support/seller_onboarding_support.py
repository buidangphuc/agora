"""E2E helper for identity-seller-onboarding (initiative p10-backend-gaps).

Black box through the gateway. Direct access outside the gateway is limited to reading the
user row's terms columns (identity_db) and reading `identity.events` from Kafka.

Env: SELLER_TERMS_VERSION (current terms version, default 2026-11-02), POSTGRES_CONTAINER,
KAFKA_BROKERS.
"""

from __future__ import annotations

import os
import subprocess
import time

import httpx

from tests.e2e.support import user_enforcement_support as ue

BECOME_SELLER = f"{ue.IDENTITY}.AuthService/BecomeSeller"
REVOKE = f"{ue.IDENTITY}.SessionService/RevokeSession"
CREATE_LISTING = "/platform.listing.v1.ListingService/CreateListing"
ROLE_GRANTED = "platform.identity.v1.UserRoleGranted"


def current_terms() -> str:
    return os.getenv("SELLER_TERMS_VERSION", "2026-11-02")


def become_seller(token: str | None, version: str | None = None) -> httpx.Response:
    return ue.call(BECOME_SELLER, {"termsVersion": current_terms() if version is None else version}, token)


def result_of(resp: httpx.Response) -> dict:
    data = resp.json()
    return data.get("result", data)


def code(resp: httpx.Response) -> str:
    return ue.code_of(resp).lower()


def new_buyer() -> dict:
    return ue.register_user("buyer", "so_buyer")


def roles_of(world, user_id: str) -> list[str]:
    return sorted(ue.stored_user(world, user_id).get("roles", []))


def scopes_of(token: str) -> list[str]:
    raw = ue.claims(token).get("scopes", [])
    return raw.split() if isinstance(raw, str) else list(raw)


def terms_row(user_id: str) -> tuple[str, str]:
    container = os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")
    sql = (
        "SELECT coalesce(seller_terms_version,''), "
        "coalesce(extract(epoch from seller_terms_accepted_at)::bigint::text,'') "
        f"FROM users WHERE id = '{user_id}';"
    )
    proc = subprocess.run(
        ["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", "identity_db",
         "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"],
        input=sql, capture_output=True, text=True, timeout=60,
    )
    assert proc.returncode == 0, f"identity_db read failed: {proc.stderr}"
    version, _, epoch = proc.stdout.strip().partition("|")
    return version, epoch


def create_listing(token: str) -> httpx.Response:
    body = {"listing": {"title": f"so e2e {time.time_ns()}", "categoryId": "cat-laptop", "price": 100000,
                        "stock": 1, "status": "LISTING_STATUS_PUBLISHED", "currency": "VND",
                        "description": "seller onboarding e2e"}}
    return ue.call(CREATE_LISTING, body, token)


class RoleGrantedTap:
    """Reads `identity.events` from creation on and decodes UserRoleGranted events."""

    TOPIC = "identity.events"

    def __init__(self) -> None:
        from confluent_kafka import Consumer, TopicPartition

        self._consumer = Consumer(
            {"bootstrap.servers": os.getenv("KAFKA_BROKERS", "localhost:19092"),
             "group.id": f"e2e-so-{time.time_ns()}", "enable.auto.commit": False}
        )
        meta = self._consumer.list_topics(self.TOPIC, timeout=15)
        assigns = []
        for p in meta.topics[self.TOPIC].partitions:
            _, high = self._consumer.get_watermark_offsets(TopicPartition(self.TOPIC, p), timeout=15)
            assigns.append(TopicPartition(self.TOPIC, p, high))
        self._consumer.assign(assigns)
        self.seen: list[dict] = []

    def events(self, user_id: str, wait_s: float, stop_at: int | None = None) -> list[dict]:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            if stop_at is not None and len([e for e in self.seen if e["user_id"] == user_id]) >= stop_at:
                break
            msg = self._consumer.poll(1.0)
            if msg is None or msg.error():
                continue
            if (msg.key() or b"").decode(errors="ignore") != user_id:
                continue
            env = ue._fields(msg.value())
            if env.get(2, [b""])[0].decode() != ROLE_GRANTED:
                continue
            payload = ue._fields(env[7][0])
            self.seen.append(
                {"user_id": payload.get(1, [b""])[0].decode(), "role": payload.get(2, [b""])[0].decode()}
            )
        return [e for e in self.seen if e["user_id"] == user_id]

    def close(self) -> None:
        self._consumer.close()
