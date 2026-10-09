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

import os
from collections.abc import Callable
from dataclasses import dataclass, fields
from typing import Any


class ConfigError(ValueError):
    """A run cannot start because a required input is missing (the job exits 2)."""


def _as_bool(v: str) -> bool:
    return v.strip().lower() in ("1", "true", "yes", "on")


def _as_int(v: str) -> int:
    return int(v.strip())


def _as_float(v: str) -> float:
    return float(v.strip())


def _as_str(v: str) -> str:
    return v


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
    # ── Governed training dataset (written by platform-featurestore) ─────────
    # The ALS job reads ONLY this: DATASET_PATH (an explicit file) wins over the
    # latest as_of=*.parquet snapshot under DATASET_DIR. Never raw tracking events.
    ("dataset_dir", "DATASET_DIR", "/features/datasets/als_interactions/v1", _as_str),
    ("dataset_path", "DATASET_PATH", "", _as_str),
    # ── Interactions ─────────────────────────────────────────────────────────
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
    # ── Two-Tower candidate retrieval stage (optional) ───────────────────────
    ("enable_two_tower", "ENABLE_TWO_TOWER", "false", _as_bool),
    ("qdrant_two_tower_collection", "QDRANT_TWO_TOWER_COLLECTION", "item_two_tower_vectors", _as_str),
    ("two_tower_dim", "TWO_TOWER_DIM", "32", _as_int),
    # ── Nearline signal consumer (python -m recsys.nearline) ─────────────────
    # A long-running consumer of analytics.events that keeps the recs:nearline:* keys fresh.
    ("kafka_brokers", "KAFKA_BROKERS", "localhost:19092", _as_str),
    ("kafka_analytics_topic", "KAFKA_ANALYTICS_TOPIC", "analytics.events", _as_str),
    ("nearline_consumer_group", "NEARLINE_CONSUMER_GROUP", "platform-recsys-nearline", _as_str),
    # Where a group with no committed offset starts: "latest" (production) or "earliest".
    ("nearline_start_offset", "NEARLINE_START_OFFSET", "latest", _as_str),
    # Lifetime of every nearline key and the age past which an event is ignored (24 h window).
    ("nearline_ttl_seconds", "NEARLINE_TTL_SECONDS", "86400", _as_int),
    # 0 = run until stopped. >0 = exit 0 after this many seconds without a message (drain mode, e2e).
    ("nearline_idle_exit_seconds", "NEARLINE_IDLE_EXIT_SECONDS", "0", _as_int),
    # ── Model Registry & Promotion Gate ──────────────────────────────────────
    ("promotion_primary_metric", "PROMOTION_PRIMARY_METRIC", "ndcg@10", _as_str),
    ("promotion_min_relative_improvement", "PROMOTION_MIN_RELATIVE_IMPROVEMENT", "0.01", _as_float),
    ("promotion_min_coverage_ratio", "PROMOTION_MIN_COVERAGE_RATIO", "0.8", _as_float),
    # Operator override: promote (and publish) this run even if the gate rejects it,
    # e.g. to repopulate Qdrant/Redis after they were reset. Never set it on a schedule.
    ("promotion_force", "PROMOTION_FORCE", "false", _as_bool),
    # ── Structural gate (before the metric gate; recsys-generations) ─────────
    # Reject a degenerate candidate: too few users with a list, too little of the catalogue
    # in any list, lists that are nearly identical across users. NaN/inf factors always reject.
    ("gate_min_user_coverage", "GATE_MIN_USER_COVERAGE", "0.5", _as_float),
    ("gate_min_item_coverage", "GATE_MIN_ITEM_COVERAGE", "0.05", _as_float),
    ("gate_max_list_overlap", "GATE_MAX_LIST_OVERLAP", "0.9", _as_float),
    # Compatibility shim for one release: also write the unscoped recs:v1:{user,item,popular}
    # keys so a reverted team-ai keeps serving the latest generation. Follow-up removes it.
    ("write_legacy_keys", "RECS_WRITE_LEGACY_KEYS", "true", _as_bool),
    # Provenance stamped on every artifact; empty ⇒ derive from the run clock.
    ("model_version", "MODEL_VERSION", "", _as_str),
]


@dataclass
class Settings:
    env: str = "local"
    log_level: str = "info"
    spark_master: str = "local[*]"
    spark_app_name: str = "platform-recsys-als"
    dataset_dir: str = "/features/datasets/als_interactions/v1"
    dataset_path: str = ""
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
    enable_two_tower: bool = False
    qdrant_two_tower_collection: str = "item_two_tower_vectors"
    two_tower_dim: int = 32
    kafka_brokers: str = "localhost:19092"
    kafka_analytics_topic: str = "analytics.events"
    nearline_consumer_group: str = "platform-recsys-nearline"
    nearline_start_offset: str = "latest"
    nearline_ttl_seconds: int = 86400
    nearline_idle_exit_seconds: int = 0
    redis_host: str = "localhost"
    redis_port: int = 6379
    redis_password: str = ""
    redis_db: int = 0
    cache_prefix: str = "recs"
    cache_schema_version: str = "v1"
    cache_ttl_seconds: int = 172800
    promotion_primary_metric: str = "ndcg@10"
    promotion_min_relative_improvement: float = 0.01
    promotion_min_coverage_ratio: float = 0.8
    promotion_force: bool = False
    gate_min_user_coverage: float = 0.5
    gate_min_item_coverage: float = 0.05
    gate_max_list_overlap: float = 0.9
    write_legacy_keys: bool = True
    model_version: str = ""

    # ── Derived helpers ──────────────────────────────────────────────────────
    @property
    def is_prod(self) -> bool:
        return self.env.strip().lower() in ("prod", "production")

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

    # Generation-scoped keys (recsys-generations): everything one model_version published.
    @property
    def _prefix(self) -> str:
        return f"{self.cache_prefix}:{self.cache_schema_version}"

    @property
    def gen_key_prefix(self) -> str:
        return f"{self._prefix}:gen:"

    def gen_user_key(self, gen: str, user_key: str) -> str:
        return f"{self.gen_key_prefix}{gen}:user:{user_key}"

    def gen_item_key(self, gen: str, listing_id: str) -> str:
        return f"{self.gen_key_prefix}{gen}:item:{listing_id}"

    def gen_popular_key(self, gen: str) -> str:
        return f"{self.gen_key_prefix}{gen}:popular"

    @property
    def serving_key(self) -> str:
        return f"{self._prefix}:serving"

    @property
    def previous_key(self) -> str:
        return f"{self._prefix}:previous"

    def validate(self) -> None:
        if self.als_rank <= 0:
            raise ValueError(f"ALS_RANK must be > 0: {self.als_rank}")
        if self.als_max_iter <= 0:
            raise ValueError(f"ALS_MAX_ITER must be > 0: {self.als_max_iter}")
        if self.top_n <= 0:
            raise ValueError(f"TOP_N must be > 0: {self.top_n}")
        if self.nearline_start_offset not in ("latest", "earliest"):
            raise ValueError(
                f"NEARLINE_START_OFFSET must be latest or earliest: {self.nearline_start_offset}"
            )
        if self.nearline_ttl_seconds <= 0:
            raise ValueError(f"NEARLINE_TTL_SECONDS must be > 0: {self.nearline_ttl_seconds}")
        for name in ("gate_min_user_coverage", "gate_min_item_coverage", "gate_max_list_overlap"):
            v = getattr(self, name)
            if not 0.0 <= v <= 1.0:
                raise ValueError(f"{name.upper()} must be within [0, 1]: {v}")


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
