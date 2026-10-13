"""Redis quota store: counters shared by every pod, reserved atomically in Lua.

Keys, under ``prefix``:

- ``c:{subject}:{resource}:{window}``  integer ``used`` of one window
- ``r:{reservation_id}``               hash of one reservation
- ``i:{subject}:{resource}:{window}:{idempotency_key}``  -> reservation id

Every key expires ``ttl_margin_seconds`` after its window resets, so reservations
expire with the window they charged.
"""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from datetime import UTC, datetime
from typing import TYPE_CHECKING, cast

from app.modules.platform.quota.models import (
    QuotaReservation,
    QuotaReservationStatus,
    QuotaUsage,
    QuotaUsageQuery,
    ReserveQuota,
)

if TYPE_CHECKING:
    from redis.asyncio import Redis

_RESERVE = """
if ARGV[7] == '1' then
    local existing = redis.call('GET', KEYS[3])
    if existing then return {1, existing} end
end
local cost = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local used = tonumber(redis.call('GET', KEYS[1]) or '0')
if used + cost > limit then return {0, tostring(used)} end
used = redis.call('INCRBY', KEYS[1], cost)
redis.call('EXPIRE', KEYS[1], ttl)
redis.call('HSET', KEYS[2], 'counter', KEYS[1], 'cost', ARGV[1], 'status', 'reserved',
    'subject', ARGV[5], 'resource', ARGV[6], 'window', ARGV[8],
    'limit', ARGV[2], 'reset_at', ARGV[4])
redis.call('EXPIRE', KEYS[2], ttl)
if ARGV[7] == '1' then redis.call('SET', KEYS[3], ARGV[9], 'EX', ttl) end
return {2, tostring(used)}
"""

# Returns {code, used}: 0 ok, 1 unknown reservation, 2 wrong state.
_FINALIZE = """
local status = redis.call('HGET', KEYS[1], 'status')
if not status then return {1, '0'} end
if status == 'refunded' then return {2, '0'} end
redis.call('HSET', KEYS[1], 'status', 'finalized')
return {0, redis.call('GET', redis.call('HGET', KEYS[1], 'counter')) or '0'}
"""

_REFUND = """
local status = redis.call('HGET', KEYS[1], 'status')
if not status then return {1, '0'} end
if status == 'finalized' then return {2, '0'} end
local counter = redis.call('HGET', KEYS[1], 'counter')
if status == 'reserved' then
    local used = tonumber(redis.call('GET', counter) or '0')
    local left = math.max(used - tonumber(redis.call('HGET', KEYS[1], 'cost')), 0)
    redis.call('SET', counter, left, 'KEEPTTL')
    redis.call('HSET', KEYS[1], 'status', 'refunded')
end
return {0, redis.call('GET', counter) or '0'}
"""


class RedisQuotaStore:
    def __init__(
        self,
        redis: Redis,
        *,
        prefix: str = "quota",
        ttl_margin_seconds: int = 3600,
    ) -> None:
        self._redis = redis
        self._prefix = prefix
        self._margin = ttl_margin_seconds
        self._reserve = redis.register_script(_RESERVE)
        self._finalize = redis.register_script(_FINALIZE)
        self._refund = redis.register_script(_REFUND)

    async def reserve(self, command: ReserveQuota) -> QuotaReservation | None:
        idempotency = command.idempotency_key
        code, value = await self._reserve(
            keys=[
                self._counter_key(command),
                self._key("r", command.reservation_id),
                self._key(
                    "i",
                    command.subject_id,
                    command.resource,
                    command.window_key,
                    idempotency or "",
                ),
            ],
            args=[
                command.cost,
                command.limit,
                self._ttl(command.reset_at),
                command.reset_at.isoformat(),
                command.subject_id,
                command.resource,
                "1" if idempotency else "0",
                command.window_key,
                command.reservation_id,
            ],
        )
        if code == 0:
            return None
        if code == 1:
            return await self._load(value)
        return QuotaReservation(
            id=command.reservation_id,
            subject_id=command.subject_id,
            resource=command.resource,
            window_key=command.window_key,
            cost=command.cost,
            status=QuotaReservationStatus.RESERVED,
            usage=_usage(command, used=int(value)),
            idempotency_key=idempotency,
        )

    async def finalize(self, reservation_id: str) -> QuotaUsage:
        code, used = await self._finalize(keys=[self._key("r", reservation_id)])
        _raise_unless_ok(
            code, reservation_id, "refunded reservation cannot be finalized"
        )
        return await self._usage_of(reservation_id, int(used))

    async def refund(self, reservation_id: str) -> QuotaUsage:
        code, used = await self._refund(keys=[self._key("r", reservation_id)])
        _raise_unless_ok(
            code, reservation_id, "finalized reservation cannot be refunded"
        )
        return await self._usage_of(reservation_id, int(used))

    async def get_usage(self, query: QuotaUsageQuery) -> QuotaUsage:
        used = await self._redis.get(
            self._key("c", query.subject_id, query.resource, query.window_key)
        )
        return _usage(query, used=int(used or 0))

    async def reset_usage(self, query: QuotaUsageQuery) -> QuotaUsage:
        await self._delete_matching(
            self._key("c", query.subject_id, query.resource, "*"),
            self._key("i", query.subject_id, query.resource, "*"),
        )
        await self._delete_reservations(
            lambda r: r["subject"] == query.subject_id
            and r["resource"] == query.resource
        )
        return _usage(query, used=0)

    async def reset_resource(self, resource: str) -> None:
        await self._delete_matching(
            self._key("c", "*", resource, "*"), self._key("i", "*", resource, "*")
        )
        await self._delete_reservations(lambda r: r["resource"] == resource)

    async def close(self) -> None:
        """The Redis client belongs to the application."""

    def _key(self, kind: str, *parts: str) -> str:
        return ":".join((self._prefix, kind, *parts))

    def _counter_key(self, command: ReserveQuota) -> str:
        return self._key("c", command.subject_id, command.resource, command.window_key)

    def _ttl(self, reset_at: datetime) -> int:
        return (
            max(int((reset_at - datetime.now(UTC)).total_seconds()), 0) + self._margin
        )

    async def _fields(self, key: str) -> dict[str, str]:
        return await cast("Awaitable[dict[str, str]]", self._redis.hgetall(key))

    async def _load(self, reservation_id: str) -> QuotaReservation:
        fields = await self._fields(self._key("r", reservation_id))
        used = int(await self._redis.get(fields["counter"]) or 0)
        return QuotaReservation(
            id=reservation_id,
            subject_id=fields["subject"],
            resource=fields["resource"],
            window_key=fields["window"],
            cost=int(fields["cost"]),
            status=QuotaReservationStatus(fields["status"]),
            usage=_stored_usage(fields, used),
        )

    async def _usage_of(self, reservation_id: str, used: int) -> QuotaUsage:
        fields = await self._fields(self._key("r", reservation_id))
        return _stored_usage(fields, used)

    async def _delete_matching(self, *patterns: str) -> None:
        for pattern in patterns:
            async for key in self._redis.scan_iter(match=pattern):
                await self._redis.delete(key)

    async def _delete_reservations(
        self, matches: Callable[[dict[str, str]], bool]
    ) -> None:
        async for key in self._redis.scan_iter(match=self._key("r", "*")):
            if matches(await self._fields(key)):
                await self._redis.delete(key)


def _raise_unless_ok(code: object, reservation_id: str, message: str) -> None:
    if code == 1:
        raise KeyError(reservation_id)
    if code == 2:
        raise ValueError(message)


def _usage(source: ReserveQuota | QuotaUsageQuery, *, used: int) -> QuotaUsage:
    return QuotaUsage(
        subject_id=source.subject_id,
        resource=source.resource,
        window_key=source.window_key,
        used=used,
        limit=source.limit,
        remaining=max(source.limit - used, 0),
        reset_at=source.reset_at,
    )


def _stored_usage(fields: dict[str, str], used: int) -> QuotaUsage:
    limit = int(fields["limit"])
    return QuotaUsage(
        subject_id=fields["subject"],
        resource=fields["resource"],
        window_key=fields["window"],
        used=used,
        limit=limit,
        remaining=max(limit - used, 0),
        reset_at=datetime.fromisoformat(fields["reset_at"]),
    )
