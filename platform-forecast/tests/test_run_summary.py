from __future__ import annotations

import datetime as dt
import json

from _fake_redis import FakeRedis

from forecast.cli import main

META_KEYS = {
    "model_version",
    "generated_at",
    "as_of",
    "horizon_days",
    "listings",
    "cold_starts",
    "wape",
    "baseline_wape",
    "status",
}


def test_summary_has_only_aggregates(write_order_facts, monkeypatch, tmp_path, capsys):
    """FNJ-17: neither meta nor stdout carries buyer, order or event ids."""
    day0 = dt.datetime(2026, 11, 9)
    rows = [
        ("S1", "L1", 2 + k % 3, (day0 - dt.timedelta(days=k)).replace(hour=10), "PAID", "buyer-zzz-77")
        for k in range(30)
    ]
    write_order_facts(tmp_path / "in", rows)
    srv = FakeRedis()
    try:
        monkeypatch.setenv("REDIS_URL", srv.url)
        monkeypatch.setenv("INPUT_DIR", str(tmp_path / "in"))
        monkeypatch.setenv("AS_OF", "2026-11-10T02:45:00Z")
        assert main(["run"]) == 0
        meta_raw = srv.store[b"fc:v1:meta"][0].decode()
    finally:
        srv.stop()
    out = capsys.readouterr().out
    for blob in (meta_raw, out):
        low = blob.lower()
        assert "buyer" not in low and "zzz-77" not in low
        assert "order_id" not in low and "event_id" not in low
    assert set(json.loads(meta_raw)) == META_KEYS
    assert set(json.loads(out.strip().splitlines()[-1])) == META_KEYS
