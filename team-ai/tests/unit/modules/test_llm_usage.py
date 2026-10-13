from langchain_core.messages import AIMessageChunk

from app.modules.ai.llm.usage import (
    CHARS_PER_TOKEN,
    ChatUsage,
    UsageAccumulator,
    estimate_tokens,
)


def test_estimate_tokens_rounds_up_by_characters():
    assert CHARS_PER_TOKEN == 4
    assert estimate_tokens("") == 0
    assert estimate_tokens("abcd") == 1
    assert estimate_tokens("abcde") == 2


def test_provider_usage_is_summed_across_chunks():
    usage = UsageAccumulator()
    usage.add(AIMessageChunk(content="a").usage_metadata)  # None is ignored
    usage.add({"input_tokens": 11, "output_tokens": 0, "total_tokens": 11})
    usage.add({"input_tokens": 0, "output_tokens": 7, "total_tokens": 7})

    assert usage.result(target="m", input_text="x" * 400, output_text="y") == ChatUsage(
        "m", 11, 7, 18
    )


def test_without_provider_usage_the_character_estimate_is_flagged():
    result = UsageAccumulator().result(
        target="m", input_text="x" * 40, output_text="abcdefgh"
    )

    assert result == ChatUsage("m", 10, 2, 12, estimated=True)


def test_total_falls_back_to_input_plus_output():
    usage = UsageAccumulator()
    usage.add({"input_tokens": 3, "output_tokens": 2, "total_tokens": 0})

    assert usage.result(target="m", input_text="", output_text="").total_tokens == 5
