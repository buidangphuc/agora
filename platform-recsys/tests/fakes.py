"""In-memory fakes for the artifact stores, used by the local-mode smoke test
so the train→load→assert loop needs no real Qdrant/Redis containers.
"""

from __future__ import annotations

import fnmatch
from types import SimpleNamespace

from recsys.load import redis_cache


class FakeQdrantClient:
    """Collections, points and aliases of a Qdrant, in memory (just what the loaders call)."""

    def __init__(self):
        self.collections: dict[str, dict] = {}
        self.aliases: dict[str, str] = {}
        self.dims: dict[str, int | None] = {}

    def get_collections(self):
        cols = [SimpleNamespace(name=n) for n in self.collections]
        return SimpleNamespace(collections=cols)

    def get_aliases(self):
        return SimpleNamespace(
            aliases=[SimpleNamespace(alias_name=a, collection_name=c) for a, c in self.aliases.items()]
        )

    def _resolve(self, name):
        return self.aliases.get(name, name)

    def create_collection(self, collection_name, vectors_config):
        self.collections[collection_name] = {}
        self.dims[collection_name] = getattr(vectors_config, "size", None)

    def get_collection(self, collection_name):
        size = self.dims.get(self._resolve(collection_name))
        return SimpleNamespace(
            config=SimpleNamespace(params=SimpleNamespace(vectors=SimpleNamespace(size=size)))
        )

    def count(self, collection_name, exact=True):
        return SimpleNamespace(count=len(self.collections[self._resolve(collection_name)]))

    def delete_collection(self, collection_name):
        self.collections.pop(collection_name, None)
        # Qdrant drops the aliases of a deleted collection.
        self.aliases = {a: c for a, c in self.aliases.items() if c != collection_name}

    def update_collection_aliases(self, change_aliases_operations):
        for op in change_aliases_operations:
            if getattr(op, "delete_alias", None) is not None:
                self.aliases.pop(op.delete_alias.alias_name, None)
            elif getattr(op, "create_alias", None) is not None:
                ca = op.create_alias
                assert ca.collection_name in self.collections, "alias target must exist"
                assert ca.alias_name not in self.collections, "alias name taken by a real collection"
                self.aliases[ca.alias_name] = ca.collection_name

    def upsert(self, collection_name, points):
        name = self._resolve(collection_name)
        store = self.collections.setdefault(name, {})
        for p in points:
            size = self.dims.get(name)
            assert (
                size is None or len(p.vector) == size
            ), f"dim mismatch: expected {size}, got {len(p.vector)}"
            store[p.id] = {"vector": p.vector, "payload": p.payload}

    def scroll(self, collection_name, limit=10, offset=None, with_vectors=False, with_payload=True):
        store = self.collections[self._resolve(collection_name)]
        ids = list(store)
        start = offset or 0
        chunk = ids[start : start + limit]
        points = [
            SimpleNamespace(id=i, vector=store[i]["vector"], payload=store[i]["payload"]) for i in chunk
        ]
        nxt = start + limit if start + limit < len(ids) else None
        return points, nxt

    def delete(self, collection_name, points_selector):
        # The job prunes points NOT matching the current model_version. Emulate by
        # dropping every point whose payload.model_version differs from the one in
        # the must_not filter condition.
        store = self.collections.get(self._resolve(collection_name), {})
        try:
            cond = points_selector.filter.must_not[0]
            keep_version = cond.match.value
        except Exception:  # pragma: no cover - defensive
            return
        for pid in [pid for pid, v in store.items() if v["payload"].get("model_version") != keep_version]:
            del store[pid]


class FakeRedis:
    """Strings with TTLs, SCAN, and the two pointer scripts (emulated: the real Lua is exercised
    against fakeredis in test_generations.py)."""

    def __init__(self):
        self.store: dict[str, str] = {}
        self.ttl_set: dict[str, int | None] = {}
        self.evals = 0

    def set(self, key, value, ex=None):
        self.store[key] = value
        self.ttl_set[key] = ex

    def get(self, key):
        return self.store.get(key)

    def ttl(self, key):
        """Seconds left, -1 for no expiry, -2 for a missing key (the real command's contract)."""
        if key not in self.store:
            return -2
        return -1 if self.ttl_set.get(key) is None else self.ttl_set[key]

    def exists(self, key):
        return int(key in self.store)

    def delete(self, *keys):
        n = 0
        for k in keys:
            n += int(self.store.pop(k, None) is not None)
            self.ttl_set.pop(k, None)
        return n

    def expire(self, key, seconds):
        if key in self.store:
            self.ttl_set[key] = seconds
            return True
        return False

    def scan_iter(self, match="*", count=None):
        yield from [k for k in list(self.store) if fnmatch.fnmatchcase(k, match)]

    def ping(self):
        return True

    def eval(self, script, numkeys, *args):
        self.evals += 1
        keys, argv = args[:numkeys], args[numkeys:]
        serving, previous, mirror = keys
        if script == redis_cache._PROMOTE_LUA:
            old = self.store.get(serving)
            if old and old != argv[0]:
                self.store[previous] = old
            self.store[serving] = argv[0]
            self.store[mirror] = argv[0]
            return old or ""
        if script == redis_cache._ROLLBACK_LUA:
            s, p = self.store.get(serving), self.store.get(previous)
            if (s or "") != argv[0]:
                return 0
            if not p:
                return -1
            self.store[serving] = p
            if s:
                self.store[previous] = s
            else:
                self.store.pop(previous, None)
            self.store[mirror] = p
            return [s or "", p]
        raise AssertionError("unexpected script")  # pragma: no cover

    def pipeline(self):
        return _FakePipeline(self)


class _FakePipeline:
    def __init__(self, client: FakeRedis):
        self._client = client
        self._ops: list[tuple] = []

    def set(self, key, value, ex=None):
        self._ops.append((key, value, ex))
        return self

    def execute(self):
        for key, value, ex in self._ops:
            self._client.set(key, value, ex=ex)
        self._ops = []
