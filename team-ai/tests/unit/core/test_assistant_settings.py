from pathlib import Path

import pytest
from pydantic import ValidationError

from tests.factories import build_test_settings


def test_assistant_rag_min_score_defaults_to_zero():
    assert build_test_settings().ASSISTANT_RAG_MIN_SCORE == 0.0


def test_assistant_rag_min_score_rejects_negative():
    with pytest.raises(ValidationError):
        build_test_settings(ASSISTANT_RAG_MIN_SCORE=-0.1)


def test_env_example_lists_assistant_rag_min_score():
    env_example = Path(__file__).parents[3] / ".env.example"

    assert "ASSISTANT_RAG_MIN_SCORE=" in env_example.read_text()
