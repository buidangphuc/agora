"""OpenTelemetry settings.

Off until ``OTEL_EXPORTER_OTLP_ENDPOINT`` is set. Then the app exports traces and
GenAI metrics over OTLP/HTTP to it (needs the ``otel`` extra).
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class TelemetrySettingsMixin(BaseModel):
    OTEL_SERVICE_NAME: str = Field(default="ai-platform", min_length=1)
    # Base OTLP/HTTP endpoint, e.g. http://localhost:4318 (no /v1/traces suffix).
    # Empty = no OpenTelemetry.
    OTEL_EXPORTER_OTLP_ENDPOINT: str = ""
    OTEL_TRACES_SAMPLE_RATIO: float = Field(default=1.0, ge=0.0, le=1.0)

    @property
    def otel_enabled(self) -> bool:
        return bool(self.OTEL_EXPORTER_OTLP_ENDPOINT)
