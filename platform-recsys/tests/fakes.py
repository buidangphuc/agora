"""In-memory fakes for the artifact stores, used by the local-mode smoke test
so the train→load→assert loop needs no real Qdrant/Redis containers.
"""

from __future__ import annotations

from types import SimpleNamespace


class FakeQdrantClient:
    def __init__(self):
        self.collections: dict[str, dict] = {}

    def get_collections(self):
        cols = [SimpleNamespace(name=n) for n in self.collections]
        return SimpleNamespace(collections=cols)

    def create_collection(self, collection_name, vectors_config):
        self.collections[collection_name] = {}

    def upsert(self, collection_name, points):
        store = self.collections.setdefault(collection_name, {})
        for p in points:
            store[p.id] = {"vector": p.vector, "payload": p.payload}

    def delete(self, collection_name, points_selector):
        # The job prunes points NOT matching the current model_version. Emulate by
        # dropping every point whose payload.model_version differs from the one in
        # the must_not filter condition.
        store = self.collections.get(collection_name, {})
        try:
            cond = points_selector.filter.must_not[0]
            keep_version = cond.match.value
        except Exception:  # pragma: no cover - defensive
            return
        for pid in [pid for pid, v in store.items() if v["payload"].get("model_version") != keep_version]:
            del store[pid]


class FakeRedis:
    def __init__(self):
        self.store: dict[str, str] = {}

    def set(self, key, value, ex=None):
        self.store[key] = value

    def pipeline(self):
        return _FakePipeline(self)


class _FakePipeline:
    def __init__(self, client: FakeRedis):
        self._client = client
        self._ops: list[tuple] = []

    def set(self, key, value, ex=None):
        self._ops.append((key, value))
        return self

    def execute(self):
        for key, value in self._ops:
            self._client.store[key] = value
        self._ops = []
