"""Steps for serve-trained-recs-locally (area tpr).

Black box. The Parquet export scenarios run the real team-analytics image as a throwaway container
on the stack network (own warehouse directory, own Kafka consumer groups) and observe the files it
writes the way a reader would. The serving scenarios read Recommend through the gateway and the
trained collection in the stack's Qdrant. The evaluation scenarios run the real platform-recsys image
(recsys_job_flow) and assert on what the job reports and leaves in the registry.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from config.settings import get_settings
from src.api.services.recommendation_service import CONTEXT_HOMEPAGE, CONTEXT_SIMILAR_ITEMS
from src.constants import PageName, timeouts
from tests.e2e.flows.recsys_job_flow import JobRun, run_recsys_job
from tests.e2e.step_definitions.pipeline_eval_registry_steps import _events
from tests.e2e.step_definitions.ssa_steps import ssa_point_id, ssa_qdrant
from tests.e2e.support.world import World

ANALYTICS_IMAGE = os.getenv("ANALYTICS_IMAGE", "agora-team-analytics:local")
EXPORT_FILE = "tracking_events.parquet"
_PARQUET_MAGIC = b"PAR1"
_PROTOCOL = "leave-last-new-item-v1"
_N_USERS = 4


# ── throwaway team-analytics ──────────────────────────────────────────────
def _start_analytics(world: World, env: dict[str, str]) -> tuple[str, Path]:
    """Run the real team-analytics image with its own warehouse dir; returns (container, dir)."""
    data = Path(tempfile.mkdtemp(prefix="tpr-analytics-"))
    data.chmod(0o777)  # the image runs as a non-root user
    name = f"e2e-tpr-analytics-{uuid.uuid4().hex[:10]}"
    group = f"e2e-tpr-{uuid.uuid4().hex[:10]}"
    base = {
        "ENV": "local",
        "GRPC_PORT": "50059",
        "KAFKA_ENABLED": "true",
        "KAFKA_BROKERS": "redpanda:9092",
        "KAFKA_CONSUMER_GROUP": group,
        "ENGAGEMENT_CONSUMER_GROUP": f"{group}.engagement",
        "WAREHOUSE_DRIVER": "duckdb",
        "DUCKDB_PATH": "/data/analytics.duckdb",
        **env,
    }
    cmd = ["docker", "run", "-d", "--name", name, "--network", get_settings().stack_network]
    cmd += ["-v", f"{data}:/data"]
    for key, value in base.items():
        cmd += ["-e", f"{key}={value}"]
    cmd.append(ANALYTICS_IMAGE)
    subprocess.run(cmd, check=True, capture_output=True, text=True, timeout=120)

    def _cleanup() -> None:
        subprocess.run(["docker", "rm", "-f", name], capture_output=True, check=False, timeout=60)
        shutil.rmtree(data, ignore_errors=True)

    world.add_cleanup(_cleanup)
    return name, data


def _running(name: str) -> bool:
    out = subprocess.run(
        ["docker", "inspect", "-f", "{{.State.Running}}", name],
        capture_output=True,
        text=True,
        check=False,
    )
    return out.stdout.strip() == "true"


def _sample(path: Path) -> tuple[int, int, bool] | None:
    """(inode, size, complete) of the file as a reader opening it now would see it."""
    try:
        with open(path, "rb") as f:
            st = os.fstat(f.fileno())
            head = f.read(4)
            f.seek(max(st.st_size - 4, 0))
            tail = f.read(4)
    except FileNotFoundError:
        return None
    complete = st.st_size > 8 and head == _PARQUET_MAGIC and tail == _PARQUET_MAGIC
    return st.st_ino, st.st_size, complete


@given("a team-analytics exporting every second whose previous export file exists")
def exporter_with_previous_file(world: World) -> None:
    export_env = {
        "PARQUET_EXPORT_PATH": f"/data/{EXPORT_FILE}",
        "PARQUET_EXPORT_INTERVAL_SECONDS": "1",
    }
    name, data = _start_analytics(world, export_env)
    path = data / EXPORT_FILE
    deadline = time.monotonic() + 60
    while _sample(path) is None:
        assert _running(name), "team-analytics exited before its first export"
        assert time.monotonic() < deadline, "no export file within 60 s"
        time.sleep(0.2)
    world.state.extra["tpr_export"] = {"path": path, "first": _sample(path)}


@when("further exports replace the file while a reader keeps opening it")
def reader_while_exports_replace(world: World) -> None:
    ctx = world.state.extra["tpr_export"]
    samples: list[tuple[int, int, bool]] = []
    deadline = time.monotonic() + 8.0
    while time.monotonic() < deadline:
        sample = _sample(ctx["path"])
        if sample is not None:
            samples.append(sample)
        time.sleep(0.01)
    ctx["samples"] = samples


@then("the file was replaced by a new one at least twice")
def file_replaced(world: World) -> None:
    ctx = world.state.extra["tpr_export"]
    inodes = {ino for ino, _, _ in ctx["samples"]} | {ctx["first"][0]}
    # Writing in place keeps one inode; a temporary file renamed over it gives a new one each export.
    assert len(inodes) >= 3, f"the export file kept {len(inodes)} inode(s) over 8 s of 1 s exports"


@then("the reader never saw an incomplete Parquet file")
def reader_never_saw_partial(world: World) -> None:
    samples = world.state.extra["tpr_export"]["samples"]
    assert len(samples) > 50, f"the reader only managed {len(samples)} reads"
    partial = [s for s in samples if not s[2]]
    assert not partial, f"{len(partial)} of {len(samples)} reads saw a partial file: {partial[:3]}"


@given(
    "team-analytics started with PARQUET_EXPORT_INTERVAL_SECONDS set to 0 and another with it unset"
)
def two_exporters_without_export(world: World) -> None:
    path = {"PARQUET_EXPORT_PATH": f"/data/{EXPORT_FILE}"}
    zero = _start_analytics(world, {**path, "PARQUET_EXPORT_INTERVAL_SECONDS": "0"})
    unset = _start_analytics(world, {})
    world.state.extra["tpr_disabled"] = {"interval 0": zero, "unset": unset}


@when("both have run for several seconds")
def exporters_run(world: World) -> None:
    # The enabled exporter writes within ~3 s of start (the other scenario); wait well past that.
    time.sleep(10.0)


@then("neither wrote an export file")
def no_export_files(world: World) -> None:
    for label, (_, data) in world.state.extra["tpr_disabled"].items():
        written = sorted(p.name for p in data.iterdir() if ".parquet" in p.name)
        assert not written, f"{label}: export files appeared: {written}"


@then("both are still running with a warehouse")
def exporters_alive(world: World) -> None:
    for label, (name, data) in world.state.extra["tpr_disabled"].items():
        assert _running(name), f"{label}: team-analytics is not running"
        assert (data / "analytics.duckdb").exists(), f"{label}: no warehouse file"


# ── serving ───────────────────────────────────────────────────────────────
def _serving_collection(world: World) -> tuple[str, str]:
    """(model_version, item collection) of the generation the gateway currently serves."""
    data = world.service_factory.recommendation.recommend(context=CONTEXT_HOMEPAGE, limit=5)
    version = data.get("modelVersion", "")
    assert re.fullmatch(r"als-.+", version), f"no trained generation is serving: {data}"
    return version, f"item_als_vectors__{version}"


def _payload_ids(collection: str) -> dict[str, str]:
    """point id -> payload listing_id of every point in the collection."""
    out: dict[str, str] = {}
    offset = None
    while True:
        body = {"limit": 1000, "with_payload": True, "with_vector": False}
        if offset is not None:
            body["offset"] = offset
        res = ssa_qdrant("POST", f"/collections/{collection}/points/scroll", body)["result"]
        for point in res["points"]:
            out[str(point["id"])] = (point.get("payload") or {}).get("listing_id", "")
        offset = res.get("next_page_offset")
        if offset is None:
            return out


@given("a listing present in the serving generation's trained collection")
def seed_in_trained_collection(world: World) -> None:
    version, collection = _serving_collection(world)
    points = _payload_ids(collection)
    assert points, f"{collection} holds no points"
    # A seed with enough neighbours that the answer is not the (shorter) popularity fallback.
    seed = next(iter(points.values()))
    world.state.extra["tpr_seed"] = {
        "version": version,
        "collection": collection,
        "points": points,
        "seed": seed,
    }


@when("similar items are requested for that listing through the gateway")
def request_similar(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    service = world.service_factory.recommendation
    # team-ai gives retrieval a 15 ms budget; the first call after idle can miss it and is answered
    # by the popularity floor. A warm repeat is the retrieval answer.
    neighbours = ssa_qdrant(
        "POST",
        f"/collections/{ctx['collection']}/points/query",
        {"query": ssa_point_id(ctx["seed"]), "limit": 20, "with_payload": True},
    )["result"]["points"]
    ctx["expected"] = [p["payload"]["listing_id"] for p in neighbours]
    answer: dict = {}
    for _ in range(10):
        answer = service.recommend(
            seed_listing_id=ctx["seed"], context=CONTEXT_SIMILAR_ITEMS, limit=5
        )
        got = [i["listingId"] for i in sorted(answer.get("items") or [], key=lambda i: i["rank"])]
        if got and set(got) <= set(ctx["expected"]):
            break
        time.sleep(0.3)
    ctx["answer"] = answer


@then("the answer is the nearest neighbours of that listing's uuid5 point")
def answer_is_neighbours(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    got = [
        i["listingId"] for i in sorted(ctx["answer"].get("items") or [], key=lambda i: i["rank"])
    ]
    assert ctx["expected"], "Qdrant knows no neighbour of the seed point"
    # team-ai re-orders the retrieved neighbours with its own ranking, so the answer is a subset of
    # the seed point's nearest neighbours rather than their exact order.
    assert got and set(got) <= set(
        ctx["expected"]
    ), f"gateway answered {got}, the seed point's nearest neighbours are {ctx['expected']}"
    assert ctx["answer"].get("modelVersion") == ctx["version"], ctx["answer"]


@then("every returned id is a listing id from the payload, never a Qdrant point id")
def ids_are_listing_ids(world: World) -> None:
    ctx = world.state.extra["tpr_seed"]
    ids = [i["listingId"] for i in ctx["answer"]["items"]]
    point_ids = set(ctx["points"])
    payload_ids = set(ctx["points"].values())
    assert ids and all(i in payload_ids for i in ids), ids
    assert not [i for i in ids if i in point_ids], f"a Qdrant point id was returned: {ids}"
    assert ctx["seed"] not in ids


@given("the training job has published a generation from the stack's tracking events")
def trained_generation_serving(world: World) -> None:
    version, collection = _serving_collection(world)
    points = _payload_ids(collection)
    assert points, f"{collection} holds no points"
    world.state.extra["tpr_trained"] = {"version": version, "listing_ids": set(points.values())}


@then('the "Gợi ý cho bạn" row shows cards for listings of that trained generation')
def row_cards_are_trained_listings(world: World) -> None:
    ctx = world.state.extra["tpr_trained"]
    row = world.get_page(PageName.HOME).recommendations  # type: ignore[attr-defined]
    expect(row.heading).to_be_visible(timeout=timeouts.LONG)
    expect(row.cards.first).to_be_visible(timeout=timeouts.LONG)
    hrefs = row.cards.evaluate_all("els => els.map(e => e.getAttribute('href') || '')")
    shown = {h.rsplit("/", 1)[-1] for h in hrefs if "/listing/" in h}
    assert shown, f"the row rendered no listing cards: {hrefs}"
    assert (
        shown <= ctx["listing_ids"]
    ), f"cards outside the trained generation: {shown - ctx['listing_ids']}"
    served = world.service_factory.recommendation.recommend(context=CONTEXT_HOMEPAGE, limit=10)
    assert served.get("modelVersion") == ctx["version"], served


# ── evaluation protocol ───────────────────────────────────────────────────
def _unique_target_events() -> list[dict]:
    """Four users share listings a and b; each then discovers a listing nobody else touched."""
    rows: list[tuple[str, str, str, float]] = []
    for i in range(_N_USERS):
        user = f"u{i}"
        rows += [(user, "a", "view", 9), (user, "b", "click", 8), (user, f"t{i}", "view", 1)]
    return _events(rows)


def _runs(world: World) -> list[JobRun]:
    return world.state.extra["tpr_runs"]


@given("every user's most recently discovered listing is one nobody else interacted with")
def unique_targets(world: World) -> None:
    world.state.extra["tpr_plan"] = (_unique_target_events(), [{}])


@given(
    "an incumbent champion whose metrics carry no evaluation protocol and a score the candidate cannot match"
)
def legacy_champion(world: World) -> None:
    world.state.extra["tpr_plan"] = (_unique_target_events(), [{"@legacy_champion": "1"}])


@when("the pipeline evaluates and trains over those interactions")
@when("the pipeline completes a run under the current protocol")
def run_job(world: World) -> None:
    events, runs = world.state.extra["tpr_plan"]
    result = run_recsys_job(events, runs)
    for run in result:
        assert run.exit_code == 0, f"the job signalled failure: {run.summary}\n{run.log}"
    world.state.extra["tpr_runs"] = result


@then("the evaluation was trained on every pair except the held-out ones")
def eval_training_excludes_targets(world: World) -> None:
    (run,) = _runs(world)
    metrics = run.summary["metrics"]
    all_pairs = _N_USERS * 3
    # Each user's target pair, and anything of that user at or after the discovery, is held out.
    assert metrics["test_events"] == _N_USERS, metrics
    assert metrics["train_events"] == all_pairs - _N_USERS, metrics


@then("the run's metrics carry the evaluation protocol identifier")
def metrics_carry_protocol(world: World) -> None:
    (run,) = _runs(world)
    version = run.summary["model_version"]
    assert run.summary["metrics"]["eval_protocol"] == _PROTOCOL, run.summary["metrics"]
    assert run.state["models"][version]["metrics"]["eval_protocol"] == _PROTOCOL


@then(
    "the held-out listings score zero for the evaluation model while the published model learned them"
)
def targets_unlearnable_by_eval_model(world: World) -> None:
    (run,) = _runs(world)
    metrics = run.summary["metrics"]
    # No held-out listing exists in the evaluation model, so it cannot rank one.
    assert metrics["hit_rate@20"] == 0.0 and metrics["ndcg@10"] == 0.0, metrics
    # The published model is trained on every event: it holds a, b and the four targets.
    assert run.summary["items"] == 2 + _N_USERS, run.summary
    assert run.state["items"]["count"] == 2 + _N_USERS, run.state["items"]


@then("the candidate becomes the champion")
def candidate_is_champion(world: World) -> None:
    (run,) = _runs(world)
    assert run.summary["decision"] == "promoted", run.summary
    assert run.summary["incumbent_version"] == "als-legacy-e2e", run.summary
    # The candidate's own score is far below the incumbent's 0.99: a comparison would reject it.
    assert run.summary["candidate_value"] < 0.99, run.summary
    assert run.state["champion"] == run.summary["model_version"], run.state["champion"]
    assert run.state["models"]["als-legacy-e2e"]["status"] == "archived", run.state["models"]


@then("the promotion reason names both protocols")
def reason_names_both(world: World) -> None:
    (run,) = _runs(world)
    reason = run.summary["reason"]
    assert "unversioned" in reason and _PROTOCOL in reason, reason
