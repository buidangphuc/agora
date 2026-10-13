"""Accent-insensitive synonym matching; matchers are compiled once per registry change."""

import re
import unicodedata
from collections.abc import Mapping

from app.modules.business.tag_classifier.schemas import TagItem

type Matchers = tuple[tuple[re.Pattern[str], str], ...]


def strip_accents(s: str) -> str:
    """Normalize Vietnamese accents and lowercase for robust fuzzy matching."""
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn")
    return s.replace("đ", "d").replace("Đ", "d").lower().strip()


def _word(phrase: str) -> re.Pattern[str]:
    return re.compile(r"\b" + re.escape(phrase) + r"\b")


def compile_synonym_matchers(synonym_index: Mapping[str, str]) -> Matchers:
    """One ``(pattern, slug)`` per normalized synonym, in index order."""
    return tuple((_word(syn), slug) for syn, slug in synonym_index.items())


def compile_candidate_matchers(candidates: Mapping[str, TagItem]) -> Matchers:
    """One ``(pattern, slug)`` per candidate, matching its normalized name."""
    return tuple(
        (_word(strip_accents(cand.name)), slug) for slug, cand in candidates.items()
    )


def matching_slugs(matchers: Matchers, norm_text: str) -> list[str]:
    """Distinct slugs with at least one matching pattern, in first-match order."""
    seen: dict[str, None] = {}
    for pattern, slug in matchers:
        if slug not in seen and pattern.search(norm_text):
            seen[slug] = None
    return list(seen)
