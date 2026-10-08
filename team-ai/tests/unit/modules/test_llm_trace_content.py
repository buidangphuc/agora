"""LLM_TRACE_CONTENT is one content policy for every sink, Langfuse included.

The spans are captured with an in-memory OpenTelemetry exporter, so the assertions
look at what would really leave the process (attributes of the exported spans).

Spike result (Langfuse SDK 4.6.1): ``Langfuse(mask=...)`` does NOT run on the
LangChain callback path -- the handler writes input/output straight onto the OTel
span -- and this SDK has no ``mask_otel_spans`` option. The policy is therefore
enforced where it cannot be bypassed, at export time, by
``ContentMaskingSpanExporter``; ``mask=`` is not used (it also blanks metadata).
"""

from __future__ import annotations

import uuid
from collections.abc import Iterator

import pytest
from langchain_core.messages import HumanMessage
from langfuse import Langfuse
from langfuse.langchain import CallbackHandler
from opentelemetry.sdk.trace.export.in_memory_span_exporter import (
    InMemorySpanExporter,
)

from app.core.config import Settings
from app.modules.ai.llm import langfuse as langfuse_module
from app.modules.ai.llm.langfuse import LLMTraceContext, build_langfuse_tracker
from app.modules.ai.llm.testing import Script, ScriptedProvider
from tests.factories import build_test_settings

PHONE = "0987654321"
REPLY_PHONE = "0912345678"
MESSAGE = f"Goi toi theo so {PHONE} nhe"
REPLY = f"Lien he {REPLY_PHONE} de biet them"


def _settings(mode: str) -> Settings:
    return build_test_settings(
        LANGFUSE_ENABLED=True,
        LANGFUSE_PUBLIC_KEY=f"pk-{uuid.uuid4().hex}",  # one SDK client per key
        LANGFUSE_SECRET_KEY="sk-test",
        LANGFUSE_BASE_URL="http://127.0.0.1:1",
        LLM_TRACE_CONTENT=mode,
    )


@pytest.fixture()
def exporter(monkeypatch: pytest.MonkeyPatch) -> Iterator[InMemorySpanExporter]:
    """Route the tracker's inner OTLP exporter into memory."""
    memory = InMemorySpanExporter()
    monkeypatch.setattr(
        langfuse_module, "_otlp_span_exporter", lambda settings, **_: memory
    )
    yield memory


async def _run_chat(tracker) -> None:
    provider = ScriptedProvider().queue(
        "a",
        Script(
            chunks=(REPLY,),
            usage={"input_tokens": 9, "output_tokens": 3, "total_tokens": 12},
        ),
    )
    model = provider.builder("a")
    config = tracker.trace_config(LLMTraceContext(request_id="req-1"))
    async for _ in model.astream([HumanMessage(content=MESSAGE)], config=config):
        pass
    tracker.flush()


def _exported_text(exporter: InMemorySpanExporter) -> str:
    """Every attribute value and event attribute of every exported span."""
    parts: list[str] = []
    for span in exporter.get_finished_spans():
        parts.extend(str(v) for v in (span.attributes or {}).values())
        if span.status.description:
            parts.append(span.status.description)
        for event in span.events:
            parts.extend(str(v) for v in (event.attributes or {}).values())
    return "\n".join(parts)


async def test_sdk_mask_alone_does_not_cover_the_langchain_callback_path():
    """Why the exporter is needed: the spike that decided the design."""
    memory = InMemorySpanExporter()
    public_key = f"pk-{uuid.uuid4().hex}"
    client = Langfuse(
        public_key=public_key,
        secret_key="sk-test",
        base_url="http://127.0.0.1:1",
        span_exporter=memory,
        mask=lambda *, data, **_: "[MASKED]" if isinstance(data, str) else data,
        flush_at=1,
    )
    provider = ScriptedProvider().queue("a", Script(chunks=(REPLY,)))
    handler = CallbackHandler(public_key=public_key)
    async for _ in provider.builder("a").astream(
        [HumanMessage(content=MESSAGE)], config={"callbacks": [handler]}
    ):
        pass
    client.flush()

    exported = _exported_text(memory)
    assert PHONE in exported, (
        "Langfuse(mask=...) now covers the callback path; ContentMaskingSpanExporter "
        "may be redundant -- revisit design D10"
    )


@pytest.mark.parametrize("mode", ["off", "redacted"])
async def test_no_exported_attribute_contains_raw_content(mode, exporter):
    tracker = build_langfuse_tracker(
        _settings(mode), instance_id="t", service_name="team-ai.test"
    )

    await _run_chat(tracker)

    spans = exporter.get_finished_spans()
    assert spans, "the trace must still be exported"
    text = _exported_text(exporter)
    assert PHONE not in text and REPLY_PHONE not in text
    assert "Goi toi theo so" not in text or mode == "redacted"
    if mode == "off":
        assert "Goi toi theo so" not in text and "Lien he" not in text
        assert "[redacted]" in text
    else:
        # Same masking as text handed to the model.
        assert "[phone]" in text


async def test_off_keeps_structure_model_usage_and_timing(exporter):
    tracker = build_langfuse_tracker(
        _settings("off"), instance_id="t", service_name="team-ai.test"
    )

    await _run_chat(tracker)

    (span,) = exporter.get_finished_spans()
    attrs = dict(span.attributes or {})
    assert attrs["langfuse.observation.type"] == "generation"
    assert attrs["langfuse.observation.usage_details"] == (
        '{"input": 9, "output": 3, "total": 12}'
    )
    assert attrs["langfuse.observation.metadata.request_id"] == "req-1"
    assert span.end_time and span.start_time and span.end_time >= span.start_time
    assert (
        attrs["langfuse.observation.input"]
        == '[{"role": "[redacted]", "content": "[redacted]"}]'
    )


async def test_redacted_shows_the_masked_input_as_phone_placeholder(exporter):
    tracker = build_langfuse_tracker(
        _settings("redacted"), instance_id="t", service_name="team-ai.test"
    )

    await _run_chat(tracker)

    (span,) = exporter.get_finished_spans()
    attrs = dict(span.attributes or {})
    assert "[phone]" in str(attrs["langfuse.observation.input"])
    assert "Goi toi theo so [phone] nhe" in str(attrs["langfuse.observation.input"])


async def test_full_installs_no_mask_and_keeps_content(exporter):
    tracker = build_langfuse_tracker(
        _settings("full"), instance_id="t", service_name="team-ai.test"
    )

    await _run_chat(tracker)

    (span,) = exporter.get_finished_spans()
    attrs = dict(span.attributes or {})
    assert MESSAGE in str(attrs["langfuse.observation.input"])
    assert REPLY in str(attrs["langfuse.observation.output"])
    assert PHONE in _exported_text(exporter)


async def test_failed_call_does_not_leak_text_through_exception_events(exporter):
    tracker = build_langfuse_tracker(
        _settings("redacted"), instance_id="t", service_name="team-ai.test"
    )
    provider = ScriptedProvider().queue(
        "a",
        Script(
            fail_at=0,
            error=RuntimeError(
                f"provider echoed the prompt: call {PHONE} sk-abcdef123"
            ),
        ),
    )
    config = tracker.trace_config(LLMTraceContext(request_id="req-err"))
    with pytest.raises(RuntimeError):
        async for _ in provider.builder("a").astream(
            [HumanMessage(content=MESSAGE)], config=config
        ):
            pass
    tracker.flush()

    text = _exported_text(exporter)
    assert exporter.get_finished_spans()
    assert PHONE not in text and "sk-abcdef123" not in text


def test_mask_value_keeps_shape_and_hides_secret_keys():
    from app.core.redaction import RedactionPolicy
    from app.modules.ai.llm.trace_content import mask_value

    data = {
        "role": "user",
        "content": [{"text": f"phone {PHONE}", "n": 3, "ok": True, "none": None}],
        "api_key": "abc",
    }
    redacted = mask_value(data, RedactionPolicy(mode="redacted"))
    assert redacted == {
        "role": "user",
        "content": [{"text": "phone [phone]", "n": 3, "ok": True, "none": None}],
        "api_key": "[secret]",
    }
    off = mask_value(data, RedactionPolicy(mode="off"))
    assert off["role"] == "[redacted]"
    assert off["content"][0] == {
        "text": "[redacted]",
        "n": 3,
        "ok": True,
        "none": None,
    }
    assert mask_value(data, RedactionPolicy(mode="full")) == data


def test_otlp_exporter_is_built_like_the_sdk_default_unless_full():
    settings = _settings("redacted")
    exporter = langfuse_module._otlp_span_exporter(settings)

    assert exporter is not None
    assert exporter._endpoint == "http://127.0.0.1:1/api/public/otel/v1/traces"
    headers = {k.lower(): v for k, v in exporter._headers.items()}
    assert headers["authorization"].startswith("Basic ")
    assert headers["x-langfuse-public-key"] == settings.LANGFUSE_PUBLIC_KEY
    assert langfuse_module._otlp_span_exporter(_settings("full")) is None
