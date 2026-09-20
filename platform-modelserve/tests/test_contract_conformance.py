"""Contract conformance tests against team-ai's _extract_vectors parser."""

from __future__ import annotations

import asyncio
from typing import Any

import httpx
from httpx import ASGITransport

from tests.test_router import get_test_app

# The exact conformance function from team-ai/app/modules/ai/rag/embeddings.py
Vector = list[float]


def _extract_vectors(
    data: Any,
    *,
    count: int,
    expected_dim: int | None = None,
) -> list[Vector]:
    """Parse an embedding server reply into ``count`` float vectors."""
    if isinstance(data, dict):
        if "embeddings" in data:
            raw = data["embeddings"]
        elif "data" in data:  # OpenAI-style: [{"embedding": [...]}, ...]
            raw = [item.get("embedding") for item in data["data"]]
        else:
            raise ValueError("Missing 'embeddings'/'data'")
    elif isinstance(data, list):
        raw = data
    else:
        raise ValueError(f"Unexpected type {type(data).__name__}")

    if not isinstance(raw, list) or len(raw) != count:
        raise ValueError(f"Returned {len(raw)} vectors, expected {count}")

    vectors: list[Vector] = []
    for item in raw:
        if not isinstance(item, list) or not item:
            raise ValueError("Returned a non-vector element")
        vector = [float(x) for x in item]
        if expected_dim is not None and len(vector) != expected_dim:
            raise ValueError(f"Embedding dim {len(vector)} != expected {expected_dim}")
        vectors.append(vector)
    return vectors


def test_embed_conformance_team_ai() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            # Test team-ai default format
            res = await client.post("/embed", json={"texts": ["one", "two", "three"]})
            assert res.status_code == 200
            data = res.json()

            # Run conformance extractor
            vectors = _extract_vectors(data, count=3, expected_dim=3)
            assert len(vectors) == 3
            assert len(vectors[0]) == 3

    asyncio.run(_run())


def test_v1_embeddings_conformance_openai_shape() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            # Test OpenAI format
            res = await client.post("/v1/embeddings", json={"input": ["query 1", "query 2"]})
            assert res.status_code == 200
            data = res.json()

            # Run conformance extractor
            vectors = _extract_vectors(data, count=2, expected_dim=3)
            assert len(vectors) == 2
            assert len(vectors[0]) == 3

    asyncio.run(_run())
