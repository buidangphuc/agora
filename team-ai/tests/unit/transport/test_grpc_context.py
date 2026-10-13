from __future__ import annotations

from typing import Any

import grpc
import pytest

from app.modules.platform.identity.schemas import Principal
from app.transport.grpc.context import (
    bind_client_ip,
    bind_principal,
    caller_key,
    current_client_ip,
    current_principal,
    ensure_scopes,
    reset_principal,
)


class _Abort(Exception):
    pass


class _Context:
    def __init__(self) -> None:
        self.code: grpc.StatusCode | None = None
        self.details: str | None = None

    async def abort(self, code: grpc.StatusCode, details: str) -> None:
        self.code, self.details = code, details
        raise _Abort


def _ctx() -> Any:
    return _Context()


async def test_no_principal_aborts_unauthenticated() -> None:
    ctx = _ctx()
    with pytest.raises(_Abort):
        await ensure_scopes(ctx, "listing.read")
    assert ctx.code is grpc.StatusCode.UNAUTHENTICATED
    assert ctx.details == "no principal on call"


async def test_missing_scopes_are_reported_sorted() -> None:
    token = bind_principal(Principal(id="u", type="user", scopes=("listing.read",)))
    ctx = _ctx()
    try:
        with pytest.raises(_Abort):
            await ensure_scopes(ctx, "listing.write", "admin")
    finally:
        reset_principal(token)
    assert ctx.code is grpc.StatusCode.PERMISSION_DENIED
    assert ctx.details == "insufficient_scope: missing ['admin', 'listing.write']"


async def test_held_scopes_return_the_principal() -> None:
    principal = Principal(id="u", type="user", scopes=("a", "b"))
    token = bind_principal(principal)
    try:
        assert await ensure_scopes(_ctx(), "a", "b") == principal
        assert await ensure_scopes(_ctx()) == principal
    finally:
        reset_principal(token)
    assert current_principal() is None


def test_caller_key_variants() -> None:
    anon = Principal(id="anonymous", type="anonymous")
    bind_client_ip("203.0.113.7")
    assert caller_key(anon) == "anonymous:ip:203.0.113.7"
    assert caller_key(Principal(id="u1", type="user")) == "user:u1"
    assert caller_key(Principal(id="s1", type="service")) == "service:s1"
    bind_client_ip("")
    assert caller_key(anon) == "anonymous:anonymous"


def test_client_ip_is_stripped_and_validated() -> None:
    bind_client_ip("  1.2.3.4 ")
    assert current_client_ip() == "1.2.3.4"
    bind_client_ip("x" * 65)
    assert current_client_ip() == ""
    bind_client_ip("1.2.3.4 evil")
    assert current_client_ip() == ""
