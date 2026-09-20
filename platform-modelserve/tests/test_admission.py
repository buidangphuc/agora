"""Tests for admission controller and queue depth backpressure."""

from __future__ import annotations

import asyncio

import pytest
from fastapi import HTTPException

from modelserve.admission import AdmissionController


def test_admission_control_allows_within_capacity() -> None:
    async def _run():
        controller = AdmissionController(max_queue_depth=2)

        async with controller.track("embed"):
            assert controller.in_flight_count == 1
            async with controller.track("embed"):
                assert controller.in_flight_count == 2

        assert controller.in_flight_count == 0

    asyncio.run(_run())


def test_admission_control_rejects_at_capacity() -> None:
    async def _run():
        controller = AdmissionController(max_queue_depth=1)

        async with controller.track("embed"):
            assert controller.in_flight_count == 1
            with pytest.raises(HTTPException) as exc_info:
                async with controller.track("embed"):
                    pass
            assert exc_info.value.status_code == 429
            assert "Retry-After" in exc_info.value.headers

    asyncio.run(_run())
