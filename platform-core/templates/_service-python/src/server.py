"""grpc.aio server assembly.

Wires interceptors (tracing -> request-id -> auth), the health + reflection
services, and one seed business RPC. The seed servicer needs generated protobuf
stubs, so its import is guarded: until you run ``make proto`` the server still
starts and serves health + reflection — enough to prove the wiring and to unit
test the pure helpers without any generated code present.
"""

from __future__ import annotations

import asyncio
import logging
import sys
from pathlib import Path

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc
from grpc_reflection.v1alpha import reflection

from config import get_settings
from interceptors.auth import AuthInterceptor, InsufficientScopeError, require_scopes
from interceptors.tracing import (
    RequestIdInterceptor,
    otel_server_interceptor,
    setup_tracing,
)
from repository import InMemorySearchRepository, SearchRepository

logger = logging.getLogger(__name__)

# Generated stubs land in ./generated (see buf.gen.yaml). buf generates with
# source-relative paths, so every generated module lives under a top-level
# ``platform`` package (proto packages are ``platform.<domain>.v1``) and they
# import each other absolutely (`from platform.common.v1 import common_pb2`).
# That ``generated`` dir must therefore be on sys.path as a roots container.
#
# ⚠️  STDLIB-SHADOWING CAVEAT  ⚠️
# ------------------------------------------------------------------------------
# The generated top-level package is literally ``platform``, which SHADOWS
# Python's stdlib ``platform`` module for the whole process once ``generated`` is
# on sys.path. Any code (yours or a third-party dependency) that does
# ``import platform`` and expects the stdlib (e.g. ``platform.system()``) will
# then resolve to the generated package instead and break.
#
# RECOMMENDED HARDENING: rewrite the generated absolute imports into RELATIVE
# imports with protoletariat, so the generated tree can live under a private
# package (e.g. ``generated``) with no top-level ``platform`` on sys.path. Run,
# in ``make proto`` after ``buf generate``:
#
#     protol --create-package --in-place --python-out generated buf --config-path buf.gen.yaml
#
# then import as ``from generated.platform.search.v1 import search_pb2`` and drop
# the sys.path insert below. See README.md / proto-vendor/README.md. The seed
# keeps the plain sys.path approach so it works out of the box.
_GENERATED = Path(__file__).resolve().parent.parent / "generated"
if _GENERATED.is_dir() and str(_GENERATED) not in sys.path:
    sys.path.insert(0, str(_GENERATED))

# ── Seed business service (requires `make proto`) ─────────────────────────────
try:
    from platform.search.v1 import search_pb2, search_pb2_grpc  # type: ignore

    _PROTO_READY = True
except ImportError:  # generated code not present yet — that's fine for the seed.
    search_pb2 = None  # type: ignore[assignment]
    search_pb2_grpc = None  # type: ignore[assignment]
    _PROTO_READY = False


if _PROTO_READY:

    class SearchServicer(search_pb2_grpc.SearchServiceServicer):  # type: ignore[misc]
        """Seed implementation of ``platform.search.v1.SearchService`` (mock hits)."""

        def __init__(self, repository: SearchRepository) -> None:
            self._repository = repository

        async def SearchListings(  # noqa: N802 (gRPC method name)
            self,
            request: "search_pb2.SearchListingsRequest",
            context: grpc.aio.ServicerContext,
        ) -> "search_pb2.SearchListingsResponse":
            try:
                require_scopes("search:read")
            except InsufficientScopeError as err:
                await context.abort(err.code(), err.details())

            page_size = request.page.page_size or 10
            hits = await self._repository.search(
                request.query,
                limit=page_size,
                filters=dict(request.filters),
            )
            return search_pb2.SearchListingsResponse(
                hits=[
                    search_pb2.SearchHit(listing_id=h.listing_id, score=h.score)
                    for h in hits
                ],
                page=None,
            )


def build_server() -> grpc.aio.Server:
    """Construct the server with interceptors + servicers (not yet started).

    Safe to call without generated code: registers health + reflection always,
    and the seed SearchService only when its stubs are available.
    """
    settings = get_settings()

    # Order matters: OTel first (opens the span), then request-id, then auth so
    # the Principal resolves inside an active trace.
    server = grpc.aio.server(
        interceptors=(
            otel_server_interceptor(),
            RequestIdInterceptor(),
            AuthInterceptor(),
        )
    )

    # Health — reports SERVING for the overall server ("" = all services).
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)

    service_names: list[str] = [
        health.SERVICE_NAME,
        reflection.SERVICE_NAME,
    ]

    if _PROTO_READY:
        search_pb2_grpc.add_SearchServiceServicer_to_server(
            SearchServicer(InMemorySearchRepository()), server
        )
        service_names.append(
            search_pb2.DESCRIPTOR.services_by_name["SearchService"].full_name
        )
    else:
        logger.warning(
            "Generated stubs not found — run `make proto`. "
            "Serving health + reflection only."
        )

    reflection.enable_server_reflection(tuple(service_names), server)
    server.add_insecure_port(f"[::]:{settings.GRPC_PORT}")

    # Mark everything SERVING (call after add_*_to_server so names resolve).
    for name in service_names:
        health_servicer.set(name, health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)

    return server


async def serve() -> None:
    """Start the server and block until a termination signal, then drain."""
    logging.basicConfig(level=logging.INFO)
    settings = get_settings()
    provider = setup_tracing()

    server = build_server()
    await server.start()
    logger.info("gRPC server listening on :%d", settings.GRPC_PORT)

    try:
        await server.wait_for_termination()
    finally:
        # Graceful shutdown: stop accepting, let in-flight RPCs finish (grace),
        # then flush spans.
        await server.stop(grace=5.0)
        provider.shutdown()
        logger.info("gRPC server stopped")


if __name__ == "__main__":
    asyncio.run(serve())
