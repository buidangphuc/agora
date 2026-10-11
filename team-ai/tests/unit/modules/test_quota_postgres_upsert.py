"""The Postgres quota counter upsert must only name real columns."""

from __future__ import annotations

from datetime import UTC, datetime

from sqlalchemy.dialects import postgresql

from app.modules.platform.quota.adapters import postgres as pg
from app.modules.platform.quota.models import ReserveQuota


class _Capture:
    statement = None

    async def execute(self, statement):
        self.statement = statement


async def test_counter_upsert_sets_the_quota_limit_column_not_limit():
    session = _Capture()
    command = ReserveQuota(
        subject_id="s",
        resource="chat",
        window_key="w",
        limit=10,
        cost=1,
        reset_at=datetime.now(UTC),
        reservation_id="r1",
    )

    await pg._ensure_counter(session, command)  # type: ignore[arg-type]

    sql = str(session.statement.compile(dialect=postgresql.dialect()))
    update_clause = sql.split("DO UPDATE SET", 1)[1]
    assert "quota_limit" in update_clause
    assert '"limit"' not in update_clause
