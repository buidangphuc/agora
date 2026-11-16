"""Black-box helpers for KYC documents (change verification-kyc-documents, initiative trust-legal).

Everything goes through the gateway as a client would (CreateKycUploadUrl, presigned PUT/GET, SubmitKyc,
review, queue, badges). Three side channels exist only where the spec promises state outside the API:
the MinIO bucket (boto3, bucket content and encryption), verification_db (clock shifts for the 15 minute
grant expiry and the 90 day retention window) and the Kafka identity.events topic (UserErased) with its
team-verification DLQ.

Env: KYC_S3_ENDPOINT (default http://localhost:9000), KYC_S3_ACCESS_KEY / KYC_S3_SECRET_KEY (minioadmin),
KYC_BUCKET (kyc-documents), KYC_POSTGRES_CONTAINER (default agora-postgres-1), KYC_SWEEP_WAIT_S (150),
KYC_ERASURE_WAIT_S (90). Presigned URLs are signed for the service's in-network endpoint; the helpers reach
them on KYC_S3_ENDPOINT and keep the signed Host header.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import time
import uuid
from datetime import datetime, timezone
from urllib.parse import urlsplit, urlunsplit

import httpx

from config.settings import get_settings

BUCKET = os.getenv("KYC_BUCKET", "kyc-documents")
DLQ_TOPIC = "identity.events.team-verification.dlq"
IDENTITY_TOPIC = "identity.events"
USER_ERASED_TYPE = "platform.identity.v1.UserErased"
PASSWORD = "Sup3r-secret-pass!"
MIB = 1024 * 1024


def s3_endpoint() -> str:
    return os.getenv("KYC_S3_ENDPOINT", "http://localhost:9000")


def s3():
    import boto3
    from botocore.config import Config

    return boto3.client(
        "s3",
        endpoint_url=s3_endpoint(),
        aws_access_key_id=os.getenv("KYC_S3_ACCESS_KEY", "minioadmin"),
        aws_secret_access_key=os.getenv("KYC_S3_SECRET_KEY", "minioadmin"),
        region_name="us-east-1",
        config=Config(s3={"addressing_style": "path"}, retries={"max_attempts": 1}),
    )


def object_exists(key: str) -> bool:
    from botocore.exceptions import ClientError

    try:
        s3().head_object(Bucket=BUCKET, Key=key)
        return True
    except ClientError as exc:
        if exc.response["Error"]["Code"] in ("404", "NoSuchKey", "NotFound"):
            return False
        raise


def keys_under(prefix: str) -> list[str]:
    out = s3().list_objects_v2(Bucket=BUCKET, Prefix=prefix)
    return [o["Key"] for o in out.get("Contents", [])]


def reach(url: str) -> tuple[str, dict[str, str]]:
    """The presigned URL rewritten to the host-reachable endpoint, with the signed Host header kept."""
    parts = urlsplit(url)
    target = urlsplit(s3_endpoint())
    return urlunsplit((target.scheme, target.netloc, parts.path, parts.query, "")), {
        "Host": parts.netloc
    }


def http_put(url: str, body: bytes, content_type: str) -> httpx.Response:
    target, headers = reach(url)
    headers["Content-Type"] = content_type
    return httpx.put(target, content=body, headers=headers, timeout=60)


def http_get(url: str) -> httpx.Response:
    target, headers = reach(url)
    return httpx.get(target, headers=headers, timeout=60)


def jwt_sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


def parse_ts(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)


def seconds_ahead(value: str) -> float:
    return (parse_ts(value) - datetime.now(timezone.utc)).total_seconds()


def register(world, role: str = "seller") -> tuple[str, str]:
    username = f"e2e_kyc_{role}_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role=role)
    return token, jwt_sub(token)


def pg_container() -> str:
    return os.getenv("KYC_POSTGRES_CONTAINER", "agora-postgres-1")


def psql(sql: str) -> str:
    proc = subprocess.run(
        [
            "docker",
            "exec",
            "-i",
            pg_container(),
            "psql",
            "-U",
            "postgres",
            "-d",
            "verification_db",
            "-At",
            "-v",
            "ON_ERROR_STOP=1",
            "-c",
            sql,
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 0, f"verification_db query failed: {proc.stderr}"
    return proc.stdout.strip()


def lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def count(table: str, user_id: str) -> int:
    return int(psql(f"SELECT count(*) FROM {table} WHERE user_id = {lit(user_id)}"))


def upload_doc(
    svc, content: bytes | None = None, content_type: str = "image/jpeg"
) -> tuple[str, bytes]:
    """CreateKycUploadUrl then PUT the exact granted bytes; returns (doc_ref, bytes)."""
    body = content if content is not None else os.urandom(2048)
    resp = svc.create_kyc_upload_url_response(content_type, len(body))
    assert resp.status_code == 200, f"CreateKycUploadUrl failed: {resp.status_code} {resp.text}"
    grant = resp.json()
    put = http_put(grant["uploadUrl"], body, content_type)
    assert (
        200 <= put.status_code < 300
    ), f"PUT to the upload URL failed: {put.status_code} {put.text}"
    return grant["docRef"], body


def submit_with_uploads(
    svc, docs: int = 1, doc_type: str = "national_id"
) -> tuple[str, list[str], list[bytes]]:
    """Upload `docs` objects and SubmitKyc with them; returns (submission id, refs, bodies)."""
    refs: list[str] = []
    bodies: list[bytes] = []
    for _ in range(docs):
        ref, body = upload_doc(svc)
        refs.append(ref)
        bodies.append(body)
    resp = svc.submit_kyc_response(doc_type, refs[0], refs[1:])
    assert resp.status_code == 200, f"SubmitKyc failed: {resp.status_code} {resp.text}"
    return resp.json()["id"], refs, bodies


def poll(predicate, timeout_s: float, what: str, interval_s: float = 2.0):
    deadline = time.monotonic() + timeout_s
    while True:
        value = predicate()
        if value:
            return value
        if time.monotonic() >= deadline:
            raise AssertionError(f"timed out after {timeout_s:.0f}s waiting for {what}")
        time.sleep(interval_s)


def verification_container() -> str:
    return os.getenv("VERIFICATION_CONTAINER", "team-verification-svc")


def trigger_sweep() -> None:
    from tests.e2e.support import apr_support as apr
    from tests.e2e.support import pear_edge_support as pe

    apr.dc("restart", "team-verification")
    pe.wait_healthy(verification_container(), 120)


def sweep_wait_s() -> float:
    return float(os.getenv("KYC_SWEEP_WAIT_S", "150"))


def erasure_wait_s() -> float:
    return float(os.getenv("KYC_ERASURE_WAIT_S", "90"))


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def _ld(field: int, data: bytes) -> bytes:
    return _varint(field << 3 | 2) + _varint(len(data)) + data


def _timestamp(epoch_s: float) -> bytes:
    return _varint(1 << 3 | 0) + _varint(int(epoch_s))


def encode_user_erased(event_id: str, user_id: str, roles: list[str]) -> bytes:
    payload = _ld(1, user_id.encode())
    for role in roles:
        payload += _ld(2, role.encode())
    payload += _ld(3, _timestamp(time.time()))
    return (
        _ld(1, event_id.encode())
        + _ld(2, USER_ERASED_TYPE.encode())
        + _ld(3, _timestamp(time.time()))
        + _ld(7, payload)
    )


def publish_user_erased(
    user_id: str, *, event_id: str | None = None, roles: list[str] | None = None
) -> str:
    from confluent_kafka import Producer

    eid = event_id or str(uuid.uuid4())
    producer = Producer({"bootstrap.servers": get_settings().kafka_brokers})
    producer.produce(
        IDENTITY_TOPIC,
        key=user_id.encode(),
        value=encode_user_erased(eid, user_id, roles or ["seller"]),
    )
    if producer.flush(15) != 0:
        raise RuntimeError("UserErased was not delivered to identity.events")
    return eid


def dlq_mentions(user_id: str) -> bool:
    """True when any message on the team-verification DLQ carries the user id."""
    from confluent_kafka import Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-kyc-dlq-{uuid.uuid4().hex[:8]}",
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
        }
    )
    try:
        meta = consumer.list_topics(DLQ_TOPIC, timeout=15)
        if DLQ_TOPIC not in meta.topics or meta.topics[DLQ_TOPIC].error is not None:
            raise AssertionError(f"topic {DLQ_TOPIC} does not exist")
        partitions = list(meta.topics[DLQ_TOPIC].partitions)
        consumer.assign([TopicPartition(DLQ_TOPIC, p, 0) for p in partitions])
        needle = user_id.encode()
        idle = 0
        while idle < 3:
            msg = consumer.poll(1.0)
            if msg is None:
                idle += 1
                continue
            if msg.error():
                continue
            idle = 0
            if needle in (msg.value() or b"") or needle in (msg.key() or b""):
                return True
        return False
    finally:
        consumer.close()
