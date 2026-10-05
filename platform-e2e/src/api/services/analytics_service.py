"""Seller analytics client (secure-seller-analytics-and-admin-seed).

The three per-seller AnalyticsQueryService RPCs through the gateway. Each helper
returns the raw response WITHOUT raising on 4xx: the access scenarios assert the
status code (200 owner/admin, 401 anonymous, 403 other seller).
"""

from __future__ import annotations

import httpx

from src.constants import gateway_endpoints as ep

from .base_service import BaseService


class AnalyticsService(BaseService):
    def funnel_response(self, seller_id: str) -> httpx.Response:
        return self.send("POST", ep.ANALYTICS_SELLER_FUNNEL, json_body={"seller_id": seller_id})

    def revenue_response(self, seller_id: str) -> httpx.Response:
        return self.send("POST", ep.ANALYTICS_REVENUE_BREAKDOWN, json_body={"seller_id": seller_id})

    def forecast_response(self, seller_id: str, listing_id: str = "e2e-listing") -> httpx.Response:
        return self.send(
            "POST",
            ep.ANALYTICS_DEMAND_FORECAST,
            json_body={"seller_id": seller_id, "listing_id": listing_id, "horizon_days": 7},
        )
