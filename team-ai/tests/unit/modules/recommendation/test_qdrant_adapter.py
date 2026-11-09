"""Qdrant adapter: producer point ids, a real timeout, and the collection check."""

import asyncio
import threading
from types import SimpleNamespace
from typing import Any

import qdrant_client
from qdrant_client import models
from qdrant_client.http.exceptions import UnexpectedResponse

from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.contract.qdrant import point_id


def _source(client: Any, **kw: Any) -> QdrantCandidateSource:
    return QdrantCandidateSource(
        client,
        collection="item_als_vectors",
        vector_dim=kw.pop("vector_dim", 4),
        distance=kw.pop("distance", "Cosine"),
        timeout_s=kw.pop("timeout_s", 1.0),
    )


async def _seeded_client(
    collection: str = "item_als_vectors__g1",
    *,
    distance: Any = None,
) -> Any:
    client = qdrant_client.AsyncQdrantClient(location=":memory:")
    await client.create_collection(
        collection_name=collection,
        vectors_config=models.VectorParams(
            size=4, distance=distance or models.Distance.COSINE
        ),
    )
    vectors = {
        "a": [1.0, 0.0, 0.0, 0.0],
        "b": [0.9, 0.1, 0.0, 0.0],
        "c": [0.0, 1.0, 0.0, 0.0],
    }
    await client.upsert(
        collection_name=collection,
        points=[
            models.PointStruct(
                id=point_id(listing),
                vector=vector,
                payload={"listing_id": listing, "model_version": "g1"},
            )
            for listing, vector in vectors.items()
        ],
    )
    return client


async def test_similar_queries_by_producer_point_id_in_the_generation_collection() -> (
    None
):
    client = await _seeded_client()
    got = await _source(client).similar("a", top_k=5, generation="g1")
    assert [c.listing_id for c in got] == ["b", "c"]  # the seed itself is excluded
    assert got[0].score > got[1].score
    assert all(c.in_stock for c in got)


async def test_similar_without_a_generation_reads_the_bare_collection_name() -> None:
    client = await _seeded_client("item_als_vectors")
    got = await _source(client).similar("a", top_k=1, generation=None)
    assert [c.listing_id for c in got] == ["b"]


async def test_similar_in_a_missing_collection_yields_nothing() -> None:
    client = await _seeded_client()
    assert await _source(client).similar("a", top_k=5, generation="g9") == []


class _SlowClient:
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.cancelled = False

    async def query_points(self, **kwargs: Any) -> Any:
        self.started.set()
        try:
            await asyncio.sleep(30)
        except asyncio.CancelledError:
            self.cancelled = True
            raise
        raise AssertionError("not reached")


async def test_a_slow_query_is_cancelled_and_yields_no_candidates() -> None:
    client = _SlowClient()
    threads_before = threading.active_count()
    got = await _source(client, timeout_s=0.02).similar("a", top_k=5, generation="g1")
    assert got == []
    assert client.cancelled  # the request was cancelled, not abandoned
    assert threading.active_count() == threads_before  # and no worker thread leaks


class _FailingClient:
    async def query_points(self, **kwargs: Any) -> Any:
        raise ConnectionError("qdrant down")


async def test_a_failing_query_yields_no_candidates() -> None:
    assert await _source(_FailingClient()).similar("a", top_k=5, generation="g1") == []


class _HitsClient:
    def __init__(self, *hits: Any) -> None:
        self.hits = hits
        self.kwargs: dict[str, Any] = {}

    async def query_points(self, **kwargs: Any) -> Any:
        self.kwargs = kwargs
        return SimpleNamespace(points=list(self.hits))


async def test_hits_map_payload_to_candidates_and_skip_those_without_a_listing_id() -> (
    None
):
    client = _HitsClient(
        SimpleNamespace(id="u1", score=0.9, payload={"listing_id": "L1"}),
        SimpleNamespace(
            id="u2", score=0.8, payload={"listing_id": "L2", "in_stock": False}
        ),
        SimpleNamespace(id="u3", score=0.7, payload=None),
    )
    got = await _source(client).similar("seed", top_k=7, generation="g1")
    assert [(c.listing_id, c.score, c.in_stock) for c in got] == [
        ("L1", 0.9, True),
        ("L2", 0.8, False),
    ]
    assert client.kwargs["collection_name"] == "item_als_vectors__g1"
    assert client.kwargs["limit"] == 7
    assert client.kwargs["query"].recommend.positive == [point_id("seed")]


async def test_ready_when_the_collection_matches_the_contract() -> None:
    client = await _seeded_client()
    assert await _source(client).ready("g1")


async def test_not_ready_when_the_collection_is_missing() -> None:
    client = await _seeded_client()
    assert not await _source(client).ready("g9")


async def test_not_ready_when_the_vector_size_differs() -> None:
    client = await _seeded_client()
    assert not await _source(client, vector_dim=64).ready("g1")


async def test_not_ready_when_the_distance_differs() -> None:
    client = await _seeded_client(distance=models.Distance.DOT)
    assert not await _source(client, distance="Cosine").ready("g1")
    assert await _source(client, distance="Dot").ready("g1")


async def test_not_ready_when_qdrant_is_unreachable() -> None:
    class Down:
        async def get_collection(self, **kwargs: Any) -> Any:
            raise ConnectionError("qdrant down")

    assert not await _source(Down()).ready("g1")


class _BatchClient:
    def __init__(self, responses: list[Any] | None = None, fail: bool = False) -> None:
        self.responses = responses or []
        self.fail = fail
        self.batch_kwargs: list[dict[str, Any]] = []
        self.single_calls = 0

    async def retrieve(
        self,
        collection_name: str,
        ids: list[Any],
        with_payload: bool = False,
        with_vectors: bool = False,
    ) -> Any:
        return [SimpleNamespace(id=pid, payload=None, vector=None) for pid in ids]

    async def query_batch_points(self, **kwargs: Any) -> Any:
        self.batch_kwargs.append(kwargs)
        if self.fail:
            raise ConnectionError("qdrant down")
        return self.responses

    async def query_points(self, **kwargs: Any) -> Any:
        self.single_calls += 1
        raise AssertionError("per-id query")


async def test_c21_similar_batch_issues_one_query_batch_points_call() -> None:
    client = _BatchClient(
        [
            SimpleNamespace(
                points=[SimpleNamespace(id="u", score=0.9, payload={"listing_id": "X"})]
            ),
            SimpleNamespace(points=[]),
            SimpleNamespace(
                points=[SimpleNamespace(id="v", score=0.4, payload={"listing_id": "Y"})]
            ),
        ]
    )
    got = await _source(client).similar_batch(["a", "b", "c"], top_k=7, generation="g1")
    assert len(client.batch_kwargs) == 1
    assert client.single_calls == 0
    kwargs = client.batch_kwargs[0]
    assert kwargs["collection_name"] == "item_als_vectors__g1"
    requests = kwargs["requests"]
    assert [r.query.recommend.positive for r in requests] == [
        [point_id("a")],
        [point_id("b")],
        [point_id("c")],
    ]
    assert all(r.limit == 7 for r in requests)
    assert {k: [c.listing_id for c in v] for k, v in got.items() if v} == {
        "a": ["X"],
        "c": ["Y"],
    }


async def test_c21_similar_batch_failure_yields_no_candidates() -> None:
    got = await _source(_BatchClient(fail=True)).similar_batch(
        ["a", "b"], top_k=5, generation="g1"
    )
    assert not any(got.values())


async def test_c21_similar_batch_slow_call_is_cancelled_within_one_budget() -> None:
    class _Slow:
        cancelled = False

        async def retrieve(
            self,
            collection_name: str,
            ids: list[Any],
            with_payload: bool = False,
            with_vectors: bool = False,
        ) -> Any:
            return [SimpleNamespace(id=pid, payload=None, vector=None) for pid in ids]

        async def query_batch_points(self, **kwargs: Any) -> Any:
            try:
                await asyncio.sleep(30)
            except asyncio.CancelledError:
                self.cancelled = True
                raise

    client = _Slow()
    got = await _source(client, timeout_s=0.02).similar_batch(
        ["a", "b"], top_k=5, generation="g1"
    )
    assert not any(got.values())
    assert client.cancelled


async def test_c21_similar_batch_of_no_ids_makes_no_request() -> None:
    client = _BatchClient()
    assert await _source(client).similar_batch([], top_k=5, generation="g1") == {}
    assert client.batch_kwargs == []


class _PartialIndexClient:
    def __init__(self, known: dict[str, str]) -> None:
        self.known = {point_id(lid): lid for lid in known}
        self.neighbours = known
        self.retrieve_kwargs: list[dict[str, Any]] = []
        self.batch_kwargs: list[dict[str, Any]] = []

    async def retrieve(self, **kwargs: Any) -> Any:
        self.retrieve_kwargs.append(kwargs)
        return [
            SimpleNamespace(id=pid, payload=None, vector=None)
            for pid in kwargs["ids"]
            if pid in self.known
        ]

    async def query_batch_points(self, **kwargs: Any) -> Any:
        self.batch_kwargs.append(kwargs)
        out = []
        for request in kwargs["requests"]:
            (positive,) = request.query.recommend.positive
            if positive not in self.known:
                raise UnexpectedResponse(
                    404, "Not Found", b'{"status":{"error":"Not found"}}', None
                )
            neighbour = self.neighbours[self.known[positive]]
            out.append(
                SimpleNamespace(
                    points=[
                        SimpleNamespace(
                            id="n", score=0.8, payload={"listing_id": neighbour}
                        )
                    ]
                )
            )
        return out


async def test_unknown_context_listing_is_skipped_known_ids_keep_their_neighbours() -> (
    None
):
    client = _PartialIndexClient({"A": "NA", "B": "NB"})
    got = await _source(client).similar_batch(
        ["A", "ghost", "B"], top_k=5, generation="g1"
    )
    assert [c.listing_id for c in got["A"]] == ["NA"]
    assert [c.listing_id for c in got["B"]] == ["NB"]
    assert got["ghost"] == []
    assert set(got) == {"A", "ghost", "B"}


async def test_unknown_context_listing_is_skipped_one_existence_check_then_one_batch_of_existing_ids() -> (
    None
):
    client = _PartialIndexClient({"A": "NA"})
    await _source(client).similar_batch(
        ["A", "ghost1", "ghost2"], top_k=5, generation="g1"
    )
    assert len(client.retrieve_kwargs) == 1
    retrieve = client.retrieve_kwargs[0]
    assert retrieve["collection_name"] == "item_als_vectors__g1"
    assert sorted(retrieve["ids"]) == sorted(
        point_id(x) for x in ("A", "ghost1", "ghost2")
    )
    assert retrieve["with_payload"] is False
    assert retrieve["with_vectors"] is False
    assert len(client.batch_kwargs) == 1
    assert [r.query.recommend.positive for r in client.batch_kwargs[0]["requests"]] == [
        [point_id("A")]
    ]


async def test_unknown_context_listing_is_skipped_all_unknown_yields_empty_lists() -> (
    None
):
    client = _PartialIndexClient({})
    got = await _source(client).similar_batch(
        ["ghost1", "ghost2"], top_k=5, generation="g1"
    )
    assert got == {"ghost1": [], "ghost2": []}
    assert len(client.retrieve_kwargs) == 1
    assert client.batch_kwargs == []


async def test_unknown_context_listing_is_skipped_slow_existence_check_is_cancelled_within_the_budget() -> (
    None
):
    class _SlowRetrieve:
        cancelled = False
        batch_calls = 0

        async def retrieve(self, **kwargs: Any) -> Any:
            try:
                await asyncio.sleep(30)
            except asyncio.CancelledError:
                self.cancelled = True
                raise

        async def query_batch_points(self, **kwargs: Any) -> Any:
            self.batch_calls += 1
            return []

    client = _SlowRetrieve()
    got = await _source(client, timeout_s=0.02).similar_batch(
        ["A", "B"], top_k=5, generation="g1"
    )
    assert got == {"A": [], "B": []}
    assert client.cancelled
    assert client.batch_calls == 0
