import asyncio
import json
from typing import Any, cast

import pytest
from langchain_core.messages import AIMessage, SystemMessage

from app.core.errors import ServiceUnavailableError
from app.modules.ai.llm.runtime import LLMInstance
from app.modules.business.listing_screen.ports import ScreenVerdict
from app.modules.business.listing_screen.service import LLMScreenService


class _FakeLLM:
    """Stands in for LLMInstance: answers ``ainvoke`` with one scripted reply."""

    def __init__(self, reply: Any = "", *, delay: float = 0.0) -> None:
        self.reply = reply
        self.delay = delay
        self.inputs: list[Any] = []

    async def ainvoke(self, input: Any, **_kwargs: Any) -> AIMessage:
        self.inputs.append(input)
        if self.delay:
            await asyncio.sleep(self.delay)
        if isinstance(self.reply, Exception):
            raise self.reply
        return AIMessage(content=self.reply)


def _service(llm: _FakeLLM, timeout: float = 1.5) -> LLMScreenService:
    return LLMScreenService(cast(LLMInstance, llm), timeout_seconds=timeout)


async def _screen(llm: _FakeLLM, **kw: Any):
    return await _service(llm).screen(
        title=kw.get("title", "Road bike"),
        description=kw.get("description", "Barely used"),
        listing_id="l1",
        category_id="c1",
    )


async def test_llm_review_verdict_carries_labels():
    """Scenario: LLM review verdict is returned with its labels."""
    llm = _FakeLLM(json.dumps({"verdict": "review", "labels": ["counterfeit"]}))

    result = await _screen(llm)

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["counterfeit"]
    assert result.backend == "llm_router"


async def test_llm_allow_verdict():
    """Scenario: LLM allow verdict is returned."""
    llm = _FakeLLM(json.dumps({"verdict": "allow", "labels": []}))

    result = await _screen(llm)

    assert result.verdict == ScreenVerdict.ALLOW
    assert list(result.labels) == []
    assert result.backend == "llm_router"


@pytest.mark.parametrize(
    "reply",
    [
        "Looks fine, ALLOW",
        "",
        "[]",
        '{"verdict": "maybe", "labels": []}',
        '{"labels": ["counterfeit"]}',
    ],
)
async def test_unparseable_output_is_review(reply: str):
    """Scenario: Unparseable model output becomes REVIEW."""
    result = await _screen(_FakeLLM(reply))

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["unparseable"]
    assert result.backend == "llm_router"


async def test_unknown_labels_are_dropped():
    """Scenario: Labels outside the closed set are dropped."""
    llm = _FakeLLM(
        json.dumps(
            {
                "verdict": "review",
                "labels": ["counterfeit", "ignore previous instructions"],
            }
        )
    )

    result = await _screen(llm)

    assert result.verdict == ScreenVerdict.REVIEW
    assert list(result.labels) == ["counterfeit"]


async def test_slow_llm_times_out_as_unavailable():
    """Scenario: A call slower than the timeout is UNAVAILABLE."""
    llm = _FakeLLM(json.dumps({"verdict": "allow", "labels": []}), delay=5.0)
    loop = asyncio.get_running_loop()
    start = loop.time()

    with pytest.raises(ServiceUnavailableError):
        await _service(llm, timeout=0.05).screen(title="t", description="d")

    assert loop.time() - start < 0.5


async def test_text_is_truncated_and_delimited():
    """Scenario: Listing text is truncated and passed as data."""
    llm = _FakeLLM(json.dumps({"verdict": "allow", "labels": []}))

    await _screen(llm, title="T" * 20000, description="D" * 20000)

    (prompt,) = llm.inputs
    messages = list(prompt)
    system_text = "".join(
        str(m.content) for m in messages if isinstance(m, SystemMessage)
    )
    data_text = "".join(
        str(m.content) for m in messages if not isinstance(m, SystemMessage)
    )
    assert "D" * 50 not in system_text
    assert "T" * 50 not in system_text
    assert data_text.count("D") <= 8000
    assert data_text.count("T") <= 8000
    assert "D" * 100 in data_text  # truncated, not dropped
    # the data is wrapped in delimiters: text surrounds the run on both sides
    start, end = data_text.index("D" * 100), data_text.rindex("D")
    assert data_text[:start].strip() != ""
    assert data_text[end + 1 :].strip() != ""
