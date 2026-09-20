"""FastAPI router application for platform-modelserve."""

from __future__ import annotations

import logging
import time
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

import httpx
from fastapi import FastAPI, HTTPException, Request, Response, status
from fastapi.responses import JSONResponse
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel

from modelserve.admission import (
    CACHE_HITS,
    CACHE_MISSES,
    REQUEST_LATENCY,
    AdmissionController,
)
from modelserve.cache import EmbeddingCache
from modelserve.config import Settings, settings

logger = logging.getLogger(__name__)


# Request Models
class EmbedRequest(BaseModel):
    texts: list[str] | None = None
    input: list[str] | str | None = None
    model: str | None = None


class RerankRequest(BaseModel):
    query: str
    texts: list[str]
    model: str | None = None
    top_k: int | None = None


def create_app(
    custom_settings: Settings | None = None,
    custom_cache: EmbeddingCache | None = None,
    http_client: httpx.AsyncClient | None = None,
) -> FastAPI:
    cfg = custom_settings or settings
    cache = custom_cache or EmbeddingCache(cfg)
    admission = AdmissionController(max_queue_depth=cfg.max_queue_depth)

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        await cache.connect()
        yield
        if app.state.http_client is not None:
            await app.state.http_client.aclose()
        await cache.close()

    app = FastAPI(
        title="platform-modelserve",
        description="Internal ML Model Serving Router (ADR-0011)",
        version="0.1.0",
        lifespan=lifespan,
    )

    app.state.http_client = http_client or httpx.AsyncClient(timeout=cfg.upstream_timeout_seconds)
    app.state.cache = cache
    app.state.admission = admission
    app.state.settings = cfg

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/metrics")
    async def metrics() -> Response:
        return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)

    @app.post("/embed")
    @app.post("/v1/embeddings")
    async def embed(request: EmbedRequest, req: Request) -> Any:
        start_time = time.perf_counter()
        texts: list[str] = []
        if request.texts is not None:
            texts = request.texts
        elif request.input is not None:
            if isinstance(request.input, str):
                texts = [request.input]
            else:
                texts = request.input

        if not texts:
            return {"embeddings": []}

        endpoint = "embed"
        async with admission.track(endpoint=endpoint):
            # Check Redis cache
            cached_vectors = await cache.get_many(texts, model_version=cfg.model_version)

            miss_indices = [i for i, v in enumerate(cached_vectors) if v is None]
            miss_texts = [texts[i] for i in miss_indices]

            if miss_texts:
                CACHE_MISSES.inc(len(miss_texts))
                # Forward uncached texts to upstream TEI embed
                client: httpx.AsyncClient = app.state.http_client
                try:
                    # TEI accepts POST /embed with {"inputs": [...]} or {"texts": [...]}
                    payload = {"inputs": miss_texts}
                    res = await client.post(
                        f"{cfg.tei_embed_url.rstrip('/')}/embed",
                        json=payload,
                    )
                except Exception as exc:
                    logger.error("Upstream TEI embed connection failed: %s", exc)
                    raise HTTPException(
                        status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                        detail=f"Upstream TEI embed unavailable: {exc}",
                    ) from exc

                if res.status_code != 200:
                    # Try fallback payload format if TEI uses /embeddings or /v1/embeddings
                    try:
                        res = await client.post(
                            f"{cfg.tei_embed_url.rstrip('/')}/v1/embeddings",
                            json={"input": miss_texts},
                        )
                    except Exception:
                        pass

                if res.status_code != 200:
                    logger.error("Upstream TEI returned status %d: %s", res.status_code, res.text)
                    raise HTTPException(
                        status_code=status.HTTP_502_BAD_GATEWAY,
                        detail=f"Upstream TEI embed returned {res.status_code}",
                    )

                data = res.json()
                new_vectors: list[list[float]] = []
                if isinstance(data, list):
                    new_vectors = data
                elif isinstance(data, dict) and "embeddings" in data:
                    new_vectors = data["embeddings"]
                elif isinstance(data, dict) and "data" in data:
                    new_vectors = [item["embedding"] for item in data["data"]]
                else:
                    raise HTTPException(
                        status_code=status.HTTP_502_BAD_GATEWAY,
                        detail="Unexpected response shape from upstream TEI embed",
                    )

                if len(new_vectors) != len(miss_texts):
                    raise HTTPException(
                        status_code=status.HTTP_502_BAD_GATEWAY,
                        detail=f"Expected {len(miss_texts)} vectors, got {len(new_vectors)}",
                    )

                # Store newly computed vectors in Redis
                await cache.set_many(
                    miss_texts,
                    new_vectors,
                    model_version=cfg.model_version,
                )

                # Reconstruct full result in original order
                for idx, vector in zip(miss_indices, new_vectors, strict=False):
                    cached_vectors[idx] = vector
            else:
                CACHE_HITS.inc(len(texts))

            duration = time.perf_counter() - start_time
            REQUEST_LATENCY.labels(endpoint=endpoint, status_code="200").observe(duration)

            # If client called /v1/embeddings (OpenAI format), return OpenAI structure
            if req.url.path.endswith("/v1/embeddings"):
                return {
                    "object": "list",
                    "data": [
                        {"object": "embedding", "index": i, "embedding": vec}
                        for i, vec in enumerate(cached_vectors)
                    ],
                    "model": request.model or cfg.model_version,
                }

            # Default team-ai format
            return {"embeddings": cached_vectors}

    @app.post("/rerank")
    async def rerank(request: RerankRequest) -> Any:
        endpoint = "rerank"
        async with admission.track(endpoint=endpoint):
            client: httpx.AsyncClient = app.state.http_client
            try:
                payload = {
                    "query": request.query,
                    "texts": request.texts,
                }
                if request.top_k is not None:
                    payload["top_k"] = request.top_k

                res = await client.post(
                    f"{cfg.tei_rerank_url.rstrip('/')}/rerank",
                    json=payload,
                )
            except Exception as exc:
                logger.error("Upstream TEI rerank connection failed: %s", exc)
                raise HTTPException(
                    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                    detail=f"Upstream TEI rerank unavailable: {exc}",
                ) from exc

            if res.status_code != 200:
                raise HTTPException(
                    status_code=status.HTTP_502_BAD_GATEWAY,
                    detail=f"Upstream TEI rerank returned {res.status_code}",
                )

            return res.json()

    @app.post("/v1/chat/completions")
    @app.post("/generate")
    async def generate(req: Request) -> Any:
        endpoint = "generate"
        async with admission.track(endpoint=endpoint):
            client: httpx.AsyncClient = app.state.http_client
            body = await req.json()
            try:
                target_path = req.url.path
                res = await client.post(
                    f"{cfg.vllm_url.rstrip('/')}{target_path}",
                    json=body,
                )
            except Exception as exc:
                logger.error("Upstream vLLM connection failed: %s", exc)
                raise HTTPException(
                    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                    detail=f"Upstream vLLM unavailable: {exc}",
                ) from exc

            return JSONResponse(
                content=res.json(),
                status_code=res.status_code,
            )

    return app


app = create_app()
