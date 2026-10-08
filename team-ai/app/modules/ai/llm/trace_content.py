"""Apply ``LLM_TRACE_CONTENT`` to what Langfuse receives.

``LLM_TRACE_CONTENT`` (``off`` / ``redacted`` / ``full``) is one content policy for
every sink that can hold prompt or completion text. For Langfuse:

* ``off``      every string in input/output becomes ``[redacted]``; the trace
               structure, model, usage, timing, status and metadata stay.
* ``redacted`` strings get the same masking the model input gets (email, secrets,
               Vietnamese phone numbers, keyword-anchored national ids).
* ``full``     nothing is installed; text passes through unchanged.

Enforcement is at export time (``ContentMaskingSpanExporter``), the one place every
span passes through. The SDK's ``Langfuse(mask=...)`` hook is not enough: in SDK
4.6.1 it does not reach input/output on the LangChain callback path (the handler
writes them straight onto the OpenTelemetry span), and it does mask metadata such
as the request id, which ``off`` must keep. This SDK has no ``mask_otel_spans``
option. See ``tests/unit/modules/test_llm_trace_content.py`` for the spike.
"""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from typing import Any

from opentelemetry.sdk.trace import Event, ReadableSpan
from opentelemetry.sdk.trace.export import SpanExporter, SpanExportResult
from opentelemetry.trace import Status

from app.core.redaction import RedactionPolicy

# Span attributes that carry prompt / completion content in Langfuse's OTel schema.
CONTENT_ATTRIBUTES = (
    "langfuse.observation.input",
    "langfuse.observation.output",
    "langfuse.trace.input",
    "langfuse.trace.output",
    # Error text: a provider error can echo the prompt.
    "langfuse.observation.status_message",
)
# Event attributes (exception events) that can echo user text.
_EXCEPTION_TEXT_ATTRIBUTES = ("exception.message", "exception.stacktrace")


def mask_value(data: Any, policy: RedactionPolicy) -> Any:
    """Recursively mask every string in a JSON-like structure.

    Keys, numbers, booleans and ``None`` are kept so the trace keeps its shape.
    Values under secret-looking keys (``api_key``, ``token``...) are always hidden
    unless the policy is ``full``.
    """
    if policy.mode == "full":
        return data
    if isinstance(data, str):
        return policy.redact_text(data)
    if isinstance(data, Mapping):
        return {
            key: (
                "[secret]"
                if isinstance(key, str) and policy._is_secret_key(key)
                else mask_value(value, policy)
            )
            for key, value in data.items()
        }
    if isinstance(data, Sequence) and not isinstance(data, bytes | bytearray):
        return [mask_value(item, policy) for item in data]
    return data


def _mask_serialized(value: Any, policy: RedactionPolicy) -> Any:
    """Mask an attribute holding JSON text (or plain text)."""
    if not isinstance(value, str):
        return value
    try:
        decoded = json.loads(value)
    except ValueError:
        return policy.redact_text(value)
    return json.dumps(mask_value(decoded, policy), ensure_ascii=False)


def mask_span(span: ReadableSpan, policy: RedactionPolicy) -> ReadableSpan:
    """A copy of ``span`` whose content attributes obey ``policy``."""
    attributes = dict(span.attributes or {})
    for key in CONTENT_ATTRIBUTES:
        if key in attributes:
            attributes[key] = _mask_serialized(attributes[key], policy)

    events = []
    for event in span.events:
        event_attrs = dict(event.attributes or {})
        for key in _EXCEPTION_TEXT_ATTRIBUTES:
            if key in event_attrs:
                event_attrs[key] = _mask_serialized(event_attrs[key], policy)
        events.append(
            Event(
                event.name,
                event_attrs,
                event.timestamp,
                limit=None,  # type: ignore[arg-type]
            )
        )

    status = span.status
    if status.description:
        status = Status(status.status_code, policy.redact_text(status.description))

    return ReadableSpan(
        name=span.name,
        context=span.context,
        parent=span.parent,
        resource=span.resource,
        attributes=attributes,
        events=events,
        links=span.links,
        kind=span.kind,
        status=status,
        start_time=span.start_time,
        end_time=span.end_time,
        instrumentation_scope=span.instrumentation_scope,
    )


class ContentMaskingSpanExporter(SpanExporter):
    """Wrap a span exporter so content is masked before it leaves the process."""

    def __init__(self, inner: SpanExporter, policy: RedactionPolicy) -> None:
        self._inner = inner
        self._policy = policy

    def export(self, spans: Sequence[ReadableSpan]) -> SpanExportResult:
        return self._inner.export([mask_span(span, self._policy) for span in spans])

    def shutdown(self) -> None:
        self._inner.shutdown()

    def force_flush(self, timeout_millis: int = 30000) -> bool:
        return self._inner.force_flush(timeout_millis)
