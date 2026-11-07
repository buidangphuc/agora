"""Fixtures and probes of recommendations/cart_cross_sell.feature (change recsys-cart-cross-sell).

Black box: the catalogue of a scenario is written straight into the serving generation's item
collection (Qdrant payload `seller_id`, `category_id`, `visible`, `in_stock`, tagged `e2e_run`) and
the serving popular list (Redis), with `visible=true` on every candidate so fail-closed does not
empty the answer. Every point and key the scenario did not write is hidden (`visible=false`) or
overwritten for its duration and put back exactly in teardown; scenarios using it are @destructive.
Answers are read through the gateway as the logged-in buyer; expectations come from Qdrant's own
similarity scores.
"""

from __future__ import annotations

import json
import math
import re
import time
import uuid
from collections.abc import Callable
from datetime import datetime, timezone
from typing import Any

from src.api.services import GatewayError
from src.api.services.recommendation_service import CONTEXT_HOMEPAGE
from src.constants import gateway_endpoints as ep
from tests.e2e.step_definitions.ssa_steps import ssa_point_id, ssa_qdrant
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

PLACEMENT = "cart_cross_sell"
CLUSTER = 10
RESULT_LIMIT = 8
SETTLE_S = 20.0
POLL_S = 0.4
_ABSENT = "__absent__"
_BATCH = 500


def state(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("cxs", {})


def unit(dim: int, coords: dict[int, float]) -> list[float]:
    vec = [0.0] * dim
    for axis, value in coords.items():
        vec[axis] = value
    norm = math.sqrt(sum(v * v for v in vec))
    return [v / norm for v in vec]


def recommend(world: World, payload: dict[str, Any]) -> dict[str, Any]:
    return world.service_factory.recommendation.post(ep.RECOMMENDATION_RECOMMEND, payload)


def ranked(answer: dict[str, Any]) -> list[dict[str, Any]]:
    return sorted(answer.get("items") or [], key=lambda item: item.get("rank", 0))


def ids_of(answer: dict[str, Any]) -> list[str]:
    return [item["listingId"] for item in ranked(answer)]


def recommend_until(
    world: World,
    payload: dict[str, Any],
    accept: Callable[[list[str]], bool],
    what: str,
    timeout_s: float = SETTLE_S,
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while True:
        try:
            last = recommend(world, payload)
            if accept(ids_of(last)):
                return last
        except GatewayError as exc:
            last = exc
        assert (
            time.monotonic() < deadline
        ), f"{what}: never answered as expected within {timeout_s:.0f}s; last answer: {last!r}"
        time.sleep(POLL_S)


def warm_up(world: World, payload: dict[str, Any], attempts: int = 6) -> None:
    for _ in range(attempts):
        try:
            recommend(world, payload)
        except GatewayError:
            pass
        time.sleep(POLL_S)


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def ai_lines_since(needle: str, since: str) -> list[str]:
    out = pe.docker("logs", "--since", since, pe.ai_container(), check=False)
    return [line for line in (out.stdout + out.stderr).splitlines() if needle in line]


def point_filter(run: str) -> dict[str, Any]:
    return {"must": [{"key": "e2e_run", "match": {"value": run}}]}


class Catalogue:
    def __init__(self, world: World) -> None:
        self.world = world
        self.run = uuid.uuid4().hex[:8]
        data = recommend(world, {"context": CONTEXT_HOMEPAGE, "limit": 5})
        version = data.get("modelVersion", "")
        assert re.fullmatch(r"als-.+", version), f"no trained generation is serving: {data}"
        self.model_version = version
        self.collection = f"item_als_vectors__{version}"
        config = ssa_qdrant("GET", f"/collections/{self.collection}")["result"]["config"]
        vectors = config["params"]["vectors"]
        self.dim = int(
            vectors["size"] if "size" in vectors else next(iter(vectors.values()))["size"]
        )
        assert self.dim >= 3, f"{self.collection} vectors have {self.dim} dimensions, need 3"
        self.points: dict[str, dict[str, Any]] = {}
        self.popular: list[dict[str, Any]] = []
        self._hide_others()
        world.add_cleanup(self._delete_mine)

    def _scroll_others(self) -> list[tuple[str, dict[str, Any]]]:
        found: list[tuple[str, dict[str, Any]]] = []
        offset = None
        while True:
            body: dict[str, Any] = {"limit": 1000, "with_payload": True, "with_vector": False}
            if offset is not None:
                body["offset"] = offset
            result = ssa_qdrant("POST", f"/collections/{self.collection}/points/scroll", body)[
                "result"
            ]
            for point in result["points"]:
                found.append((str(point["id"]), point.get("payload") or {}))
            offset = result.get("next_page_offset")
            if offset is None:
                return found

    def _hide_others(self) -> None:
        groups: dict[Any, list[str]] = {True: [], False: [], _ABSENT: []}
        for point_id, payload in self._scroll_others():
            groups[payload["visible"] if "visible" in payload else _ABSENT].append(point_id)
        everything = [pid for ids in groups.values() for pid in ids]
        self.world.add_cleanup(lambda: self._restore(groups))
        for start in range(0, len(everything), _BATCH):
            self._set_payload({"visible": False}, everything[start : start + _BATCH])

    def _restore(self, groups: dict[Any, list[str]]) -> None:
        for value, ids in groups.items():
            for start in range(0, len(ids), _BATCH):
                batch = ids[start : start + _BATCH]
                if value == _ABSENT:
                    ssa_qdrant(
                        "POST",
                        f"/collections/{self.collection}/points/payload/delete?wait=true",
                        {"keys": ["visible"], "points": batch},
                    )
                else:
                    self._set_payload({"visible": value}, batch)

    def _delete_mine(self) -> None:
        ssa_qdrant(
            "POST",
            f"/collections/{self.collection}/points/delete?wait=true",
            {"filter": point_filter(self.run)},
        )

    def _set_payload(self, payload: dict[str, Any], point_ids: list[str]) -> None:
        ssa_qdrant(
            "POST",
            f"/collections/{self.collection}/points/payload?wait=true",
            {"payload": payload, "points": point_ids},
        )

    def add(
        self,
        listing_id: str,
        vector: list[float],
        *,
        seller_id: str | None,
        category_id: str | None,
        visible: bool = True,
        in_stock: bool | None = None,
    ) -> None:
        payload: dict[str, Any] = {
            "listing_id": listing_id,
            "e2e_run": self.run,
            "visible": visible,
        }
        if seller_id is not None:
            payload["seller_id"] = seller_id
        if category_id is not None:
            payload["category_id"] = category_id
        if in_stock is not None:
            payload["in_stock"] = in_stock
        ssa_qdrant(
            "PUT",
            f"/collections/{self.collection}/points?wait=true",
            {"points": [{"id": ssa_point_id(listing_id), "vector": vector, "payload": payload}]},
        )
        self.points[listing_id] = payload

    def patch(self, listing_ids: list[str], **payload: Any) -> None:
        self._set_payload(payload, [ssa_point_id(lid) for lid in listing_ids])
        for lid in listing_ids:
            self.points[lid].update(payload)

    def add_cluster(
        self,
        centre: int,
        ids: list[str],
        *,
        category_id: str,
        sellers: list[str] | None = None,
    ) -> None:
        for n, listing_id in enumerate(ids):
            vector = unit(self.dim, {centre: 1.0, 2 if centre != 2 else 1: 0.03 * (n + 1)})
            self.add(
                listing_id,
                vector,
                seller_id=(sellers[n] if sellers else f"cx-{self.run}-seller-{listing_id}"),
                category_id=category_id,
            )

    def set_popular(self, entries: list[dict[str, Any]]) -> None:
        redis = rss.keys(self.world, rss.SERVING_DB)
        redis.put(f"{rss.serving_prefix(redis)}:popular", json.dumps(entries))
        self.popular = entries

    def similarities(self, listing_id: str) -> dict[str, float]:
        result = ssa_qdrant(
            "POST",
            f"/collections/{self.collection}/points/query",
            {
                "query": ssa_point_id(listing_id),
                "filter": point_filter(self.run),
                "limit": 200,
                "with_payload": True,
            },
        )["result"]["points"]
        return {p["payload"]["listing_id"]: float(p["score"]) for p in result}

    def neighbours(self, listing_id: str, top: int = CLUSTER) -> list[str]:
        sims = self.similarities(listing_id)
        return sorted(sims, key=lambda lid: -sims[lid])[:top]

    def eligible(self, listing_id: str) -> bool:
        payload = self.points[listing_id]
        return bool(payload.get("visible")) and payload.get("in_stock", True) is not False


class Standard:
    def __init__(self, cat: Catalogue) -> None:
        run = cat.run
        self.cat = cat
        self.a = f"cx-{run}-A"
        self.b = f"cx-{run}-B"
        self.category_c = f"cx-{run}-cat-C"
        self.category_d = f"cx-{run}-cat-D"
        self.a_cluster = [f"cx-{run}-a{n}" for n in range(CLUSTER)]
        self.b_cluster = [f"cx-{run}-b{n}" for n in range(CLUSTER)]
        self.popular_c = [f"cx-{run}-pc{n}" for n in range(4)]
        self.popular_d = [f"cx-{run}-pd{n}" for n in range(4)]
        self.popular_none = f"cx-{run}-pn"
        dim = cat.dim
        cat.add(
            self.a,
            unit(dim, {0: 1.0}),
            seller_id=f"cx-{run}-seller-A",
            category_id=self.category_c,
        )
        cat.add(
            self.b,
            unit(dim, {0: 0.6, 1: 1.0}),
            seller_id=f"cx-{run}-seller-B",
            category_id=self.category_d,
        )
        cat.add_cluster(0, self.a_cluster, category_id=self.category_c)
        for n, listing_id in enumerate(self.b_cluster):
            cat.add(
                listing_id,
                unit(dim, {0: 0.6, 1: 1.0, 2: 0.03 * (n + 1)}),
                seller_id=f"cx-{run}-seller-{listing_id}",
                category_id=self.category_d,
            )
        entries: list[dict[str, Any]] = []
        score = 100.0
        for n in range(4):
            for listing_id, category in (
                (self.popular_c[n], self.category_c),
                (self.popular_d[n], self.category_d),
            ):
                entries.append(
                    {
                        "listing_id": listing_id,
                        "score": score,
                        "seller_id": f"cx-{run}-seller-{listing_id}",
                        "category_id": category,
                        "visible": True,
                    }
                )
                score -= 1.0
            if n == 1:
                entries.append(
                    {
                        "listing_id": self.popular_none,
                        "score": score,
                        "seller_id": f"cx-{run}-seller-{self.popular_none}",
                        "visible": True,
                    }
                )
                score -= 1.0
        cat.set_popular(entries)

    def similar_allowed(self, contexts: list[str]) -> set[str]:
        allowed: set[str] = set()
        for context in contexts:
            allowed |= set(self.cat.neighbours(context))
        return {lid for lid in allowed if lid not in contexts and self.cat.eligible(lid)}

    def popular_ids(self, *categories: str) -> set[str]:
        return {
            e["listing_id"]
            for e in self.cat.popular
            if e.get("visible") and e.get("category_id") in categories
        }


def accepts(allowed: set[str]) -> Callable[[list[str]], bool]:
    return lambda ids: bool(ids) and set(ids) <= allowed
