"""Unit tests for model_server embedding adapter (ML/LLM decoupling seam)."""

import pytest

from app.core.errors import ServiceUnavailableError
from app.modules.ai.rag.embeddings import _extract_vectors, embed_texts


def test_extract_vectors_from_embeddings_dict():
    payload = {"embeddings": [[0.1, 0.2, 0.3], [0.4, 0.5, 0.6]]}
    vectors = _extract_vectors(payload, count=2, expected_dim=3)
    assert len(vectors) == 2
    assert vectors[0] == [0.1, 0.2, 0.3]
    assert vectors[1] == [0.4, 0.5, 0.6]


def test_extract_vectors_from_openai_shape():
    payload = {
        "data": [
            {"embedding": [0.1, 0.2, 0.3]},
            {"embedding": [0.4, 0.5, 0.6]},
        ]
    }
    vectors = _extract_vectors(payload, count=2, expected_dim=3)
    assert len(vectors) == 2
    assert vectors[0] == [0.1, 0.2, 0.3]


def test_extract_vectors_dimension_mismatch_raises():
    payload = {"embeddings": [[0.1, 0.2]]}
    with pytest.raises(ServiceUnavailableError, match=r"Embedding dim 2 != expected 3"):
        _extract_vectors(payload, count=1, expected_dim=3)


def test_extract_vectors_count_mismatch_raises():
    payload = {"embeddings": [[0.1, 0.2, 0.3]]}
    with pytest.raises(ServiceUnavailableError, match=r"returned 1 vectors, expected 2"):
        _extract_vectors(payload, count=2, expected_dim=3)


class DummySyncClient:
    def post(self, path: str, json: dict | None = None):
        assert path == "/embed"
        texts = json.get("texts", []) if json else []
        class DummyResponse:
            status_code = 200
            def json(self, *args, **kwargs):
                return {"embeddings": [[0.1 * (i + 1)] * 4 for i in range(len(texts))]}
        return DummyResponse()


def test_embed_texts_sync_roundtrip():
    client = DummySyncClient()
    vectors = embed_texts(client, ["hello", "world"], path="/embed", expected_dim=4)
    assert len(vectors) == 2
    assert len(vectors[0]) == 4
