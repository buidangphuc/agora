import asyncio
import hashlib
import json
import logging
import time
from collections.abc import Callable

import httpx
import pytest

from app.modules.business.recommendation.adapters.flipt import FliptExperimentAssigner
from app.modules.business.recommendation.placements import load_placements
from app.modules.business.recommendation.ports import Assignment

FLAG = "recs-home-feed-ranker"
SALT = "pepper-salt-9f3"


def _assigner(
    handler: Callable[[httpx.Request], httpx.Response], *, timeout_s: float = 0.5
) -> FliptExperimentAssigner:
    client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), base_url="http://flipt:8080"
    )
    return FliptExperimentAssigner(client, salt=SALT, timeout_s=timeout_s)


def _match(variant: str = "similarity") -> Callable[[httpx.Request], httpx.Response]:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "match": True,
                "variantKey": variant,
                "variantAttachment": json.dumps({"ranking": {"model": "cosine_rank"}}),
            },
        )

    return handler


def test_placement_without_flag_makes_no_call():
    placements = load_placements()
    assert placements.get("similar_items").experiment is None
    assert placements.get("home_feed").experiment is not None


async def test_placement_without_flag_makes_no_call_empty_flag_is_no_call():
    calls: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"match": False})

    result = await _assigner(handler).assign(None, "u1")

    assert result.status == "none"
    assert calls == []


async def test_no_identity_no_arm():
    calls: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(200, json={"match": True, "variantKey": "control"})

    for entity in (None, ""):
        result = await _assigner(handler).assign(FLAG, entity)
        assert result.status == "none"
        assert result.variant is None
    assert calls == []


async def test_slow_flipt_times_out():
    gate = asyncio.Event()

    async def handler(request: httpx.Request) -> httpx.Response:
        await gate.wait()
        return httpx.Response(200, json={"match": True, "variantKey": "control"})

    started = time.monotonic()
    result = await _assigner(handler, timeout_s=0.02).assign(FLAG, "u1")

    assert result.status == "unassigned"
    assert result.variant is None
    assert time.monotonic() - started < 0.15


async def test_entity_id_is_salted_hash_and_not_logged(
    caplog: pytest.LogCaptureFixture, capsys: pytest.CaptureFixture[str]
):
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return _match()(request)

    with caplog.at_level(logging.DEBUG):
        result = await _assigner(handler).assign(FLAG, "u1")

    expected = hashlib.sha256(f"{SALT}:u1".encode()).hexdigest()
    assert len(seen) == 1
    body = json.loads(seen[0].content)
    assert body["entityId"] == expected
    assert body["flagKey"] == FLAG
    assert body["namespaceKey"] == "default"
    assert seen[0].method == "POST"
    raw_request = seen[0].url.path + str(seen[0].url.query) + seen[0].content.decode()
    assert "u1" not in raw_request.replace(expected, "")
    assert SALT not in raw_request
    assert result == Assignment("similarity", "cosine_rank", "assigned")

    out = capsys.readouterr()
    logged = caplog.text + out.out + out.err
    assert "u1" not in logged.replace("recs-home-feed-ranker", "")
    assert SALT not in logged
    assert expected not in logged


async def test_entity_hash_is_stable_visitor_always_lands_in_same_arm():
    entities: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        entities.append(json.loads(request.content)["entityId"])
        return _match()(request)

    assigner = _assigner(handler)
    for _ in range(3):
        await assigner.assign(FLAG, "anon-7")
    await assigner.assign(FLAG, "anon-8")

    assert entities[0] == entities[1] == entities[2]
    assert entities[3] != entities[0]


async def test_flipt_500_is_unassigned():
    result = await _assigner(lambda r: httpx.Response(500, text="boom")).assign(
        FLAG, "u1"
    )

    assert result.status == "unassigned"
    assert result.variant is None


async def test_unreachable_flipt_is_unassigned_flipt_unreachable_serves_default():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("refused", request=request)

    result = await _assigner(handler).assign(FLAG, "u1")

    assert result.status == "unassigned"


async def test_unparsable_body_is_unassigned_flipt_unreachable_serves_default():
    result = await _assigner(lambda r: httpx.Response(200, text="<html>")).assign(
        FLAG, "u1"
    )

    assert result.status == "unassigned"


async def test_disabled_flag_means_no_arm():
    result = await _assigner(lambda r: httpx.Response(200, json={"match": False})).assign(
        FLAG, "u1"
    )

    assert result.status == "none"
    assert result.variant is None


async def test_missing_flag_404_means_no_arm_disabled_flag_means_no_arm():
    result = await _assigner(lambda r: httpx.Response(404, json={})).assign(FLAG, "u1")

    assert result.status == "none"


async def test_matched_variant_without_attachment_has_no_ranking_model_visitor_always_lands_in_same_arm():
    result = await _assigner(
        lambda r: httpx.Response(200, json={"match": True, "variantKey": "control"})
    ).assign(FLAG, "u1")

    assert result == Assignment("control", None, "assigned")
