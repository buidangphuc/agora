"""Model-server embedding: one request/parse path, three accepted reply shapes."""

import json

import httpx
import pytest

from app.core.errors import ServiceUnavailableError
from app.modules.ai.rag.embeddings import ModelServerEmbedding

VECTORS = [[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]]


def _embedding(
    transport: httpx.MockTransport | None = None,
    *,
    reply: object = None,
    status: int = 200,
    dim: int = 3,
) -> tuple[ModelServerEmbedding, list[httpx.Request]]:
    seen: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(status, json=reply)

    transport = transport or httpx.MockTransport(respond)
    embedding = ModelServerEmbedding(
        sync_client=httpx.Client(base_url="http://embed", transport=transport),
        async_client=httpx.AsyncClient(base_url="http://embed", transport=transport),
        path="/embed",
        embed_dim=dim,
    )
    return embedding, seen


@pytest.mark.parametrize(
    "reply",
    [
        {"embeddings": VECTORS},
        {"data": [{"embedding": v} for v in VECTORS]},
        VECTORS,
    ],
    ids=["embeddings", "openai", "bare"],
)
async def test_every_reply_shape_parses_to_the_same_vectors(reply: object) -> None:
    embedding, seen = _embedding(reply=reply)

    sync = embedding._get_text_embeddings(["a", "b"])  # pyright: ignore[reportPrivateUsage]
    async_ = await embedding._aget_text_embeddings(["a", "b"])  # pyright: ignore[reportPrivateUsage]

    assert sync == async_ == VECTORS
    assert [str(r.url) for r in seen] == ["http://embed/embed"] * 2
    assert json.loads(seen[0].content) == {"texts": ["a", "b"]}
    await embedding.aclose()


async def test_non_200_is_service_unavailable() -> None:
    embedding, _ = _embedding(reply={}, status=500)

    with pytest.raises(ServiceUnavailableError):
        await embedding._aget_text_embeddings(["a"])  # pyright: ignore[reportPrivateUsage]
    with pytest.raises(ServiceUnavailableError):
        embedding._get_text_embeddings(["a"])  # pyright: ignore[reportPrivateUsage]


async def test_vector_count_mismatch_is_service_unavailable() -> None:
    embedding, _ = _embedding(reply={"embeddings": VECTORS[:1]})

    with pytest.raises(ServiceUnavailableError):
        await embedding._aget_text_embeddings(["a", "b"])  # pyright: ignore[reportPrivateUsage]


async def test_dimension_mismatch_is_service_unavailable() -> None:
    embedding, _ = _embedding(reply={"embeddings": VECTORS}, dim=4)

    with pytest.raises(ServiceUnavailableError):
        embedding._get_text_embeddings(["a", "b"])  # pyright: ignore[reportPrivateUsage]


async def test_unreachable_server_is_service_unavailable() -> None:
    def boom(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("down", request=request)

    embedding, _ = _embedding(httpx.MockTransport(boom))

    with pytest.raises(ServiceUnavailableError):
        await embedding._aget_text_embeddings(["a"])  # pyright: ignore[reportPrivateUsage]
    with pytest.raises(ServiceUnavailableError):
        embedding._get_text_embeddings(["a"])  # pyright: ignore[reportPrivateUsage]
