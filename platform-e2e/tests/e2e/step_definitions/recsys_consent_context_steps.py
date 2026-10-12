"""Steps for recsys-consent-and-context (capability recommendations).

Black box, nothing mocked and nothing in-process:
  * dataset scenarios run the real platform-featurestore job image (`dataset`) on synthetic warehouse
    inputs written by a throw-away driver run inside that image, and read the v2 datasets back;
  * nearline scenarios produce protobuf EventEnvelope/TrackingEvent messages (hand-encoded from the public
    proto, field numbers per platform-core) onto a scratch Kafka topic, drain it with the real recsys image
    (`python -m recsys.nearline`, drain mode) and read the stack's Redis through redis-cli;
  * the export scenario posts beacons through the gateway and reads the analytics volume.
Environment: KAFKA_BROKERS (host side, e.g. localhost:19092), STACK_NETWORK (e.g. agora_default),
REDIS_CONTAINER (default agora-redis-1), RECSYS_IMAGE, FEATURESTORE_IMAGE.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
import time
import uuid
from datetime import datetime, timedelta
from pathlib import Path

import pytest
from pytest_bdd import given, then, when

from config.settings import get_settings
from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.flows import rgd_flow as rgd
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

_TIMEOUT_S = 600
EXPORT_WAIT_S = float(os.getenv("RGD_EXPORT_WAIT_S", "390"))

# ── in-image drivers (run inside platform-featurestore, which has duckdb + pyarrow) ───────────────
_MKINPUTS = r"""
import json, os
from datetime import datetime
import pyarrow as pa, pyarrow.parquet as pq
plan = json.load(open("/work/plan.json"))
S = pa.schema([("event_id", pa.string()), ("event_type", pa.string()), ("listing_id", pa.string()),
    ("occurred_at", pa.timestamp("us")), ("ingested_at", pa.timestamp("us")), ("principal_type", pa.string()),
    ("user_key", pa.string()), ("position", pa.int32()), ("impression_id", pa.string()),
    ("consent_personalization", pa.bool_()), ("consent_version", pa.int32())])
F = pa.schema([("event_id", pa.string()), ("fact", pa.string()), ("user_id", pa.string()), ("listing_id", pa.string()),
    ("seller_id", pa.string()), ("rating", pa.int32()), ("occurred_at", pa.timestamp("us")), ("ingested_at", pa.timestamp("us"))])
O = pa.schema([("event_id", pa.string()), ("order_id", pa.string()), ("listing_id", pa.string()), ("variant_id", pa.string()),
    ("seller_id", pa.string()), ("quantity", pa.int32()), ("unit_price", pa.int64()), ("currency", pa.string()),
    ("occurred_at", pa.timestamp("us")), ("status", pa.string()), ("buyer_id", pa.string())])
os.makedirs("/work/in", exist_ok=True)
rows = []
for i, e in enumerate(plan["events"]):
    at = datetime.fromisoformat(e["at"])
    rows.append({"event_id": "rcc-%d" % i, "event_type": "view", "listing_id": e["listing"], "occurred_at": at,
        "ingested_at": at, "principal_type": "USER", "user_key": e["user"], "position": None, "impression_id": None,
        "consent_personalization": e["granted"], "consent_version": 1})
pq.write_table(pa.Table.from_pylist(rows, schema=S), "/work/in/tracking_events_resolved.parquet")
pq.write_table(pa.Table.from_pylist([], schema=F), "/work/in/engagement_facts.parquet")
pq.write_table(pa.Table.from_pylist([], schema=O), "/work/in/order_facts.parquet")
print(json.dumps({"written": len(rows)}))
"""

_ROWS = r"""
import glob, json
import pyarrow.parquet as pq
plan = json.load(open("/work/plan.json"))
paths = sorted(glob.glob("/features/datasets/%s/%s/as_of=*.parquet" % (plan["dataset"], plan["version"])))
if not paths:
    print(json.dumps({"found": False, "rows": []}))
else:
    wanted = set(plan["users"])
    rows = [r for r in pq.read_table(paths[-1]).to_pylist() if r["user_key"] in wanted]
    print(json.dumps({"found": True, "rows": rows}, default=str))
"""

_EXPORT = r"""
import json
import duckdb
plan = json.load(open("/work/plan.json"))
src = "read_parquet('/data/tracking_events.parquet')"
cols = [c[0] for c in duckdb.execute("DESCRIBE SELECT * FROM " + src).fetchall()]
cur = duckdb.execute("SELECT * FROM " + src + " WHERE ga_client_id = ?", [plan["ga_client_id"]])
names = [d[0] for d in cur.description]
rows = [dict(zip(names, r)) for r in cur.fetchall()]
print(json.dumps({"columns": cols, "rows": rows}, default=str))
"""


def _in_image(script: str, plan: dict, mounts: list[str], *, rw: bool = False) -> tuple[dict, Path]:
    work = Path(tempfile.mkdtemp(prefix="rcc-drv-"))
    (work / "driver.py").write_text(script)
    (work / "plan.json").write_text(json.dumps(plan))
    work.chmod(0o777)
    args = ["docker", "run", "--rm", "-v", f"{work}:/work{'' if rw else ':ro'}", *mounts]
    args += ["--entrypoint", "python", fsm.image(), "/work/driver.py"]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    assert proc.returncode == 0, f"driver failed:\n{proc.stderr[-2000:]}"
    return json.loads(proc.stdout.strip().splitlines()[-1]), work


@pytest.fixture
def rcc():
    ctx = {"offline": fsm.new_offline_dir(), "work": [], "cleanup": []}
    yield ctx
    fsm.drop_offline_dir(ctx["offline"])
    for p in ctx["work"]:
        shutil.rmtree(p, ignore_errors=True)
    for fn in ctx["cleanup"]:
        fn()


def _naive(dt: datetime) -> str:
    return dt.strftime("%Y-%m-%dT%H:%M:%S")


# ── Governed datasets ─────────────────────────────────────────────────────────────────────────────
def _events(user: str, listing: str, n: int, at: datetime, granted: bool) -> list[dict]:
    return [
        {"user": user, "listing": f"{listing}-{i}", "at": _naive(at), "granted": granted}
        for i in range(n)
    ]


@given(
    "a buyer u-deny with 10 view events all with consent_personalization false and a buyer u-ok with 3 with true"
)
def deny_and_ok(rcc):
    tag = uuid.uuid4().hex[:8]
    now = datetime.utcnow().replace(microsecond=0)
    rcc["users"] = {"deny": f"u-deny-{tag}", "ok": f"u-ok-{tag}"}
    rcc["events"] = _events(
        rcc["users"]["deny"], f"l-deny-{tag}", 10, now - timedelta(hours=3), False
    )
    rcc["events"] += _events(rcc["users"]["ok"], f"l-ok-{tag}", 3, now - timedelta(hours=2), True)


@given("a buyer u-late with 4 denied events at T1 and 2 granted events at T2 later than T1")
def late_grant(rcc):
    tag = uuid.uuid4().hex[:8]
    now = datetime.utcnow().replace(microsecond=0)
    rcc["t2"] = now - timedelta(hours=2)
    rcc["users"] = {"late": f"u-late-{tag}"}
    listing = f"l-late-{tag}"
    # Same listing on both sides so the collapsed (user, item) row is the one the grant boundary trims.
    rcc["events"] = [
        {
            "user": rcc["users"]["late"],
            "listing": listing,
            "at": _naive(now - timedelta(hours=5)),
            "granted": False,
        }
        for _ in range(4)
    ] + [
        {"user": rcc["users"]["late"], "listing": listing, "at": _naive(rcc["t2"]), "granted": True}
        for _ in range(2)
    ]


@when("the governed datasets are built")
def build_datasets(rcc):
    out, work = _in_image(_MKINPUTS, {"events": rcc["events"]}, [], rw=True)
    rcc["work"].append(work)
    assert out["written"] == len(rcc["events"])
    inputs = work / "in"
    inputs.chmod(0o755)
    run = rgd.run_dataset(rcc["offline"], inputs=inputs)
    assert run.exit_code == 0, f"dataset job exited {run.exit_code}:\n{run.output[-3000:]}"
    rows, w2 = _in_image(
        _ROWS,
        {"dataset": "als_interactions", "version": "v2", "users": list(rcc["users"].values())},
        ["-v", f"{rcc['offline']}:/features:ro"],
    )
    rcc["work"].append(w2)
    assert rows[
        "found"
    ], "als_interactions v2 was not built (no datasets/als_interactions/v2 snapshot)"
    rcc["rows"] = rows["rows"]


@then("als_interactions has 0 rows for u-deny and at least 1 row for u-ok")
def deny_ok_rows(rcc):
    by_user: dict[str, list] = {}
    for r in rcc["rows"]:
        by_user.setdefault(r["user_key"], []).append(r)
    assert by_user.get(rcc["users"]["deny"], []) == [], by_user
    assert len(by_user.get(rcc["users"]["ok"], [])) >= 1, by_user


@then("the als_interactions row for u-late counts interactions 2 and its last_occurred_at is T2")
def late_row(rcc):
    rows = [r for r in rcc["rows"] if r["user_key"] == rcc["users"]["late"]]
    assert len(rows) == 1, rows
    assert int(rows[0]["interactions"]) == 2, rows
    got = datetime.fromisoformat(str(rows[0]["last_occurred_at"]).replace("Z", "")[:19])
    assert got == rcc["t2"], (got, rcc["t2"])


# ── Recommend ─────────────────────────────────────────────────────────────────────────────────────
@when(
    "the signed-in buyer with no granted events calls Recommend for the home feed through the gateway"
)
def denied_buyer_recommend(world: World):
    world.state.extra["rcc_response"] = rss.recommend(world)


@then("the Recommend response lists at least 1 listing id")
def has_listing_ids(world: World):
    got = [i for i in rss.ids(world.state.extra["rcc_response"]) if i]
    assert len(got) >= 1, world.state.extra["rcc_response"]


# ── Nearline ──────────────────────────────────────────────────────────────────────────────────────
_VIEW, _BEGIN_CHECKOUT, _VIEW_CART = 1, 6, 11


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        out.append(b | (0x80 if n else 0))
        if not n:
            return bytes(out)


def _f_varint(num: int, v: int) -> bytes:
    return _varint(num << 3) + _varint(v) if v else b""


def _f_bytes(num: int, v: bytes) -> bytes:
    return _varint(num << 3 | 2) + _varint(len(v)) + v if v else b""


def _f_str(num: int, v: str) -> bytes:
    return _f_bytes(num, v.encode())


def _envelope(
    actor: str, etype: int, listing: str, session: str, category: str, personalization: bool | None
) -> bytes:
    """EventEnvelope{event_id=1,type=2,occurred_at=3,principal=4,payload=7} around a TrackingEvent.

    personalization None = no consent field (26) at all; otherwise ConsentState{personalization=3, version=4}.
    """
    consent = (
        b"" if personalization is None else (_f_varint(3, int(personalization)) + _f_varint(4, 1))
    )
    event = (
        _f_varint(1, etype)
        + _f_str(2, listing)
        + _f_str(3, session)
        + _f_str(19, category)
        + (_f_bytes(26, consent) if personalization is not None else b"")
    )
    now = time.time()
    ts = _f_varint(1, int(now)) + _f_varint(2, int((now % 1) * 1e9))
    principal = _f_str(1, actor) + _f_varint(2, 2)  # PRINCIPAL_TYPE_USER
    return (
        _f_str(1, str(uuid.uuid4()))
        + _f_str(2, "platform.analytics.v1.TrackingEvent")
        + _f_bytes(3, ts)
        + _f_bytes(4, principal)
        + _f_bytes(7, event)
    )


def _redis(*args: str) -> str:
    name = os.getenv("REDIS_CONTAINER", "agora-redis-1")
    proc = subprocess.run(
        ["docker", "exec", name, "redis-cli", *args],
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )
    assert proc.returncode == 0, proc.stderr
    return proc.stdout.strip()


def _scan(pattern: str) -> list[str]:
    out = _redis("--scan", "--pattern", pattern)
    return [k for k in out.splitlines() if k]


def _produce(rcc, messages: list[bytes]) -> str:
    from confluent_kafka import Producer
    from confluent_kafka.admin import AdminClient, NewTopic

    brokers = get_settings().kafka_brokers
    topic = f"e2e.recsys-consent.{uuid.uuid4().hex[:10]}"
    admin = AdminClient({"bootstrap.servers": brokers})
    for fut in admin.create_topics(
        [NewTopic(topic, num_partitions=1, replication_factor=1)]
    ).values():
        fut.result(timeout=30)
    prod = Producer({"bootstrap.servers": brokers})
    for m in messages:
        prod.produce(topic, value=m, key=b"k")
    assert prod.flush(30) == 0, "messages not delivered to the scratch topic"
    rcc["cleanup"].append(lambda: admin.delete_topics([topic]))
    return topic


def _drain(rcc, topic: str) -> str:
    """Run `python -m recsys.nearline` until the scratch topic has been idle for 10 s; return its log."""
    s = get_settings()
    brokers_in_net = os.getenv("NEARLINE_E2E_BROKERS", "redpanda:9092")
    args = ["docker", "run", "--rm", "--network", s.stack_network]
    env = {
        "KAFKA_BROKERS": brokers_in_net,
        "KAFKA_ANALYTICS_TOPIC": topic,
        "NEARLINE_CONSUMER_GROUP": f"e2e-rcc-{uuid.uuid4().hex[:8]}",
        "NEARLINE_START_OFFSET": "earliest",
        "NEARLINE_IDLE_EXIT_SECONDS": "10",
        "REDIS_HOST": "redis",
        "REDIS_PORT": "6379",
    }
    for k, v in env.items():
        args += ["-e", f"{k}={v}"]
    args += ["--entrypoint", "python", s.recsys_image, "-m", "recsys.nearline"]
    proc = subprocess.run(args, capture_output=True, text=True, timeout=_TIMEOUT_S, check=False)
    log = proc.stdout + proc.stderr
    assert proc.returncode == 0, f"nearline consumer exited {proc.returncode}:\n{log[-3000:]}"
    return log


def _cleanup_actor_keys(*needles: str):
    def run() -> None:
        for n in needles:
            keys = _scan(f"recs:nearline:*{n}*")
            if keys:
                _redis("DEL", *keys)

    return run


@given(
    "a nearline topic holding a denied view of actor u-deny and then a granted view of actor u-ok"
)
def denied_then_granted(rcc):
    tag = uuid.uuid4().hex[:8]
    rcc["deny"], rcc["ok"] = f"u-deny-{tag}", f"u-ok-{tag}"
    rcc["l_deny"], rcc["l_ok"] = f"l-deny-{tag}", f"l-ok-{tag}"
    rcc["s_deny"], rcc["s_ok"] = f"s-deny-{tag}", f"s-ok-{tag}"
    rcc["cleanup"].append(_cleanup_actor_keys(tag))
    rcc["topic"] = _produce(
        rcc,
        [
            _envelope(rcc["deny"], _VIEW, rcc["l_deny"], rcc["s_deny"], "books", False),
            _envelope(rcc["ok"], _VIEW, rcc["l_ok"], rcc["s_ok"], "books", True),
        ],
    )


@given(
    "a nearline topic holding a view_cart then a begin_checkout of a books listing from a consented actor"
)
def cart_then_checkout(rcc):
    tag = uuid.uuid4().hex[:8]
    rcc["actor"], listing, session = f"u-co-{tag}", f"l-co-{tag}", f"s-co-{tag}"
    rcc["cleanup"].append(_cleanup_actor_keys(tag))
    rcc["topic"] = _produce(
        rcc,
        [
            _envelope(rcc["actor"], _VIEW_CART, listing, session, "books", True),
            _envelope(rcc["actor"], _BEGIN_CHECKOUT, listing, session, "books", True),
        ],
    )


@when("the nearline consumer drains the topic")
def drain(rcc):
    rcc["log"] = _drain(rcc, rcc["topic"])


@then("no recs:nearline:user key, session key or co-view key exists for the denied view")
def denied_keys_absent(rcc):
    # Positive control first: the consumer demonstrably ran and wrote for the granted actor.
    assert _scan(
        f"recs:nearline:user:{rcc['ok']}:*"
    ), f"consumer wrote nothing for the granted actor:\n{rcc['log'][-1500:]}"
    leaked = (
        _scan(f"recs:nearline:user:{rcc['deny']}:*")
        + _scan(f"recs:nearline:session:{rcc['s_deny']}:*")
        + _scan(f"recs:nearline:coview:{rcc['l_deny']}")
        + _scan(f"recs:nearline:ctr:{rcc['l_deny']}")
    )
    assert leaked == [], f"a denied event wrote {leaked}"


@then("the consumer reports consent_denied 1")
def consent_denied_counted(rcc):
    # The spec fixes the counter name, not where it is reported: it is read from the consumer's output.
    assert re.search(r"consent_denied\W+1\b", rcc["log"]), rcc["log"][-1500:]


@then("the granted actor's recents list the viewed listing at rank 1")
def granted_recents(rcc):
    top = _redis("ZREVRANGE", f"recs:nearline:user:{rcc['ok']}:items", "0", "0")
    assert top == rcc["l_ok"], top


@then("the actor's category affinity for books is 9.0")
def books_affinity(rcc):
    raw = _redis("HGET", f"recs:nearline:user:{rcc['actor']}:cats", "books")
    assert raw, f"no books affinity written:\n{rcc['log'][-1500:]}"
    assert float(raw) == pytest.approx(9.0), raw


# ── Export ────────────────────────────────────────────────────────────────────────────────────────
_SEVEN = (
    "consent_personalization",
    "consent_version",
    "item_list_id",
    "position",
    "session_source",
    "session_medium",
    "session_campaign",
)


@when("the export cycle completes")
def export_completes(world: World):
    x = world.state.extra
    client = x["g4_clients"]["spring"]
    deadline = time.monotonic() + EXPORT_WAIT_S
    last: dict = {}
    while True:
        last, work = _in_image(
            _EXPORT, {"ga_client_id": client}, ["-v", f"{fsm.analytics_volume()}:/data:ro"]
        )
        shutil.rmtree(work, ignore_errors=True)
        if len(last["rows"]) >= 4:
            break
        assert (
            time.monotonic() < deadline
        ), f"the export never held the 4 spring views in {EXPORT_WAIT_S}s: {last}"
        time.sleep(10.0)
    x["rcc_export"] = last


@then("the exported tracking Parquet has the seven consent and context columns")
def seven_columns(world: World):
    cols = set(world.state.extra["rcc_export"]["columns"])
    missing = [c for c in _SEVEN if c not in cols]
    assert not missing, f"exported tracking Parquet lacks columns {missing}"


@then(
    "the spring visitor's exported rows carry the spring session source, medium and campaign and "
    "consent personalization false with version 1"
)
def spring_rows(world: World):
    x = world.state.extra
    rows = x["rcc_export"]["rows"]
    campaign = x["g4_campaigns"]["spring"]
    assert len(rows) >= 4, rows
    for r in rows:
        assert (r["session_source"], r["session_medium"], r["session_campaign"]) == (
            "google",
            "cpc",
            campaign,
        ), r
        assert r["consent_personalization"] is False and int(r["consent_version"]) == 1, r
