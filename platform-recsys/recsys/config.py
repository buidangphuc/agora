"""Env-driven configuration with an `.env.example` drift gate.

Mirrors the reflection-style config used across the platform (team-analytics'
`internal/config` struct tags, team-ai's pydantic Settings): every setting
declares its env var name and default in ONE place — the ``_FIELDS`` table —
which is the single source of truth for BOTH loading the environment AND the
`.env.example` drift test (``env_names()`` vs the file's keys, both directions).

Pure stdlib (dataclasses) so it loads and unit-tests without PySpark, Qdrant, or
Redis on the host.
"""

from __future__ import annotations

import json
import os
from collections.abc import Callable
from dataclasses import dataclass, field, fields
from typing import Any

# Warehouse driver identifiers selected by WAREHOUSE_DRIVER (mirrors
# team-analytics' DriverDuckDB / DriverBigQuery — same env value both sides).
DRIVER_DUCKDB = "duckdb"
DRIVER_BIGQUERY = "bigquery"

# The default per-event confidence weights (implicit feedback). JSON so the whole
# map is overridable from a single env var.
DEFAULT_EVENT_WEIGHTS = {
    "impression": 0.5,
    "view": 1.0,
    "click": 2.0,
    "add_to_cart": 5.0,
}


def _as_bool(v: str) -> bool:
    return v.strip().lower() in ("1", "true", "yes", "on")


def _as_int(v: str) -> int:
    return int(v.strip())


def _as_float(v: str) -> float:
    return float(v.strip())


def _as_str(v: str) -> str:
    return v


def _as_weights(v: str) -> dict[str, float]:
    """Parse the JSON event->weight map, keyed by lowercase event_type."""
    raw = json.loads(v)
    return {str(k).lower(): float(w) for k, w in raw.items()}


# (attr, env, default_str, caster). default_str is the exact literal that must
# appear in .env.example, so the drift gate compares like-for-like.
_FIELDS: list[tuple[str, str, str, Callable[[str], Any]]] = [
    # ── Runtime ──────────────────────────────────────────────────────────────
    ("env", "ENV", "local", _as_str),
    ("log_level", "LOG_LEVEL", "info", _as_str),
    # ── Spark ────────────────────────────────────────────────────────────────
    # Local dev / CI runs in-process local mode; a real cluster overrides this.
    ("spark_master", "SPARK_MASTER", "local[*]", _as_str),
    ("spark_app_name", "SPARK_APP_NAME", "platform-recsys-als", _as_str),
    # ── Warehouse reader seam (the only driver-specific seam) ────────────────
    ("warehouse_driver", "WAREHOUSE_DRIVER", "duckdb", _as_str),
    # Local: DuckDB-exported Parquet on the warehouse volume
    # (team-analytics `ExportParquet`), read via spark.read.parquet(...).
    ("warehouse_parquet_path", "WAREHOUSE_PARQUET_PATH", "/data/tracking_events.parquet", _as_str),
    # Prod: BigQuery table via the Spark BigQuery connector.
    ("bigquery_project", "BIGQUERY_PROJECT", "", _as_str),
    ("bigquery_dataset", "BIGQUERY_DATASET", "analytics", _as_str),
    ("bigquery_table", "BIGQUERY_TABLE", "tracking_events", _as_str),
    # ── Interactions ─────────────────────────────────────────────────────────
    ("interaction_window_days", "INTERACTION_WINDOW_DAYS", "30", _as_int),
    ("event_weights_json", "EVENT_WEIGHTS_JSON", json.dumps(DEFAULT_EVENT_WEIGHTS), _as_weights),
    # Half-life (days) for optional recency decay; 0 disables decay.
    ("recency_half_life_days", "RECENCY_HALF_LIFE_DAYS", "0", _as_float),
    ("min_interactions_per_user", "MIN_INTERACTIONS_PER_USER", "1", _as_int),
    ("min_interactions_per_item", "MIN_INTERACTIONS_PER_ITEM", "1", _as_int),
    # ── ALS hyperparameters (Spark MLlib) ────────────────────────────────────
    ("als_rank", "ALS_RANK", "64", _as_int),
    ("als_reg_param", "ALS_REG_PARAM", "0.05", _as_float),
    ("als_alpha", "ALS_ALPHA", "40.0", _as_float),
    ("als_max_iter", "ALS_MAX_ITER", "15", _as_int),
    # ── Outputs ──────────────────────────────────────────────────────────────
    ("top_n", "TOP_N", "50", _as_int),
    # Qdrant (:6333 — the instance team-ai targets via RAG_QDRANT_URL).
    ("qdrant_url", "QDRANT_URL", "http://localhost:6333", _as_str),
    # Item-vector collection: MUST equal the consumer's RECS_QDRANT_COLLECTION
    # default (serve-recommendations-teamai) so producer and reader agree.
    ("qdrant_item_collection", "QDRANT_ITEM_COLLECTION", "item_als_vectors", _as_str),
    ("qdrant_user_collection", "QDRANT_USER_COLLECTION", "user_als_vectors", _as_str),
    # Redis (:6379) precomputed cache.
    ("redis_host", "REDIS_HOST", "localhost", _as_str),
    ("redis_port", "REDIS_PORT", "6379", _as_int),
    ("redis_password", "REDIS_PASSWORD", "", _as_str),
    ("redis_db", "REDIS_DATABASE", "0", _as_int),
    # Cache key schema — MUST match the consumer's RECS_CACHE_PREFIX +
    # schema version so keys line up: `recs:v1:user:{id}` etc.
    ("cache_prefix", "RECS_CACHE_PREFIX", "recs", _as_str),
    ("cache_schema_version", "RECS_SCHEMA_VERSION", "v1", _as_str),
    # TTL a bit longer than the batch cadence (nightly) so a missed run degrades
    # gracefully rather than emptying the cache. Default 48h.
    ("cache_ttl_seconds", "RECS_CACHE_TTL_SECONDS", "172800", _as_int),
    # Provenance stamped on every artifact; empty ⇒ derive from the run clock.
    ("model_version", "MODEL_VERSION", "", _as_str),
]


@dataclass
class Settings:
    env: str = "local"
    log_level: str = "info"
    spark_master: str = "local[*]"
    spark_app_name: str = "platform-recsys-als"
    warehouse_driver: str = "duckdb"
    warehouse_parquet_path: str = "/data/tracking_events.parquet"
    bigquery_project: str = ""
    bigquery_dataset: str = "analytics"
    bigquery_table: str = "tracking_events"
    interaction_window_days: int = 30
    event_weights_json: dict[str, float] = field(default_factory=lambda: dict(DEFAULT_EVENT_WEIGHTS))
    recency_half_life_days: float = 0.0
    min_interactions_per_user: int = 1
    min_interactions_per_item: int = 1
    als_rank: int = 64
    als_reg_param: float = 0.05
    als_alpha: float = 40.0
    als_max_iter: int = 15
    top_n: int = 50
    qdrant_url: str = "http://localhost:6333"
    qdrant_item_collection: str = "item_als_vectors"
    qdrant_user_collection: str = "user_als_vectors"
    redis_host: str = "localhost"
    redis_port: int = 6379
    redis_password: str = ""
    redis_db: int = 0
    cache_prefix: str = "recs"
    cache_schema_version: str = "v1"
    cache_ttl_seconds: int = 172800
    model_version: str = ""

    # ── Derived helpers ──────────────────────────────────────────────────────
    @property
    def is_prod(self) -> bool:
        return self.env.strip().lower() in ("prod", "production")

    @property
    def event_weights(self) -> dict[str, float]:
        return self.event_weights_json

    def user_cache_key(self, user_key: str) -> str:
        return f"{self.cache_prefix}:{self.cache_schema_version}:user:{user_key}"

    def item_cache_key(self, listing_id: str) -> str:
        return f"{self.cache_prefix}:{self.cache_schema_version}:item:{listing_id}"

    @property
    def popular_cache_key(self) -> str:
        return f"{self.cache_prefix}:{self.cache_schema_version}:popular"

    @property
    def model_version_cache_key(self) -> str:
        return f"{self.cache_prefix}:{self.cache_schema_version}:model_version"

    def validate(self) -> None:
        if self.warehouse_driver == DRIVER_DUCKDB:
            if not self.warehouse_parquet_path.strip():
                raise ValueError("WAREHOUSE_PARQUET_PATH is required when WAREHOUSE_DRIVER=duckdb")
        elif self.warehouse_driver == DRIVER_BIGQUERY:
            if not (
                self.bigquery_project.strip()
                and self.bigquery_dataset.strip()
                and self.bigquery_table.strip()
            ):
                raise ValueError(
                    "BIGQUERY_PROJECT, BIGQUERY_DATASET and BIGQUERY_TABLE are required "
                    "when WAREHOUSE_DRIVER=bigquery"
                )
        else:
            raise ValueError(
                f"WAREHOUSE_DRIVER must be {DRIVER_DUCKDB!r} or {DRIVER_BIGQUERY!r}, "
                f"got {self.warehouse_driver!r}"
            )
        if self.als_rank <= 0:
            raise ValueError(f"ALS_RANK must be > 0: {self.als_rank}")
        if self.als_max_iter <= 0:
            raise ValueError(f"ALS_MAX_ITER must be > 0: {self.als_max_iter}")
        if self.top_n <= 0:
            raise ValueError(f"TOP_N must be > 0: {self.top_n}")
        if self.interaction_window_days <= 0:
            raise ValueError(f"INTERACTION_WINDOW_DAYS must be > 0: {self.interaction_window_days}")
        if not self.event_weights:
            raise ValueError("EVENT_WEIGHTS_JSON must map at least one event_type to a weight")


def env_names() -> list[str]:
    """Ordered env var names declared by the config — the drift-gate anchor."""
    return [env for (_attr, env, _default, _caster) in _FIELDS]


def _defaults_map() -> dict[str, str]:
    return {env: default for (_attr, env, default, _caster) in _FIELDS}


def load_settings(environ: dict[str, str] | None = None) -> Settings:
    """Read the environment into Settings, applying declared defaults."""
    src = os.environ if environ is None else environ
    kwargs: dict[str, Any] = {}
    for attr, env, default, caster in _FIELDS:
        raw = src.get(env)
        kwargs[attr] = caster(raw if raw is not None else default)
    known = {f.name for f in fields(Settings)}
    kwargs = {k: v for k, v in kwargs.items() if k in known}
    s = Settings(**kwargs)
    s.validate()
    return s
