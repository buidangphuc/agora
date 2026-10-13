from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from starlette.middleware.trustedhost import TrustedHostMiddleware

from app.bootstrap.state import get_in_flight_tracker
from app.core.config import Settings
from app.core.middleware import (
    AccessLogMiddleware,
    InFlightTrackerMiddleware,
    RequestBodyLimitMiddleware,
    RequestTimeoutMiddleware,
)
from app.core.request_context import RequestIdMiddleware


def install_core_middlewares(app: FastAPI, settings: Settings) -> None:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.cors_allow_origins,
        allow_credentials=settings.CORS_ALLOW_CREDENTIALS,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    app.add_middleware(
        TrustedHostMiddleware,
        allowed_hosts=settings.trusted_hosts,
    )
    app.add_middleware(
        RequestBodyLimitMiddleware,
        max_body_bytes=settings.MAX_REQUEST_BODY_BYTES,
    )
    app.add_middleware(AccessLogMiddleware)
    app.add_middleware(
        InFlightTrackerMiddleware,
        tracker=get_in_flight_tracker(app),
    )
    app.add_middleware(
        RequestTimeoutMiddleware,
        timeout_seconds=settings.REQUEST_TIMEOUT_SECONDS,
        exclude_patterns=tuple(settings.request_timeout_exclude_patterns),
    )
    app.add_middleware(RequestIdMiddleware)
