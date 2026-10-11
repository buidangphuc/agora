"""AIService error mapping: field names out, fixed messages, full detail in logs."""

from __future__ import annotations

import grpc
import pytest
from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.core.request_context import reset_request_id, set_request_id
from app.modules.business.ai_assistant.service import AIAssistantService
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.errors import INTERNAL_MESSAGE, map_servicer_error
from app.transport.grpc.servicers.ai import AIServicer

SECRET = "sk-live-TOPSECRET-9f8e7d"
ADMIN = Principal(
    id="admin-1",
    type="user",
    scopes=("admin", "listing.read", "listing.write", "ai:use"),
)
BUYER = Principal(id="b1", type="user", scopes=("listing.read",))


class _Aborted(Exception):
    def __init__(self, code: grpc.StatusCode, details: str) -> None:
        super().__init__(details)
        self.code = code
        self.details = details


class _FakeContext:
    def __init__(self) -> None:
        self.trailing: tuple = ()

    def set_trailing_metadata(self, metadata) -> None:
        self.trailing = tuple(metadata)

    async def abort(self, code: grpc.StatusCode, details: str = ""):
        raise _Aborted(code, details)


class _Exploding:
    """An assistant whose every method raises ``error``."""

    def __init__(self, error: Exception) -> None:
        self._error = error

    async def shopping_assistant(self, _req):
        raise self._error

    async def magic_listing(self, _req):
        raise self._error

    async def chat_copilot(self, _req):
        raise self._error

    async def summarize_reviews(self, _req):
        raise self._error


def _servicer(error: Exception) -> AIServicer:
    return AIServicer(lambda: _Exploding(error))  # type: ignore[arg-type, return-value]


async def _call(servicer, rpc: str, request, *, principal=ADMIN, context=None):
    context = context or _FakeContext()
    token = bind_principal(principal)
    try:
        return await getattr(servicer, rpc)(request, context)
    finally:
        reset_principal(token)


REQUESTS = {
    "ShoppingAssistant": ai_pb2.ShoppingAssistantRequest(message="ao thun"),
    "MagicListing": ai_pb2.MagicListingRequest(title_hint="ao thun"),
    "ChatCopilot": ai_pb2.ChatCopilotRequest(buyer_message="hi"),
    "SummarizeReviews": ai_pb2.SummarizeReviewsRequest(listing_id="l1"),
}


@pytest.fixture()
def captured_logs():
    lines: list[str] = []
    sink = logger.add(
        lambda message: lines.append(str(message)),
        format="{message} | {exception}",
        level="DEBUG",
    )
    yield lines
    logger.remove(sink)


async def test_validation_error_names_the_field_and_never_echoes_the_input():
    servicer = AIServicer(AIAssistantService)
    request = ai_pb2.MagicListingRequest(title_hint="Ω")  # below min_length=2

    with pytest.raises(_Aborted) as info:
        await _call(servicer, "MagicListing", request)

    assert info.value.code is grpc.StatusCode.INVALID_ARGUMENT
    assert "title_hint" in info.value.details
    assert "Ω" not in info.value.details
    assert "input_value" not in info.value.details


@pytest.mark.parametrize("rpc", sorted(REQUESTS))
async def test_unexpected_failure_is_opaque_but_fully_logged(rpc, captured_logs):
    servicer = _servicer(RuntimeError(f"db password={SECRET} at /srv/app/x.py"))
    context = _FakeContext()
    rid_token = set_request_id("req-abc123")
    try:
        with pytest.raises(_Aborted) as info:
            await _call(servicer, rpc, REQUESTS[rpc], context=context)
    finally:
        reset_request_id(rid_token)

    assert info.value.code is grpc.StatusCode.INTERNAL
    assert info.value.details == INTERNAL_MESSAGE == "internal error"
    assert SECRET not in info.value.details
    assert SECRET not in repr(context.trailing)

    log = "\n".join(captured_logs)
    assert SECRET in log, "the full exception must be logged server-side"
    assert "req-abc123" in log, "the log line carries the request id"


async def test_service_unavailable_is_distinguishable():
    servicer = _servicer(ServiceUnavailableError(f"search backend {SECRET} down"))

    with pytest.raises(_Aborted) as info:
        await _call(servicer, "ShoppingAssistant", REQUESTS["ShoppingAssistant"])

    assert info.value.code is grpc.StatusCode.UNAVAILABLE
    assert SECRET not in info.value.details


async def test_abort_from_a_scope_check_is_not_remapped():
    servicer = _servicer(RuntimeError("never reached"))

    with pytest.raises(_Aborted) as info:
        await _call(servicer, "MagicListing", REQUESTS["MagicListing"], principal=BUYER)

    assert info.value.code is grpc.StatusCode.PERMISSION_DENIED
    assert "insufficient_scope" in info.value.details


async def test_abort_raised_inside_the_body_is_reraised_untouched():
    class _AbortingContextError(grpc.aio.AbortError):
        pass

    servicer = _servicer(_AbortingContextError())

    with pytest.raises(grpc.aio.AbortError):
        await _call(servicer, "SummarizeReviews", REQUESTS["SummarizeReviews"])


def test_map_servicer_error_table():
    assert map_servicer_error(RuntimeError("boom"))[0] is grpc.StatusCode.INTERNAL
    code, message = map_servicer_error(ServiceUnavailableError("x"))
    assert code is grpc.StatusCode.UNAVAILABLE and "x" not in message
