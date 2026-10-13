"""Remote embedding adapter: vectorization is delegated to a model server.

Wire shape: ``POST {path}  {"texts": ["a", "b"]}``. The reply is one of
``{"embeddings": [[...]]}``, the OpenAI-style ``{"data": [{"embedding": [...]}]}``
or a bare ``[[...]]``. Sync (llama-index indexing) and async (retrieval) calls
share one request body and one parser; only the transport call differs.
"""

from __future__ import annotations

from typing import Any

import httpx
from llama_index.core.base.embeddings.base import BaseEmbedding
from pydantic import PrivateAttr

from app.core.errors import ServiceUnavailableError

Vector = list[float]


def _bad_reply(message: str, code: str = "embedding_server_bad_response") -> Exception:
    return ServiceUnavailableError(message=message, code=code)


def _raw_vectors(data: Any) -> Any:
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        if "embeddings" in data:
            return data["embeddings"]
        if isinstance(data.get("data"), list):
            return [item.get("embedding") for item in data["data"]]
    raise _bad_reply("Embedding server reply has an unknown shape")


def parse_embeddings(data: Any, *, count: int, dim: int) -> list[Vector]:
    """Validate a server reply into ``count`` float vectors of size ``dim``."""
    raw = _raw_vectors(data)
    if not isinstance(raw, list) or len(raw) != count:
        raise _bad_reply(f"Embedding server did not return {count} vectors")
    vectors: list[Vector] = []
    for item in raw:
        try:
            vector = [float(x) for x in item]
        except (TypeError, ValueError) as exc:
            raise _bad_reply("Embedding server returned a non-vector element") from exc
        if len(vector) != dim:
            raise _bad_reply(
                f"Embedding dim {len(vector)} != expected {dim}",
                "embedding_server_dim_mismatch",
            )
        vectors.append(vector)
    return vectors


def _unreachable() -> Exception:
    return ServiceUnavailableError(
        message="Embedding server is unreachable",
        code="embedding_server_unavailable",
    )


class ModelServerEmbedding(BaseEmbedding):
    """``BaseEmbedding`` backed by the model server; owns both httpx clients."""

    _sync_client: httpx.Client = PrivateAttr()
    _async_client: httpx.AsyncClient = PrivateAttr()
    _path: str = PrivateAttr()
    _dim: int = PrivateAttr()

    def __init__(
        self,
        *,
        sync_client: httpx.Client,
        async_client: httpx.AsyncClient,
        path: str,
        embed_dim: int,
    ) -> None:
        super().__init__(model_name="model-server")
        self._sync_client = sync_client
        self._async_client = async_client
        self._path = path
        self._dim = embed_dim

    def _reply(self, texts: list[str], response: httpx.Response) -> list[Vector]:
        if response.status_code != 200:
            raise ServiceUnavailableError(
                message=f"Embedding server returned {response.status_code}",
                code="embedding_server_error",
            )
        return parse_embeddings(response.json(), count=len(texts), dim=self._dim)

    def _get_text_embeddings(self, texts: list[str]) -> list[Vector]:
        try:
            response = self._sync_client.post(self._path, json={"texts": texts})
        except httpx.HTTPError as exc:
            raise _unreachable() from exc
        return self._reply(texts, response)

    async def _aget_text_embeddings(self, texts: list[str]) -> list[Vector]:
        try:
            response = await self._async_client.post(self._path, json={"texts": texts})
        except httpx.HTTPError as exc:
            raise _unreachable() from exc
        return self._reply(texts, response)

    def _get_text_embedding(self, text: str) -> Vector:
        return self._get_text_embeddings([text])[0]

    def _get_query_embedding(self, query: str) -> Vector:
        return self._get_text_embeddings([query])[0]

    async def _aget_text_embedding(self, text: str) -> Vector:
        return (await self._aget_text_embeddings([text]))[0]

    async def _aget_query_embedding(self, query: str) -> Vector:
        return (await self._aget_text_embeddings([query]))[0]

    async def aclose(self) -> None:
        await self._async_client.aclose()
        self._sync_client.close()
