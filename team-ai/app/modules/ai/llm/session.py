"""Bounded, TTL'd chat session history behind a small port.

A *turn* here is one exchange (user message + assistant reply). Histories are
scoped by ``(principal_id, session_id)`` so one user cannot read another's session
by guessing an id. Callers store *redacted* user text and append only after a fully
successful reply.
"""

from __future__ import annotations

import json
import time
from collections.abc import Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol
from urllib.parse import quote

if TYPE_CHECKING:
    from redis.asyncio import Redis


@dataclass(frozen=True)
class Turn:
    user: str
    assistant: str


class SessionStore(Protocol):
    async def load(self, principal_id: str, session_id: str) -> list[Turn]:
        """Prior turns, oldest first, already bounded by turn and char limits."""
        ...

    async def append(self, principal_id: str, session_id: str, turn: Turn) -> None:
        """Record one completed exchange (refreshes the TTL)."""
        ...


def bound_turns(turns: list[Turn], *, max_turns: int, max_chars: int) -> list[Turn]:
    """Keep the newest turns within both bounds, dropping the oldest first."""
    kept = turns[-max_turns:] if max_turns > 0 else []
    while kept and sum(len(t.user) + len(t.assistant) for t in kept) > max_chars:
        kept = kept[1:]
    return kept


class InMemorySessionStore:
    """Per-process store for dev/test. TTL is checked lazily on access."""

    def __init__(
        self,
        *,
        ttl_seconds: float,
        max_turns: int,
        max_chars: int,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._ttl = ttl_seconds
        self._max_turns = max_turns
        self._max_chars = max_chars
        self._clock = clock
        self._data: dict[tuple[str, str], tuple[float, list[Turn]]] = {}

    async def load(self, principal_id: str, session_id: str) -> list[Turn]:
        key = (principal_id, session_id)
        entry = self._data.get(key)
        if entry is None:
            return []
        expires_at, turns = entry
        if self._clock() >= expires_at:
            del self._data[key]
            return []
        return bound_turns(turns, max_turns=self._max_turns, max_chars=self._max_chars)

    async def append(self, principal_id: str, session_id: str, turn: Turn) -> None:
        key = (principal_id, session_id)
        turns = [*(await self.load(principal_id, session_id)), turn]
        bounded = bound_turns(
            turns, max_turns=self._max_turns, max_chars=self._max_chars
        )
        self._data[key] = (self._clock() + self._ttl, bounded)


class RedisSessionStore:
    """Redis list per session: ``chat:session:{principal}:{session}``.

    One JSON element per exchange; ``RPUSH`` + ``LTRIM`` to the turn bound +
    ``EXPIRE`` to the TTL. The char bound is applied on read (oldest first).
    """

    def __init__(
        self,
        redis: Redis,
        *,
        ttl_seconds: int,
        max_turns: int,
        max_chars: int,
        prefix: str = "chat:session",
        owns_client: bool = False,
    ) -> None:
        self._redis = redis
        self._owns_client = owns_client
        self._ttl = ttl_seconds
        self._max_turns = max_turns
        self._max_chars = max_chars
        self._prefix = prefix

    async def aclose(self) -> None:
        """Close the Redis client, but only if this store created it."""
        if self._owns_client:
            await self._redis.aclose()

    def key(self, principal_id: str, session_id: str) -> str:
        # Escape ':' so (a, b:c) and (a:b, c) can never collide.
        return f"{self._prefix}:{quote(principal_id, safe='')}:{quote(session_id, safe='')}"

    async def load(self, principal_id: str, session_id: str) -> list[Turn]:
        if self._max_turns <= 0:
            return []  # 0 means keep nothing (``-0`` would select the whole list)
        raw = await self._redis.lrange(  # type: ignore[misc]
            self.key(principal_id, session_id), -self._max_turns, -1
        )
        turns: list[Turn] = []
        for item in raw:
            try:
                data = json.loads(item)
                turns.append(Turn(user=str(data["u"]), assistant=str(data["a"])))
            except (ValueError, KeyError, TypeError):
                continue  # ignore a corrupt element rather than fail the chat
        return bound_turns(turns, max_turns=self._max_turns, max_chars=self._max_chars)

    async def append(self, principal_id: str, session_id: str, turn: Turn) -> None:
        key = self.key(principal_id, session_id)
        if self._max_turns <= 0:
            # Keep nothing, like the in-memory store (``ltrim key -0 -1`` keeps all).
            await self._redis.delete(key)
            return
        payload = json.dumps(
            {
                "u": turn.user[: self._max_chars],
                "a": turn.assistant[: self._max_chars],
            },
            ensure_ascii=False,
        )
        async with self._redis.pipeline(transaction=True) as pipe:
            pipe.rpush(key, payload)
            pipe.ltrim(key, -self._max_turns, -1)
            pipe.expire(key, self._ttl)
            await pipe.execute()
