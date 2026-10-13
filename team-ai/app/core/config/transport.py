"""gRPC transport settings: the platform-contract server (team-ai env names)."""

from __future__ import annotations

from pydantic import BaseModel, Field


class TransportSettingsMixin(BaseModel):
    # gRPC server (implements the platform-core proto contract).
    # Always served once the app opens its resources. Port 0 binds an ephemeral
    # port (tests).
    GRPC_HOST: str = "0.0.0.0"
    GRPC_PORT: int = Field(default=50051, ge=0)
    GRPC_GRACE_SECONDS: float = Field(default=10.0, ge=0)
    # Require the ``ai:use`` scope on ShoppingAssistant. Off until the identity
    # service grants ``ai:use`` to buyer/seller/admin (it grants none today, so
    # enabling it earlier would deny every shopper). See transport/grpc/scopes.py.
    AI_USE_SCOPE_REQUIRED: bool = False
