from __future__ import annotations

from pathlib import Path

import pytest


@pytest.fixture
def write_order_facts():
    def _write(
        input_dir: Path,
        rows: list[tuple],
        columns: tuple[str, ...] = (
            "seller_id",
            "listing_id",
            "quantity",
            "occurred_at",
            "status",
            "buyer_id",
        ),
    ) -> Path:
        import duckdb

        input_dir.mkdir(parents=True, exist_ok=True)
        target = input_dir / "order_facts.parquet"
        con = duckdb.connect()
        types = {
            "seller_id": "VARCHAR",
            "listing_id": "VARCHAR",
            "quantity": "INTEGER",
            "occurred_at": "TIMESTAMP",
            "status": "VARCHAR",
            "buyer_id": "VARCHAR",
        }
        con.execute(
            "CREATE TABLE t (" + ", ".join(f"{c} {types[c]}" for c in columns) + ")"
        )
        marks = ", ".join("?" for _ in columns)
        for row in rows:
            con.execute(f"INSERT INTO t VALUES ({marks})", list(row))
        con.execute(f"COPY t TO '{target}' (FORMAT PARQUET)")
        con.close()
        return target

    return _write
