from __future__ import annotations

import datetime as dt
import json

import pytest
from _fake_redis import FakeRedis, closed_port_url

from forecast.cli import main

AS_OF = "2026-11-10T02:45:00Z"
DAY0 = dt.datetime(2026, 11, 9)


def _rows(listings: dict[str, int], seller: str = "S1"):
    rows = []
    for lid, days in listings.items():
        for k in range(days):
            day = DAY0 - dt.timedelta(days=k)
            rows.append(
                (seller, lid, 2 + (k % 3), day.replace(hour=10), "PAID", "buyer-secret-1")
            )
    return rows


@pytest.fixture
def redis_srv():
    srv = FakeRedis()
    yield srv
    srv.stop()


@pytest.fixture
def env(monkeypatch, tmp_path):
    def _set(url: str, input_dir=None):
        monkeypatch.setenv("REDIS_URL", url)
        monkeypatch.setenv("INPUT_DIR", str(input_dir or tmp_path / "in"))
        monkeypatch.setenv("AS_OF", AS_OF)

    return _set


def test_redis_down_exits_3(write_order_facts, env, tmp_path, capsys):
    """FNJ-5: unreachable Redis exits 3."""
    write_order_facts(tmp_path / "in", _rows({"L1": 30}))
    env(closed_port_url())
    assert main(["run"]) == 3


def test_redis_down_does_not_write_meta(write_order_facts, env, tmp_path, redis_srv):
    """FNJ-5: meta is not written when listing writes fail."""
    write_order_facts(tmp_path / "in", _rows({"L1": 30}))
    redis_srv.fail_writes = True
    env(redis_srv.url)
    assert main(["run"]) == 3
    assert "fc:v1:meta" not in redis_srv.keys()


def test_failed_run_keeps_old_keys(write_order_facts, env, tmp_path, redis_srv):
    """FNJ-10: runs failing with 2 or 3 leave earlier keys untouched."""
    write_order_facts(tmp_path / "in", _rows({"L1": 30, "L2": 25}))
    env(redis_srv.url)
    assert main(["run"]) == 0
    before = {k: v for k, v in redis_srv.store.items()}
    assert len(redis_srv.keys()) == 3

    env(redis_srv.url, tmp_path / "missing")
    redis_srv.write_log.clear()
    assert main(["run"]) == 2
    assert redis_srv.store == before

    write_order_facts(tmp_path / "in2", _rows({"L1": 40, "L2": 35}))
    env(redis_srv.url, tmp_path / "in2")
    redis_srv.fail_writes = True
    assert main(["run"]) == 3
    assert redis_srv.store == before
    assert "DEL" not in redis_srv.write_log


def test_listing_key_payload_and_ttl(write_order_facts, env, tmp_path, redis_srv):
    """FNJ-6, FNJ-7, FNJ-8: 14-day key with TTL; unpaid-only listing has no key."""
    rows = _rows({"L1": 30})
    rows.append(("S1", "X", 3, DAY0.replace(hour=9), "CANCELLED", "buyer-secret-1"))
    write_order_facts(tmp_path / "in", rows)
    env(redis_srv.url)
    assert main(["run"]) == 0
    assert "fc:v1:seller:S1:L1" in redis_srv.keys()
    assert not [k for k in redis_srv.keys() if k.endswith(":X")]
    raw, ttl = redis_srv.store[b"fc:v1:seller:S1:L1"]
    assert 172700 <= ttl <= 172800
    days = json.loads(raw)["daily_forecasts"]
    assert len(days) == 14
    assert days[0]["date"] == "2026-11-10"
    assert days[-1]["date"] == "2026-11-23"
    for d in days:
        assert 0 <= d["p10"] <= d["p50"] <= d["p90"]


def test_summary_counts_the_run(write_order_facts, env, tmp_path, redis_srv, capsys):
    """FNJ-15: meta counts listings and cold starts; stdout equals meta."""
    write_order_facts(tmp_path / "in", _rows({"L1": 30, "L2": 28, "C": 5}))
    env(redis_srv.url)
    assert main(["run"]) == 0
    meta = json.loads(redis_srv.store[b"fc:v1:meta"][0])
    assert meta["listings"] == 3
    assert meta["cold_starts"] == 1
    assert meta["status"] == "ok"
    out = capsys.readouterr().out.strip().splitlines()
    assert json.loads(out[-1]) == meta


def test_empty_export_writes_nothing(write_order_facts, env, tmp_path, redis_srv, capsys):
    """FNJ-16: empty export exits 0, status empty, no fc:v1 key."""
    write_order_facts(tmp_path / "in", [])
    env(redis_srv.url)
    assert main(["run"]) == 0
    assert json.loads(capsys.readouterr().out.strip().splitlines()[-1])["status"] == "empty"
    assert redis_srv.keys() == []
    assert redis_srv.write_log == []
