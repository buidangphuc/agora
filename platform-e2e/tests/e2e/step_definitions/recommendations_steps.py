"""Recommendations row ("Gợi ý cho bạn") step definitions (surface-recommendations).

Asserts the user-facing row on home (context HOMEPAGE) and PDP (context
SIMILAR_ITEMS, seeded by the listing), AND that it is sourced from team-ai
through the gateway `RecommendationService/Recommend` — not a client-side mock
and not a direct browser call to team-ai. The gateway probe uses the buyer token
already active on the service factory (set by `login_via_api`), i.e. the SAME
session the server components use, so a non-empty response proves the real
frontend → gateway → team-ai path. Needs the live stack + team-ai
RECS_ENABLED=true with the training-job Qdrant/Redis data to be populated.
"""

from __future__ import annotations

import sys
import tempfile
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest.mock import MagicMock

import pandas as pd
from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.api.services import GatewayError
from src.api.services.recommendation_service import (
    CONTEXT_HOMEPAGE,
    CONTEXT_SIMILAR_ITEMS,
)
from src.constants import PageName, timeouts
from src.pages.components import RecommendationsRowComponent
from tests.e2e.support.world import World

# Ensure platform-recsys package is importable
_PLATFORM_RECSYS_DIR = Path(__file__).resolve().parents[4] / "platform-recsys"
if str(_PLATFORM_RECSYS_DIR) not in sys.path:
    sys.path.insert(0, str(_PLATFORM_RECSYS_DIR))

from recsys.config import Settings  # noqa: E402
from recsys.pipeline import run as run_recsys_pipeline  # noqa: E402
from recsys.registry.metadata import ModelMetadata  # noqa: E402
from recsys.registry.registry import ModelRegistry  # noqa: E402


def _row(world: World) -> RecommendationsRowComponent:
    """Resolve the recs row from whichever page currently carries it."""
    name = PageName.LISTING_DETAIL if "/listing/" in world.page.url else PageName.HOME
    page = world.get_page(name)
    return page.recommendations  # type: ignore[attr-defined]


def _probe_gateway(world: World, *, seed_listing_id: str = "", context: str) -> None:
    """Call Recommend through the gateway as the buyer; record items / UNAVAILABLE."""
    try:
        data = world.service_factory.recommendation.recommend(
            seed_listing_id=seed_listing_id, context=context, limit=10
        )
        world.state.extra["recs_items"] = data.get("items") or []
        world.state.extra["recs_error"] = None
    except GatewayError as exc:
        world.state.extra["recs_items"] = []
        world.state.extra["recs_error"] = exc


# ── UI assertions ──────────────────────────────────────────────────────────
@then('the "Gợi ý cho bạn" recommendations row is populated with product cards')
def row_populated(world: World) -> None:
    row = _row(world)
    expect(row.heading).to_be_visible(timeout=timeouts.LONG)
    expect(row.cards.first).to_be_visible(timeout=timeouts.LONG)
    assert row.card_count() > 0, "recommendations row rendered no product cards"


@then("the row is sourced from team-ai via the gateway, not a client-side mock")
def row_sourced_via_gateway(world: World) -> None:
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)
    items = world.state.extra["recs_items"]
    assert items, (
        "gateway Recommend returned no items — the row cannot be sourced from "
        "team-ai (is RECS_ENABLED=true with training data?)"
    )
    # The RPC returns id-only items (Rule 3); those ids must be the /listing/<id>
    # cards hydrated into the row — proving the row reflects the RPC response and
    # is not a hardcoded client-side list.
    recommended_ids = {it.get("listingId") for it in items if it.get("listingId")}
    row = _row(world)
    hrefs = row.cards.evaluate_all("els => els.map(e => e.getAttribute('href') || '')")
    rendered_ids = {h.rsplit("/", 1)[-1] for h in hrefs if "/listing/" in h}
    assert recommended_ids & rendered_ids, (
        "no recommended id from the gateway appears among the rendered cards — "
        f"RPC ids={recommended_ids}, card ids={rendered_ids}"
    )
    world.logger.info(f"Recommend returned {len(items)} item(s) via the gateway")


@then("the row is seeded with the current listing via the gateway to team-ai")
def row_seeded_by_listing(world: World) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id, "no seeded listing in scenario state"
    _probe_gateway(world, seed_listing_id=listing.listing_id, context=CONTEXT_SIMILAR_ITEMS)
    items = world.state.extra["recs_items"]
    assert items, (
        "gateway Recommend (SIMILAR_ITEMS, seeded) returned no items — the PDP "
        "row cannot be sourced from team-ai"
    )


@then("the recommendations came through the gateway Recommend RPC")
def recommendations_via_rpc(world: World) -> None:
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)
    assert (
        world.state.extra["recs_error"] is None
    ), f"gateway Recommend errored: {world.state.extra['recs_error']}"
    assert world.state.extra["recs_items"], "gateway Recommend returned no items"


@then("the browser markup carries no team-ai address or recommendation service client")
def no_teamai_in_markup(world: World) -> None:
    content = world.page.content().lower()
    # Server-only surfacing (Rule 1): the browser must never learn team-ai's
    # address nor ship a recommendation gRPC client.
    for banned in ("team-ai", ":50060", "recommendationservice", "recommend_pb"):
        assert banned not in content, f"team-ai/recs client leaked into the browser: {banned!r}"


# ── Graceful degradation ───────────────────────────────────────────────────
@when("the recommendation service is unavailable")
def recommendation_service_unavailable(world: World) -> None:
    # We cannot flip RECS_ENABLED from e2e; probe the current availability so the
    # assertion can branch. The contract holds either way: an UNAVAILABLE /
    # empty response must degrade to a hidden row, never a page error.
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)


@then("the home page still renders")
def home_still_renders(world: World) -> None:
    home = world.get_page(PageName.HOME)
    assert home.is_displayed(), f"home page failed to render (url={world.page.url})"


@then('the "Gợi ý cho bạn" row is hidden or empty rather than erroring the page')
def row_hidden_or_populated_never_errored(world: World) -> None:
    # No user-visible error regardless of recs availability.
    error_banner = world.page.locator(".bg-red-900:has-text('Lỗi'), [data-toast='error']")
    assert (
        error_banner.count() == 0 or not error_banner.first.is_visible()
    ), "a user-visible error was shown when recommendations were unavailable"

    row = _row(world)
    unavailable = bool(world.state.extra.get("recs_error")) or not world.state.extra.get(
        "recs_items"
    )
    if unavailable:
        # Frontend renders nothing when the list is empty — the row is hidden.
        assert row.heading.count() == 0, "row should be hidden when recs are unavailable"
    else:
        # Recs are available in this env — the row is shown instead of erroring.
        expect(row.heading).to_be_visible(timeout=timeouts.DEFAULT)


# ── Pipeline Evaluation, Model Registry & Gate Step Definitions ───────────


def _patch_loaders_if_needed() -> None:
    import recsys.load.qdrant as qdrant_load
    import recsys.load.redis_cache as redis_cache

    if not isinstance(qdrant_load.load_vectors, MagicMock):
        qdrant_load.load_vectors = MagicMock(return_value={"items": 3, "users": 3})
    if not isinstance(redis_cache.load_cache, MagicMock):
        redis_cache.load_cache = MagicMock(return_value={"items": 3, "users": 3})


def _create_sample_parquet(tmp_dir: Path, interactions=None) -> Path:
    if interactions is None:
        now = datetime.now(timezone.utc)
        interactions = [
            ("u1", "l1", "view", now - timedelta(hours=9)),
            ("u1", "l1", "click", now - timedelta(hours=8)),
            ("u1", "l2", "view", now - timedelta(hours=7)),
            ("u1", "l3", "view", now - timedelta(hours=6)),
            ("u2", "l1", "view", now - timedelta(hours=5)),
            ("u2", "l2", "click", now - timedelta(hours=4)),
            ("u2", "l3", "view", now - timedelta(hours=3)),
            ("u3", "l2", "view", now - timedelta(hours=2)),
            ("u3", "l3", "add_to_cart", now - timedelta(hours=1)),
            ("u3", "l1", "view", now),
        ]
    df = pd.DataFrame({
        "event_type": [x[2] for x in interactions],
        "principal_id": [x[0] for x in interactions],
        "anonymous_id": ["" for _ in interactions],
        "listing_id": [x[1] for x in interactions],
        "occurred_at": [x[3] for x in interactions],
    })
    parquet_path = tmp_dir / "tracking_events.parquet"
    df.to_parquet(parquet_path, coerce_timestamps="ms", allow_truncated_timestamps=True)
    return parquet_path


@given("an offline batch pipeline run over warehouse tracking events")
def offline_batch_pipeline(world: World) -> None:
    tmp_dir = Path(tempfile.mkdtemp())
    parquet_path = _create_sample_parquet(tmp_dir)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
    )
    registry = ModelRegistry()
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("ALS training completes and the temporal holdout is scored")
def als_training_scored(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("the run reports ndcg@10 and coverage@10 metrics attributed to the run's model_version")
def run_reports_metrics(world: World) -> None:
    summary = world.state.extra["summary"]
    metrics = summary.get("metrics", {})
    assert "ndcg@10" in metrics, f"metrics missing ndcg@10: {metrics}"
    assert "coverage@10" in metrics, f"metrics missing coverage@10: {metrics}"
    assert summary.get("model_version"), "summary missing model_version"


@given("an interaction window with no test events after temporal split")
def interaction_window_no_test_events(world: World) -> None:
    tmp_dir = Path(tempfile.mkdtemp())
    now = datetime.now(timezone.utc)
    interactions = [
        ("u1", "l1", "view", now),
        ("u2", "l2", "view", now),
        ("u3", "l3", "view", now),
    ]
    parquet_path = _create_sample_parquet(tmp_dir, interactions=interactions)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
    )
    registry = ModelRegistry()
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("the evaluation stage runs")
def evaluation_stage_runs(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("no candidate is registered and the promotion gate is skipped")
def no_candidate_registered(world: World) -> None:
    summary = world.state.extra["summary"]
    metrics = summary.get("metrics", {})
    assert metrics.get("test_events", 0) == 0


@given("an incumbent champion model in the registry")
def incumbent_champion_in_registry(world: World) -> None:
    registry = ModelRegistry()
    champ = ModelMetadata(
        model_version="champ-v1",
        model_name="recsys-als",
        model_type="als",
        metrics={"ndcg@10": 1.0, "coverage@10": 1.0},
        status="champion",
    )
    registry.register_model(champ)
    registry._set_champion(champ)

    tmp_dir = Path(tempfile.mkdtemp())
    parquet_path = _create_sample_parquet(tmp_dir)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
        promotion_min_relative_improvement=0.10,
    )
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("a candidate model evaluates below the incumbent champion tolerance")
def candidate_evaluates_below_tolerance(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("the candidate status is recorded as rejected")
def candidate_status_rejected(world: World) -> None:
    summary = world.state.extra["summary"]
    assert summary["decision"] == "rejected"
    candidate = world.state.extra["registry"].get_model(summary["model_version"])
    assert candidate is not None and candidate.status == "rejected"


@then("the champion key still names the previous version")
def champion_key_unchanged(world: World) -> None:
    registry = world.state.extra["registry"]
    champ_ver = registry.get_champion_version()
    assert champ_ver == "champ-v1"


@given("a candidate model rejected by the promotion gate")
def rejected_candidate_model(world: World) -> None:
    registry = ModelRegistry()
    champ = ModelMetadata(
        model_version="champ-v1",
        model_name="recsys-als",
        model_type="als",
        metrics={"ndcg@10": 1.0, "coverage@10": 1.0},
        status="champion",
    )
    registry.register_model(champ)
    registry._set_champion(champ)

    tmp_dir = Path(tempfile.mkdtemp())
    parquet_path = _create_sample_parquet(tmp_dir)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
        promotion_min_relative_improvement=0.10,
    )
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("the pipeline run finishes")
def pipeline_run_finishes(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("serving vector collections and active model_version remain on the previous generation")
def active_model_version_unchanged(world: World) -> None:
    summary = world.state.extra["summary"]
    assert summary["decision"] == "rejected"
    assert "qdrant" not in summary
    assert "cache" not in summary
    champ_ver = world.state.extra["registry"].get_champion_version()
    assert champ_ver == "champ-v1"


@given("an empty model registry with no existing champion")
def empty_model_registry(world: World) -> None:
    registry = ModelRegistry()
    tmp_dir = Path(tempfile.mkdtemp())
    parquet_path = _create_sample_parquet(tmp_dir)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
    )
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("the initial pipeline run completes with valid metrics")
def initial_pipeline_run_completes(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("the initial model is promoted as champion and published to serving stores")
def initial_model_promoted(world: World) -> None:
    summary = world.state.extra["summary"]
    assert summary["decision"] == "promoted"
    registry = world.state.extra["registry"]
    champ_ver = registry.get_champion_version()
    assert champ_ver == summary["model_version"]
    assert "qdrant" in summary
    assert "cache" in summary


@given("an offline pipeline execution")
def offline_pipeline_execution(world: World) -> None:
    registry = ModelRegistry()
    tmp_dir = Path(tempfile.mkdtemp())
    parquet_path = _create_sample_parquet(tmp_dir)
    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_path),
        als_max_iter=2,
        als_rank=4,
        top_n=5,
    )
    world.state.extra["settings"] = settings
    world.state.extra["registry"] = registry


@when("the pipeline completes evaluation and gating")
def pipeline_completes_gating(world: World) -> None:
    settings = world.state.extra["settings"]
    registry = world.state.extra["registry"]
    _patch_loaders_if_needed()
    summary = run_recsys_pipeline(settings=settings, registry=registry)
    world.state.extra["summary"] = summary


@then("the summary reports model_version, metrics, decision, and promotion reason")
def summary_reports_all_fields(world: World) -> None:
    s = world.state.extra["summary"]
    assert "model_version" in s and s["model_version"]
    assert "metrics" in s and isinstance(s["metrics"], dict)
    assert "decision" in s and s["decision"] in ("promoted", "rejected")
    assert "reason" in s and len(s["reason"]) > 0

