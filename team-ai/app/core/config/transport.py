"""gRPC transport settings — the platform-contract server for team-ai."""

from __future__ import annotations

from pydantic import BaseModel, Field


class TransportSettingsMixin(BaseModel):
    # gRPC server (implements the platform-core proto contract).
    GRPC_ENABLED: bool = False
    GRPC_HOST: str = "0.0.0.0"
    GRPC_PORT: int = Field(default=50051, gt=0)
    GRPC_REFLECTION_ENABLED: bool = True
    GRPC_GRACE_SECONDS: float = Field(default=10.0, ge=0)
    # Throttle StreamChat + ShoppingAssistant per forwarded principal, using
    # RATE_LIMIT_BACKEND / RATE_LIMIT_PRINCIPAL_PER_MINUTE / RATE_LIMIT_WINDOW_SECONDS.
    # Opt-in (off by default). Anonymous callers all share one bucket
    # ("anonymous:anonymous"); a limiter backend error fails open.
    GRPC_RATE_LIMIT_ENABLED: bool = False
    # Require the ``ai:use`` scope on ShoppingAssistant / StreamChat. Off until
    # team-identity grants ``ai:use`` to buyer/seller/admin (it grants none today,
    # so enabling it earlier would deny every shopper). See transport/grpc/scopes.py.
    AI_USE_SCOPE_REQUIRED: bool = False
    # Static-bearer fallback on the gRPC surface (``authorization: bearer
    # AUTH_BEARER_TOKEN`` -> service principal with AUTH_ROLES scopes). Off by
    # default so a direct caller needs the gateway-forwarded x-principal-* metadata;
    # local tooling (grpcurl) may opt in. Refused at boot outside dev/local/test.
    # The REST API bearer auth is unaffected.
    GRPC_BEARER_FALLBACK_ENABLED: bool = False
