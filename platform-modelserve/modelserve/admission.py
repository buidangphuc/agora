"""Admission control and in-flight queue depth tracker."""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import HTTPException, status
from prometheus_client import Counter, Gauge, Histogram

# Prometheus Metrics
IN_FLIGHT_REQUESTS = Gauge(
    "modelserve_in_flight_requests",
    "Current number of in-flight inference requests",
    ["endpoint"],
)
ADMISSION_REJECTIONS = Counter(
    "modelserve_admission_rejections_total",
    "Total number of requests rejected due to queue saturation",
    ["endpoint"],
)
REQUEST_LATENCY = Histogram(
    "modelserve_request_duration_seconds",
    "Request latency in seconds",
    ["endpoint", "status_code"],
)
CACHE_HITS = Counter(
    "modelserve_cache_hits_total",
    "Total number of vector cache hits",
)
CACHE_MISSES = Counter(
    "modelserve_cache_misses_total",
    "Total number of vector cache misses",
)


class AdmissionController:
    """Controls concurrency and protects against queue saturation."""

    def __init__(self, max_queue_depth: int = 100) -> None:
        self.max_queue_depth = max_queue_depth
        self._in_flight: int = 0
        self._lock = asyncio.Lock()

    @property
    def in_flight_count(self) -> int:
        return self._in_flight

    @asynccontextmanager
    async def track(self, endpoint: str = "embed") -> AsyncIterator[None]:
        async with self._lock:
            if self._in_flight >= self.max_queue_depth:
                ADMISSION_REJECTIONS.labels(endpoint=endpoint).inc()
                raise HTTPException(
                    status_code=status.HTTP_429_TOO_MANY_REQUESTS,
                    detail="Inference queue depth saturated. Please retry later.",
                    headers={"Retry-After": "2"},
                )
            self._in_flight += 1
            IN_FLIGHT_REQUESTS.labels(endpoint=endpoint).set(self._in_flight)

        try:
            yield
        finally:
            async with self._lock:
                self._in_flight -= 1
                IN_FLIGHT_REQUESTS.labels(endpoint=endpoint).set(self._in_flight)
