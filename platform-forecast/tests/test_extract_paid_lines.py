from __future__ import annotations

import datetime as dt
import os
import subprocess
import sys
from pathlib import Path

import pytest

from forecast.extract import ConfigError, read_paid_series

ROOT = Path(__file__).resolve().parents[1]
AS_OF = dt.datetime(2026, 11, 10, 2, 45, tzinfo=dt.timezone.utc)


def _ts(day: str, hour: int = 12) -> dt.datetime:
    d = dt.date.fromisoformat(day)
    return dt.datetime(d.year, d.month, d.day, hour)


def _series(df, seller: str, listing: str):
    sub = df[(df["seller_id"] == seller) & (df["listing_id"] == listing)]
    return sub.sort_values("date").reset_index(drop=True)


def test_only_paid_complete_days(tmp_path, write_order_facts):
    """FNJ-1"""
    write_order_facts(
        tmp_path,
        [
            ("S", "L", 3, _ts("2026-11-08"), "PAID", "b1"),
            ("S", "L", 4, _ts("2026-11-09"), "PAID", "b2"),
            ("S", "L", 50, _ts("2026-11-09", 15), "PENDING", "b3"),
            ("S", "L", 70, _ts("2026-11-10", 1), "PAID", "b4"),
        ],
    )
    df = read_paid_series(tmp_path, AS_OF, history_days=90)
    s = _series(df, "S", "L")
    assert [str(d)[:10] for d in s["date"]] == ["2026-11-08", "2026-11-09"]
    assert list(s["units"]) == [3, 4]
    assert "buyer_id" not in df.columns


def test_series_starts_at_first_sale(tmp_path, write_order_facts):
    """FNJ-2"""
    a_first = AS_OF.date() - dt.timedelta(days=20)
    b_first = AS_OF.date() - dt.timedelta(days=5)
    write_order_facts(
        tmp_path,
        [
            ("S", "A", 1, _ts(a_first.isoformat()), "PAID", "b"),
            ("S", "A", 2, _ts("2026-11-09"), "PAID", "b"),
            ("S", "B", 1, _ts(b_first.isoformat()), "PAID", "b"),
            ("S", "B", 2, _ts("2026-11-09"), "PAID", "b"),
        ],
    )
    df = read_paid_series(tmp_path, AS_OF, history_days=90)
    a = _series(df, "S", "A")
    b = _series(df, "S", "B")
    assert len(a) == 20
    assert len(b) == 5
    assert str(b["date"].iloc[0])[:10] == b_first.isoformat()
    assert int(b["units"].iloc[0]) == 1
    assert int(b["units"].iloc[-1]) == 2
    assert int(b["units"].iloc[1]) == 0


def test_missing_column_exits_2(tmp_path, write_order_facts):
    """FNJ-4 (and FNJ-3 reader path: ConfigError on missing file)"""
    write_order_facts(
        tmp_path,
        [("S", "L", 3, _ts("2026-11-08"), "b1")],
        columns=("seller_id", "listing_id", "quantity", "occurred_at", "buyer_id"),
    )
    with pytest.raises(ConfigError):
        read_paid_series(tmp_path, AS_OF, history_days=90)

    env = {
        **os.environ,
        "INPUT_DIR": str(tmp_path),
        "AS_OF": "2026-11-10T02:45:00Z",
        "REDIS_URL": "redis://127.0.0.1:1/0",
    }
    proc = subprocess.run(
        [sys.executable, "-m", "forecast", "run"],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 2, proc.stderr
    assert "status" in proc.stderr

    empty = tmp_path / "empty"
    empty.mkdir()
    with pytest.raises(ConfigError, match="order_facts.parquet"):
        read_paid_series(empty, AS_OF, history_days=90)
    proc = subprocess.run(
        [sys.executable, "-m", "forecast", "run"],
        cwd=ROOT,
        env={**env, "INPUT_DIR": str(empty)},
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 2
    assert "order_facts.parquet" in proc.stderr
