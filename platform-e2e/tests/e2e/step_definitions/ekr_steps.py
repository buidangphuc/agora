"""Steps for engagement/consumer_key_redaction.feature (change engagement-consumer-key-redaction)."""

from __future__ import annotations

import re
import time

import pytest
from pytest_bdd import given, then, when

from tests.e2e.support import ekr_support as k


@pytest.fixture
def ekr() -> dict:
    return {}


def _start(ekr: dict) -> None:
    ekr["since_s"] = int(time.time()) - 2
    ekr["since_ms"] = int(time.time() * 1000) - 2000


def _has_key_attr(line: dict) -> bool:
    return "key" in line


def _lines_for(ekr: dict, fps: set[str], msg: str) -> list[dict]:
    return [m for m in k.log_lines(ekr["since_s"]) if m.get("msg") == msg and m.get("key_fp") in fps]


def _no_raw(ekr: dict, keys: list[bytes]) -> None:
    text = k.raw_logs(ekr["since_s"])
    for key in keys:
        assert key.decode() not in text, f"raw key {key[:24]!r} appears in {k.container()} logs"


# ── givens / whens ────────────────────────────────────────────────────────
@given("a poison record keyed by a seller id is published to listing.events")
def given_poison(ekr: dict) -> None:
    _start(ekr)
    key = k.new_user_key().encode()
    ekr["keys"] = [key]
    k.poison(key)


@given("a poison record with a nil key is published to listing.events")
def given_nil(ekr: dict) -> None:
    _start(ekr)
    ekr["keys"] = [b""]
    k.poison(None)


@given("two poison records with the same key and one with a different 200-byte key are published to listing.events")
def given_three(ekr: dict) -> None:
    _start(ekr)
    same = k.new_user_key("user").encode()
    other = (k.new_user_key("user") + "x" * 200).encode()[:200]
    ekr["keys"] = [same, same, other]
    for key in ekr["keys"]:
        k.poison(key)


@when("the consumer has retried and parked it")
def when_parked(ekr: dict) -> None:
    ekr["parked"] = k.wait_parked(ekr["since_s"], ekr["keys"])


# ── thens ─────────────────────────────────────────────────────────────────
@then("the handler failure log carries key_fp equal to the first 12 hex of SHA-256 of the key and the logs never contain the raw key")
def then_failed(ekr: dict) -> None:
    key = ekr["keys"][0]
    assert k.log_lines(ekr["since_s"]), "positive control: no engagement logs at all"
    hits = _lines_for(ekr, {k.fp(key)}, k.FAILED_MSG)
    assert hits, f"no '{k.FAILED_MSG}' record with key_fp={k.fp(key)}"
    assert all(h.get("attempt") is not None for h in hits)
    _no_raw(ekr, [key])


@then("the parking log carries key_fp and attempts, has no key attribute, the logs never contain the raw key and the DLQ record key is the raw key")
def then_parked(ekr: dict) -> None:
    key = ekr["keys"][0]
    hits = _lines_for(ekr, {k.fp(key)}, k.PARKED_MSG)
    assert hits, f"no '{k.PARKED_MSG}' record with key_fp={k.fp(key)}; saw {ekr['parked']}"
    assert all("attempts" in h for h in hits)
    assert not any(_has_key_attr(m) for m in ekr["parked"]), "a parking record still has `key`"
    _no_raw(ekr, [key])
    assert k.dlq_keys(ekr["since_ms"], {key}) == {key}, "DLQ record key is not the raw key"


@then("the parking log carries key_fp e3b0c44298fc and no key attribute")
def then_nil(ekr: dict) -> None:
    hits = _lines_for(ekr, {"e3b0c44298fc"}, k.PARKED_MSG)
    assert hits, f"no parking record with key_fp=e3b0c44298fc; saw {ekr['parked']}"
    assert not any(_has_key_attr(m) for m in ekr["parked"]), "a parking record still has `key`"


@then("the two same-key parking logs share one key_fp, the third differs, and every key_fp matches 12 lowercase hex")
def then_stable(ekr: dict) -> None:
    same, _, other = ekr["keys"]
    deadline = time.monotonic() + k.PARK_S
    while True:
        same_hits = _lines_for(ekr, {k.fp(same)}, k.PARKED_MSG)
        other_hits = _lines_for(ekr, {k.fp(other)}, k.PARKED_MSG)
        if (len(same_hits) >= 2 and other_hits) or time.monotonic() >= deadline:
            break
        time.sleep(3.0)
    assert len(same_hits) >= 2, f"want 2 parking records with key_fp={k.fp(same)}, got {len(same_hits)}"
    assert other_hits, f"no parking record for the 200-byte key (key_fp={k.fp(other)})"
    assert k.fp(same) != k.fp(other)
    for line in k.log_lines(ekr["since_s"]):
        if "key_fp" in line:
            assert re.fullmatch(r"[0-9a-f]{12}", line["key_fp"]), line
    _no_raw(ekr, [same, other])
