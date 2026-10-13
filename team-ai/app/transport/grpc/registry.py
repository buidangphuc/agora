from collections.abc import Callable
from dataclasses import dataclass

import grpc


@dataclass(frozen=True, slots=True)
class GrpcServiceRegistration:
    service_name: str  # full name, e.g. "platform.ai.v1.AIService"
    register: Callable[[grpc.aio.Server], None]
    # Full method paths, e.g. "/platform.ai.v1.AIService/ShoppingAssistant".
    rate_limited_methods: frozenset[str] = frozenset()
