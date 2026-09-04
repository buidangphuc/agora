"""Admin cockpit metrics client (replace-cockpit-mock-metrics).

Reads the shaped `CockpitMetricsResponse` the gateway serves at
`GET /api/admin/metrics` from real Prometheus data. The gateway is a thin
read-only proxy: it runs a fixed, hardcoded PromQL set server-side and never
exposes raw Prometheus or accepts arbitrary PromQL from the browser — so this
client only reads the shaped response (and probes that the raw Prometheus query
path is NOT reachable through the gateway).
"""

from __future__ import annotations

from typing import Any

from src.constants import gateway_endpoints as ep

from .base_service import BaseService


class MetricsService(BaseService):
    def get_cockpit_metrics(self) -> dict[str, Any]:
        """Return the CockpitMetricsResponse (services[], total_rps, ...)."""
        return self.get(ep.ADMIN_METRICS)

    def service_row(self, metrics: dict[str, Any], service: str) -> dict[str, Any]:
        """Pick one service's health row out of a metrics response (name-insensitive)."""
        for row in metrics.get("services", []) or []:
            name = str(row.get("name") or row.get("service") or "").lower()
            if service.lower() in name:
                return row
        return {}

    def raw_prometheus_query_status(self, promql: str = "up") -> int:
        """Probe whether the gateway forwards raw PromQL. Expected: a 4xx (not exposed)."""
        resp = self.send("GET", ep.PROM_RAW_QUERY, extra_headers={"Accept": "application/json"})
        return resp.status_code
