from __future__ import annotations

from typing import Any

import grpc
import pytest
from pydantic import BaseModel, Field, ValidationError

from app.core.errors import ServiceUnavailableError
from app.transport.grpc.errors import map_errors


class _Abort(Exception):
    pass


class _Context:
    def __init__(self) -> None:
        self.code: grpc.StatusCode | None = None
        self.details: str | None = None

    async def abort(self, code: grpc.StatusCode, details: str) -> None:
        self.code, self.details = code, details
        raise _Abort


class _Item(BaseModel):
    title: str = Field(min_length=50)


class _Bag(BaseModel):
    items: dict[str, int]


def _validation_error(model: type[BaseModel], **data: Any) -> ValidationError:
    with pytest.raises(ValidationError) as info:
        model(**data)
    return info.value


async def _run(exc: BaseException) -> _Context:
    @map_errors("test")
    async def handler(self: Any, request: Any, context: Any) -> None:
        raise exc

    ctx = _Context()
    with pytest.raises(_Abort):
        await handler(None, None, ctx)
    return ctx


async def test_validation_error_never_echoes_input() -> None:
    ctx = await _run(_validation_error(_Item, title="secret text"))
    assert ctx.code is grpc.StatusCode.INVALID_ARGUMENT
    assert ctx.details == "invalid request: title"
    assert "secret text" not in (ctx.details or "")


async def test_client_chosen_dict_keys_are_sanitised() -> None:
    ctx = await _run(_validation_error(_Bag, items={"bad key!<x>": "nan"}))
    assert ctx.details is not None
    assert ctx.details.startswith("invalid request: items.")
    assert "<" not in ctx.details
    assert "!" not in ctx.details
    assert "bad key" not in ctx.details


async def test_service_unavailable_maps_to_unavailable() -> None:
    ctx = await _run(ServiceUnavailableError("db host 10.0.0.1 down"))
    assert ctx.code is grpc.StatusCode.UNAVAILABLE
    assert ctx.details == "service unavailable"


async def test_other_exceptions_map_to_internal_without_detail() -> None:
    ctx = await _run(RuntimeError("password=hunter2"))
    assert ctx.code is grpc.StatusCode.INTERNAL
    assert ctx.details == "internal error"


async def test_abort_error_is_reraised_untouched() -> None:
    @map_errors("test")
    async def handler(self: Any, request: Any, context: Any) -> None:
        raise grpc.aio.AbortError()

    ctx = _Context()
    with pytest.raises(grpc.aio.AbortError):
        await handler(None, None, ctx)
    assert ctx.code is None


async def test_success_passes_through() -> None:
    @map_errors("test")
    async def handler(self: Any, request: Any, context: Any) -> str:
        return "ok"

    assert await handler(None, "req", _Context()) == "ok"
