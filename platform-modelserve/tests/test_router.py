"""Integration tests for router endpoints."""

from __future__ import annotations

import asyncio

import httpx
from httpx import ASGITransport

from modelserve.cache import EmbeddingCache
from modelserve.config import Settings
from modelserve.router import create_app
from tests.test_cache import FakeRedis


class MockUpstreamTransport(httpx.AsyncBaseTransport):
    """Mocks upstream TEI and vLLM responses."""

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        url_str = str(request.url)
        if "/embed" in url_str:
            import json

            body = json.loads(request.content.decode("utf-8"))
            inputs = body.get("inputs", [])
            # Return dummy vectors of dim 3
            vectors = [[0.1, 0.2, 0.3] for _ in inputs]
            return httpx.Response(200, json=vectors)
        elif "/rerank" in url_str:
            import json

            body = json.loads(request.content.decode("utf-8"))
            texts = body.get("texts", [])
            results = [{"index": i, "score": 0.9 - (0.1 * i)} for i in range(len(texts))]
            return httpx.Response(200, json=results)
        elif "/v1/chat/completions" in url_str or "/generate" in url_str:
            return httpx.Response(
                200,
                json={
                    "id": "chatcmpl-test",
                    "choices": [{"message": {"role": "assistant", "content": "mock answer"}}],
                },
            )
        return httpx.Response(404, json={"error": "not found"})


def get_test_app():
    fake_redis = FakeRedis()
    settings = Settings(
        embed_cache_enabled=True,
        model_version="test-v1",
        tei_embed_url="http://mock-tei:8101",
        tei_rerank_url="http://mock-tei:8102",
        vllm_url="http://mock-vllm:8103",
    )
    cache = EmbeddingCache(settings, redis_client=fake_redis)
    mock_upstream_client = httpx.AsyncClient(transport=MockUpstreamTransport())
    app = create_app(
        custom_settings=settings,
        custom_cache=cache,
        http_client=mock_upstream_client,
    )
    return app


def test_healthz() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            res = await client.get("/healthz")
            assert res.status_code == 200
            assert res.json() == {"status": "ok"}

    asyncio.run(_run())


def test_metrics() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            res = await client.get("/metrics")
            assert res.status_code == 200
            assert b"modelserve_in_flight_requests" in res.content

    asyncio.run(_run())


def test_embed_endpoint() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            # First request -> computed from upstream
            res1 = await client.post("/embed", json={"texts": ["foo", "bar"]})
            assert res1.status_code == 200
            data1 = res1.json()
            assert "embeddings" in data1
            assert len(data1["embeddings"]) == 2
            assert len(data1["embeddings"][0]) == 3

            # Second request with same inputs -> served from cache
            res2 = await client.post("/embed", json={"texts": ["foo", "bar"]})
            assert res2.status_code == 200
            assert res2.json() == data1

    asyncio.run(_run())


def test_v1_embeddings_openai_format() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            res = await client.post("/v1/embeddings", json={"input": ["test sentence"]})
            assert res.status_code == 200
            data = res.json()
            assert "data" in data
            assert len(data["data"]) == 1
            assert "embedding" in data["data"][0]

    asyncio.run(_run())


def test_rerank_endpoint() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            res = await client.post(
                "/rerank",
                json={"query": "shoes", "texts": ["red sneaker", "blue hat"]},
            )
            assert res.status_code == 200
            results = res.json()
            assert len(results) == 2
            assert results[0]["score"] >= results[1]["score"]

    asyncio.run(_run())


def test_generate_endpoint() -> None:
    async def _run():
        app = get_test_app()
        transport = ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            res = await client.post(
                "/v1/chat/completions",
                json={"messages": [{"role": "user", "content": "hello"}]},
            )
            assert res.status_code == 200
            data = res.json()
            assert data["choices"][0]["message"]["content"] == "mock answer"

    asyncio.run(_run())
