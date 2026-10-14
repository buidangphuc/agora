"""ADR-0017 content checks (spec: model-registry, ADR scenarios)."""
import re
from pathlib import Path

ADR_DIR = Path(__file__).resolve().parents[3] / "docs" / "ADR"
ADR_17 = ADR_DIR / "0017-unified-model-registry-and-publish-gate.md"
ADR_14 = ADR_DIR / "0014-model-registry-promotion.md"
ADR_13 = ADR_DIR / "0013-seller-demand-forecasting.md"

SECTIONS = ("Status", "Context", "Decision", "Alternatives", "Consequences")
STATES = ("pending", "promotable", "promoted", "rejected")
STATE_TERMS = ("concurrencyPolicy: Forbid",) + STATES


def _adr17() -> str:
    assert ADR_17.is_file(), f"missing {ADR_17}"
    return ADR_17.read_text(encoding="utf-8")


def missing_terms(text: str) -> list[str]:
    """Return required single-writer/state terms absent from the ADR text."""
    return [t for t in STATE_TERMS if t not in text]


def missing_sections(text: str) -> list[str]:
    found = []
    if re.search(r"^\*\*Status:\*\*|^#+\s*Status\b", text, re.M):
        found.append("Status")
    for s in SECTIONS[1:]:
        if re.search(rf"^#+\s*{s}\b", text, re.M):
            found.append(s)
    return [s for s in SECTIONS if s not in found]


def test_supersession_lines():
    """Scenario: Supersession is visible from the old ADRs."""
    text17 = _adr17()
    assert re.search(r"[Ss]upersede[sd]?[^\n]*ADR-0014", text17)
    assert re.search(r"[Ss]upersede[sd]?[^\n]*ADR-0013", text17)
    for name in ("ADR-0014", "ADR-0013"):
        assert name in text17
    assert missing_sections(text17) == [], missing_sections(text17)

    t14 = ADR_14.read_text(encoding="utf-8")
    status14 = next(l for l in t14.splitlines() if l.startswith("**Status:**"))
    assert "Superseded by ADR-0017" in status14

    t13 = ADR_13.read_text(encoding="utf-8")
    lines = t13.splitlines()
    idx = next(i for i, l in enumerate(lines) if "MLflow + MinIO + Postgres registry" in l)
    # the marker sits on the MLflow alternative itself (same or next line)
    assert any("Superseded by ADR-0017" in l for l in lines[idx : idx + 2])
    # ADR-0013 as a whole is NOT superseded
    status13 = next(l for l in lines if l.startswith("**Status:**"))
    assert "Superseded" not in status13


def test_privacy_rule_present():
    """Scenario: Consumer privacy rule is recorded."""
    text = re.sub(r"\s+", " ", _adr17()).lower()
    assert "mlflow" in text
    assert re.search(r"must not (contain|store)[^.]*person identifiers", text)
    assert "interaction rows" in text
    assert "per-person lists" in text
    for kind in ("runs", "tags", "artifacts"):
        assert kind in text


def test_single_writer_and_states_recorded():
    """Scenario: Single-writer assumption is written down."""
    text = _adr17()
    assert missing_terms(text) == [], missing_terms(text)
    low = text.lower()
    assert re.search(r"no lock|without a lock|does not take a lock|no lock is taken", low)
    assert re.search(r"single writer", low)
    assert re.search(r"crash[^.]*evaluate[^.]*apply", low)


def test_adr_rejects_missing_sections():
    """Scenario: ADR without the state model fails the check (names missing term)."""
    text = _adr17()
    assert missing_terms(text) == []
    for term in STATE_TERMS:
        broken = text.replace(term, "")
        assert missing_terms(broken) == [term]
    assert missing_terms("") == list(STATE_TERMS)
