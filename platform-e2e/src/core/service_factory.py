"""Service factory (mirrors bds `ServiceFactory`).

Instantiates gateway API services on demand and shares one bearer token across
them — the bridge that lets UI-login and API-seeding act as the same principal.
"""

from __future__ import annotations

from typing import TypeVar

from src.api.services import (
    AddressService,
    AiService,
    AuthService,
    BaseService,
    CartService,
    EngagementService,
    ListingService,
    MetricsService,
    OrderService,
    PaymentService,
    RecommendationService,
    SearchService,
    TrackingService,
)

T = TypeVar("T", bound=BaseService)

_SERVICES: tuple[type[BaseService], ...] = (
    AddressService,
    AuthService,
    ListingService,
    SearchService,
    CartService,
    OrderService,
    PaymentService,
    EngagementService,
    AiService,
    RecommendationService,
    TrackingService,
    MetricsService,
)


class ServiceFactory:
    def __init__(self, token: str | None = None) -> None:
        self._token = token
        self._cache: dict[type[BaseService], BaseService] = {}

    def get(self, service_cls: type[T]) -> T:
        if service_cls not in self._cache:
            self._cache[service_cls] = service_cls(token=self._token)
        return self._cache[service_cls]  # type: ignore[return-value]

    def set_token(self, token: str | None) -> None:
        """Propagate a freshly obtained token to every live service."""
        self._token = token
        for svc in self._cache.values():
            svc.set_token(token)

    @property
    def address(self) -> AddressService:
        return self.get(AddressService)

    @property
    def auth(self) -> AuthService:
        return self.get(AuthService)

    @property
    def listing(self) -> ListingService:
        return self.get(ListingService)

    @property
    def search(self) -> SearchService:
        return self.get(SearchService)

    @property
    def cart(self) -> CartService:
        return self.get(CartService)

    @property
    def order(self) -> OrderService:
        return self.get(OrderService)

    @property
    def payment(self) -> PaymentService:
        return self.get(PaymentService)

    @property
    def engagement(self) -> EngagementService:
        return self.get(EngagementService)

    @property
    def ai(self) -> AiService:
        return self.get(AiService)

    @property
    def recommendation(self) -> RecommendationService:
        return self.get(RecommendationService)

    @property
    def tracking(self) -> TrackingService:
        return self.get(TrackingService)

    @property
    def metrics(self) -> MetricsService:
        return self.get(MetricsService)

    def close(self) -> None:
        for svc in self._cache.values():
            svc.close()
        self._cache.clear()
