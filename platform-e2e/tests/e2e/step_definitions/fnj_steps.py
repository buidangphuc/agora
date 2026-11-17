"""Steps for forecast-nightly-job (capability demand-forecast-job).

Black box: the job runs as the real platform-forecast image on the stack network (fnj_job_flow); its
output is read from the stack Redis DB 3 and from the process (exit code, stdout, stderr). Controlled
histories are synthetic `order_facts.parquet` files built inside the image; the label-honesty scenario
uses the real analytics export after one paid order (shared with the ofb scenarios). The compose and
CronJob scenarios are static (`docker compose config/run`, `kubectl kustomize`).

Every scenario deletes `fc:v1:meta` before the run so a present meta can only come from that run, and
removes the listing keys it created. Scenarios that read or assert the shared `fc:v1:meta` must not run
in parallel with each other.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import uuid
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import pytest
import yaml
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import fnj_job_flow as job
from tests.e2e.step_definitions import ofb_steps as ofb

ROOT = Path(__file__).resolve().parents[3].parent
GITOPS = ROOT / "platform-gitops"
LGBM = "lgbm_quantile_v1"
BASELINE = "seasonal_naive_v1"
AS_OF_DAY = date(2026, 11, 10)
TTL_LOW, TTL_HIGH = 172700, 172800


@dataclass
class Ctx:
    lines: list = field(default_factory=list)
    names: dict = field(default_factory=dict)
    seller: str = field(default_factory=lambda: f"fnj-s-{uuid.uuid4().hex[:10]}")
    inputs: Path | None = None
    result: job.Result | None = None
    keys_before: list = field(default_factory=list)
    created: set = field(default_factory=set)
    data: dict = field(default_factory=dict)


@pytest.fixture
def fnj():
    c = Ctx()
    yield c
    if c.inputs is not None:
        job.drop(c.inputs)
    if c.created:
        try:
            r = job.Redis()
        except OSError:
            r = None
        if r is not None:
            try:
                r.delete(*c.created)
            finally:
                r.close()
    shutil.rmtree(c.data.get("tmp", ""), ignore_errors=True)
    if c.data.get("compose_project"):
        _compose(c.data["compose_project"], "down", "-v", "--remove-orphans")


def _listing(c: Ctx, name: str) -> str:
    if name not in c.names:
        c.names[name] = f"fnj-l-{uuid.uuid4().hex[:8]}-{name}"
    return c.names[name]


def _key(c: Ctx, name: str) -> str:
    return f"fc:v1:seller:{c.seller}:{_listing(c, name)}"


def _paid_days(c: Ctx, name: str, days: int) -> None:
    lid = _listing(c, name)
    for k in range(1, days + 1):
        day = AS_OF_DAY - timedelta(days=k)
        c.lines.append({"seller": c.seller, "listing": lid, "day": day.isoformat(), "qty": 1 + k % 4})
    c.created.add(f"fc:v1:seller:{c.seller}:{lid}")


def _redis_get(key: str) -> str | None:
    r = job.Redis()
    try:
        return r.get(key)
    finally:
        r.close()


def _payload(key: str) -> dict:
    raw = _redis_get(key)
    assert raw is not None, f"{key} is not in Redis DB {job.REDIS_DB}"
    return json.loads(raw)


def _run(c: Ctx, as_of: str, *, from_stack: bool = False) -> None:
    if c.inputs is None and not from_stack:
        c.inputs = job.make_export(c.lines)
    r = job.Redis()
    try:
        r.delete(job.META_KEY)
        c.keys_before = r.keys("fc:v1:*")
    finally:
        r.close()
    c.result = job.run_job(as_of=as_of, inputs=None if from_stack else c.inputs)


def _assert_exit(c: Ctx, code: int) -> None:
    assert c.result is not None, "the job did not run"
    assert c.result.exit_code == code, (
        f"exit {c.result.exit_code}, wanted {code}\nstdout: {c.result.stdout[-1500:]}\n"
        f"stderr: {c.result.stderr[-1500:]}"
    )


def _no_new_keys(c: Ctx) -> None:
    r = job.Redis()
    try:
        after = r.keys("fc:v1:*")
    finally:
        r.close()
    assert after == c.keys_before, f"keys changed: {sorted(set(after) ^ set(c.keys_before))}"
    assert job.META_KEY not in after, "fc:v1:meta was written"


# -- Given ------------------------------------------------------------------------------------
@given("an input directory without order_facts.parquet")
def no_export_file(fnj):
    fnj.inputs = job.make_export([], file=False)


@given("an export with zero rows")
def zero_rows(fnj):
    fnj.inputs = job.make_export([])


@given(parsers.parse("a listing with {days:d} days of paid lines before 2026-11-10"))
def listing_with_history(fnj, days):
    _paid_days(fnj, "L", days)


@given(
    parsers.parse(
        'a listing "X" with only an unpaid line and a PAID line on the as-of day, and a listing "Y" '
        "with {days:d} days of paid lines"
    )
)
def listing_x_and_y(fnj, days):
    x = _listing(fnj, "X")
    fnj.lines.append({"seller": fnj.seller, "listing": x, "day": "2026-11-09", "qty": 2, "status": "CANCELLED"})
    fnj.lines.append({"seller": fnj.seller, "listing": x, "day": "2026-11-10", "qty": 2})
    _paid_days(fnj, "Y", days)


@given(
    parsers.parse(
        'a listing "C" with {short:d} days of paid lines and a listing "Y" with {long:d} days of paid lines'
    )
)
def listing_c_and_y(fnj, short, long):
    _paid_days(fnj, "C", short)
    _paid_days(fnj, "Y", long)


@given(
    parsers.parse(
        "3 listings of which 1 has {short:d} days of paid lines and 2 have {long:d} days"
    )
)
def three_listings(fnj, short, long):
    _paid_days(fnj, "A", long)
    _paid_days(fnj, "B", long)
    _paid_days(fnj, "C", short)


@given("a paid order has reached the analytics export")
def paid_order_exported(fnj):
    fnj.data["shared"] = ofb._wait_export(ofb._shared())


# -- When -------------------------------------------------------------------------------------
@when(parsers.parse("the forecast job runs with AS_OF {as_of}"))
def run_job_as_of(fnj, as_of):
    _run(fnj, as_of)


@when("the forecast job runs over the analytics export with AS_OF tomorrow 02:45 UTC")
def run_job_stack(fnj):
    tomorrow = datetime.now(timezone.utc).date() + timedelta(days=1)
    _run(fnj, f"{tomorrow.isoformat()}T02:45:00Z", from_stack=True)


# -- Then: exit codes and writes --------------------------------------------------------------
@then(parsers.parse("the job exits with code {code:d}"))
def exits(fnj, code):
    _assert_exit(fnj, code)


@then(parsers.parse("the job exits with code 2 and its stderr names order_facts.parquet"))
def exits_2(fnj):
    _assert_exit(fnj, 2)
    assert "order_facts.parquet" in fnj.result.stderr, fnj.result.stderr[-1500:]


@then("the job wrote no fc:v1 key and no fc:v1:meta")
def wrote_nothing(fnj):
    _no_new_keys(fnj)


@then(parsers.parse('the job exits with code 0 and its stdout status is "{status}"'))
def exits_0_status(fnj, status):
    _assert_exit(fnj, 0)
    summary = json.loads(fnj.result.stdout.strip().splitlines()[-1])
    assert summary["status"] == status, summary


# -- Then: listing keys -----------------------------------------------------------------------
@then(
    "Redis holds the listing key with 14 daily forecasts dated 2026-11-10 to 2026-11-23, each with "
    "0 <= p10 <= p50 <= p90"
)
def key_has_14_days(fnj):
    p = _payload(_key(fnj, "L"))
    assert p["seller_id"] == fnj.seller and p["listing_id"] == _listing(fnj, "L"), p
    assert p["model_version"] in (LGBM, BASELINE), p
    assert p["generated_at"], p
    days = p["daily_forecasts"]
    assert [d["date"] for d in days] == [
        (AS_OF_DAY + timedelta(days=i)).isoformat() for i in range(14)
    ], days
    for d in days:
        assert 0 <= d["p10"] <= d["p50"] <= d["p90"], d


@then("the listing key expires in 48 hours")
def key_ttl(fnj):
    r = job.Redis()
    try:
        ttl = r.ttl(_key(fnj, "L"))
    finally:
        r.close()
    assert TTL_LOW <= ttl <= TTL_HIGH, ttl


@then(parsers.parse('Redis holds no fc:v1:seller key for "{name}"'))
def no_key_for(fnj, name):
    r = job.Redis()
    try:
        found = r.keys(f"fc:v1:seller:*:{_listing(fnj, name)}")
    finally:
        r.close()
    assert found == [], found


@then(parsers.parse('Redis holds a key for "{name}"'))
def holds_key_for(fnj, name):
    _assert_exit(fnj, 0)
    p = _payload(_key(fnj, name))
    assert len(p["daily_forecasts"]) == 14, p


@then(
    parsers.parse(
        'the key of "{name}" has is_cold_start true, model_version seasonal_naive_v1 and 14 daily points'
    )
)
def cold_start_key(fnj, name):
    _assert_exit(fnj, 0)
    p = _payload(_key(fnj, name))
    assert p["is_cold_start"] is True, p
    assert p["model_version"] == BASELINE, p
    assert len(p["daily_forecasts"]) == 14, p


@then(parsers.parse('the key of "{name}" has is_cold_start false'))
def warm_key(fnj, name):
    assert _payload(_key(fnj, name))["is_cold_start"] is False


# -- Then: summary ----------------------------------------------------------------------------
@then(
    "fc:v1:meta has listings 3, cold_starts 1 and status ok, and its TTL is 48 hours"
)
def meta_counts(fnj):
    _assert_exit(fnj, 0)
    meta = _payload(job.META_KEY)
    assert meta["listings"] == 3 and meta["cold_starts"] == 1 and meta["status"] == "ok", meta
    assert meta["horizon_days"] == 14, meta
    assert meta["model_version"] in (LGBM, BASELINE), meta
    r = job.Redis()
    try:
        ttl = r.ttl(job.META_KEY)
    finally:
        r.close()
    assert TTL_LOW <= ttl <= TTL_HIGH, ttl
    fnj.created.add(job.META_KEY)


@then("the stdout of the job is one JSON line equal to fc:v1:meta")
def stdout_equals_meta(fnj):
    lines = [ln for ln in fnj.result.stdout.splitlines() if ln.strip()]
    assert len(lines) == 1, fnj.result.stdout[-1500:]
    assert json.loads(lines[0]) == _payload(job.META_KEY)


# -- Then: label honesty (real analytics export) ----------------------------------------------
@then("fc:v1:meta has status ok and the paid order's listings each have a key")
def stack_meta_and_keys(fnj):
    _assert_exit(fnj, 0)
    meta = _payload(job.META_KEY)
    assert meta["status"] == "ok", meta
    r = job.Redis()
    try:
        for lid in fnj.data["shared"].listings:
            found = r.keys(f"fc:v1:seller:*:{lid}")
            assert len(found) == 1, (lid, found)
    finally:
        r.close()


@then(
    "every fc:v1 payload carries lgbm_quantile_v1 only if the meta wape is lower than baseline_wape"
)
def label_never_overstates(fnj):
    meta = _payload(job.META_KEY)
    r = job.Redis()
    try:
        keys = r.keys("fc:v1:seller:*")
    finally:
        r.close()
    assert keys, "no listing payload to check"
    labels = {meta["model_version"]}
    for k in keys:
        labels.add(_payload(k)["model_version"])
    assert labels <= {LGBM, BASELINE}, labels
    if LGBM in labels:
        assert meta["wape"] is not None and meta["baseline_wape"] is not None, meta
        assert meta["wape"] < meta["baseline_wape"], meta


# -- compose (FNJ-18) -------------------------------------------------------------------------
def _compose(project: str, *args: str, timeout: int = 1800) -> subprocess.CompletedProcess:
    if shutil.which("docker") is None:
        pytest.fail("docker is not installed")
    cmd = ["docker", "compose", "-p", project, "-f", "docker-compose.yaml", *args]
    return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, timeout=timeout, check=False)


@when("the compose service forecast-job of profile forecast is rendered and started")
def compose_render(fnj):
    project = f"fnj-e2e-{uuid.uuid4().hex[:6]}"
    fnj.data["compose_project"] = project
    res = _compose(project, "--profile", "forecast", "config", "--format", "json")
    assert res.returncode == 0, f"docker compose config: {res.stderr[:500]}"
    services = json.loads(res.stdout)["services"]
    assert "forecast-job" in services, f"forecast-job is not defined: {sorted(services)[:40]}"
    fnj.data["service"] = services["forecast-job"]


@then("the service mounts analytics_data read-only at /data and writes Redis DB 3")
def compose_config(fnj):
    svc = fnj.data["service"]
    assert svc.get("profiles") == ["forecast"], svc.get("profiles")
    mounts = [v for v in svc.get("volumes", []) if v.get("target") == "/data"]
    assert len(mounts) == 1, svc.get("volumes")
    assert mounts[0]["source"] == "analytics_data" and mounts[0].get("read_only") is True, mounts
    env = svc.get("environment") or {}
    if isinstance(env, list):
        env = dict(e.split("=", 1) for e in env)
    assert env.get("REDIS_URL", "").rstrip("/").endswith("/3"), env.get("REDIS_URL")


@then("the container user id is 1001 and the export mount rejects a write")
def compose_runtime(fnj):
    project = fnj.data["compose_project"]
    base = ["--profile", "forecast", "run", "--rm", "--no-deps"]
    uid = _compose(project, *base, "--entrypoint", "id", "forecast-job", "-u")
    assert uid.returncode == 0, f"id -u failed: {uid.stderr[-500:]}"
    assert uid.stdout.strip().splitlines()[-1] == "1001", uid.stdout
    write = _compose(project, *base, "--entrypoint", "sh", "forecast-job", "-c", "touch /data/probe")
    assert write.returncode != 0, "the export mount accepted a write"
    assert "read-only" in (write.stderr + write.stdout).lower(), write.stderr[-500:]


# -- CronJob (FNJ-19) -------------------------------------------------------------------------
@when("the platform-gitops forecast manifests are rendered")
def render_gitops(fnj):
    if shutil.which("kubectl") is None:
        pytest.fail("kubectl is not installed")
    assert (GITOPS / "platform" / "forecast").is_dir(), "platform-gitops/platform/forecast does not exist"
    res = subprocess.run(
        ["kubectl", "kustomize", "platform/forecast"],
        cwd=GITOPS, capture_output=True, text=True, timeout=120, check=False,
    )
    assert res.returncode == 0, f"kustomize render failed: {res.stderr[:400]}"
    fnj.data["docs"] = [d for d in yaml.safe_load_all(res.stdout) if d]


def _one(docs: list[dict], kind: str) -> dict:
    found = [d for d in docs if d["kind"] == kind]
    assert len(found) == 1, f"expected exactly one {kind}, rendered {len(found)}"
    return found[0]


@then(
    'the CronJob schedule is "45 2 * * *", concurrencyPolicy is Forbid, the pod is runAsNonRoot with a '
    "read-only root filesystem and the analytics-data volume is readOnly"
)
def cronjob_checks(fnj):
    cron = _one(fnj.data["docs"], "CronJob")
    assert cron["spec"]["schedule"] == "45 2 * * *"
    assert cron["spec"]["concurrencyPolicy"] == "Forbid"
    pod = cron["spec"]["jobTemplate"]["spec"]["template"]["spec"]
    containers = pod["containers"]
    assert containers, "no container"
    for c in containers:
        sec = c.get("securityContext") or {}
        non_root = sec.get("runAsNonRoot", (pod.get("securityContext") or {}).get("runAsNonRoot"))
        assert non_root is True, f"{c['name']}: runAsNonRoot {non_root}"
        assert sec.get("readOnlyRootFilesystem") is True, f"{c['name']}: no readOnlyRootFilesystem"
        mine = [m for m in c.get("volumeMounts", []) if m["name"] == "analytics-data"]
        assert mine and all(m.get("readOnly") is True for m in mine), f"{c['name']}: {mine}"
    vols = [v for v in pod.get("volumes", []) if v["name"] == "analytics-data"]
    assert len(vols) == 1, pod.get("volumes")
    assert vols[0]["persistentVolumeClaim"].get("readOnly") is True, vols[0]


@then("the NetworkPolicy has no ingress and egress only to DNS and valkey port 6379")
def networkpolicy_checks(fnj):
    pol = _one(fnj.data["docs"], "NetworkPolicy")["spec"]
    assert not pol.get("ingress"), "NetworkPolicy must have no ingress rule"
    assert "Ingress" in pol["policyTypes"] and "Egress" in pol["policyTypes"], pol["policyTypes"]
    kinds = []
    for rule in pol.get("egress", []):
        ports = {(p.get("protocol", "TCP"), p.get("port")) for p in rule.get("ports", [])}
        labels = {
            f"{k}={v}"
            for peer in rule.get("to", [])
            for k, v in (peer.get("podSelector", {}).get("matchLabels") or {}).items()
        }
        if ports and {p for _, p in ports} == {53}:
            kinds.append("dns")
        elif ports == {("TCP", 6379)} and "app=valkey" in labels:
            kinds.append("valkey")
        else:
            raise AssertionError(f"unexpected egress rule: {rule}")
    assert sorted(kinds) == ["dns", "valkey"], kinds
