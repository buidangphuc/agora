"""Helpers for the platform-modelserve contract scenarios (area ms).

The router and the TEI fake come from `platform-e2e/compose/modelserve.override.yaml`; they are
reached on the host ports that overlay publishes:

    MODELSERVE_URL           default http://localhost:18100   the platform-modelserve router
    TEI_FAKE_URL             default http://localhost:18110   the deterministic TEI / vLLM stand-in
    MS_ROUTER_CONTAINER      default agora-modelserve-router-1  `docker stop` target of the outage scenarios

The fake keeps a request log (`/_requests`), so "the router did not call the upstream" is observed
on the upstream itself, never inferred from the router's reply. Every text carries a per-run unique
word, so the router's Redis cache (shared across runs) can never serve a previous run's vector.
"""

from __future__ import annotations

import json
import os
import uuid
from typing import Any

import httpx

EMBED_DIM = 384


def router_url() -> str:
    return os.getenv("MODELSERVE_URL", "http://localhost:18100").rstrip("/")


def fake_url() -> str:
    return os.getenv("TEI_FAKE_URL", "http://localhost:18110").rstrip("/")


def router_container() -> str:
    return os.getenv("MS_ROUTER_CONTAINER", "agora-modelserve-router-1")


def uid() -> str:
    return "zms" + uuid.uuid4().hex[:10]


def router_post(path: str, body: dict[str, Any], timeout: float = 30.0) -> httpx.Response:
    return httpx.post(router_url() + path, json=body, timeout=timeout)


def fake_post(path: str, body: dict[str, Any]) -> httpx.Response:
    return httpx.post(fake_url() + path, json=body, timeout=30.0)


def fake_calls(contains: str, kind: str = "") -> list[dict[str, Any]]:
    """Requests the fake received whose raw body contains `contains` (optionally of one kind)."""
    params = {"contains": contains}
    if kind:
        params["kind"] = kind
    resp = httpx.get(fake_url() + "/_requests", params=params, timeout=15.0)
    resp.raise_for_status()
    return resp.json()["requests"]


def fake_inputs(call: dict[str, Any]) -> list[str]:
    """The texts of one recorded embed request, in the order the router sent them."""
    body = json.loads(call["body"])
    value = body.get("inputs", body.get("input", body.get("texts")))
    return [value] if isinstance(value, str) else list(value or [])


def fake_vectors(texts: list[str]) -> list[list[float]]:
    """What the fake answers for `texts` (ground truth for order and values)."""
    resp = fake_post("/embed", {"inputs": texts})
    resp.raise_for_status()
    return resp.json()


def conforms_to_team_ai_parser(data: Any, count: int, expected_dim: int = EMBED_DIM) -> None:
    """The rules of team-ai `_extract_vectors` (app/modules/ai/rag/embeddings.py), raised as asserts."""
    assert (
        isinstance(data, dict) and "embeddings" in data
    ), f"no 'embeddings' key: {str(data)[:200]}"
    raw = data["embeddings"]
    assert (
        isinstance(raw, list) and len(raw) == count
    ), f"expected {count} vectors, got {raw!r:.120}"
    for vector in raw:
        assert isinstance(vector, list) and vector, "a non-vector element"
        assert all(
            isinstance(x, (int, float)) and not isinstance(x, bool) for x in vector
        ), "a non-numeric component"
        assert len(vector) == expected_dim, f"dimension {len(vector)} != {expected_dim}"
