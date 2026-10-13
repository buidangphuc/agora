"""The live taxonomy: canonical tags, candidates and the synonym index."""

import asyncio
from collections.abc import Iterable, Mapping

from app.modules.business.tag_classifier.matching import (
    Matchers,
    compile_candidate_matchers,
    compile_synonym_matchers,
    strip_accents,
)
from app.modules.business.tag_classifier.schemas import TagItem


class TaxonomyRegistry:
    """Mutated only by the curation path, under ``lock``; reads need no lock."""

    def __init__(self, seed: Iterable[TagItem]) -> None:
        self.lock = asyncio.Lock()
        self.canonical: dict[str, TagItem] = {}
        self.candidates: dict[str, TagItem] = {}
        self._synonym_index: dict[str, str] = {}  # normalized synonym -> tag slug
        self._synonym_matchers: Matchers | None = None
        self._candidate_matchers: Matchers | None = None
        for tag in seed:
            self.register_canonical(tag)

    def register_canonical(self, tag: TagItem) -> None:
        self.canonical[tag.slug] = tag
        self._synonym_index[strip_accents(tag.name)] = tag.slug
        self._synonym_index[strip_accents(tag.slug.replace("-", " "))] = tag.slug
        for syn in tag.synonyms:
            self._synonym_index[strip_accents(syn)] = tag.slug
        self._synonym_matchers = None

    def register_candidate(self, tag: TagItem) -> None:
        if tag.slug in self.canonical:
            return
        existing = self.candidates.get(tag.slug)
        if existing is None:
            self.candidates[tag.slug] = tag
            self._candidate_matchers = None
        else:
            existing.occurrence_count += tag.occurrence_count
            existing.confidence = max(existing.confidence, tag.confidence)

    def drop_candidate(self, slug: str) -> None:
        if self.candidates.pop(slug, None) is not None:
            self._candidate_matchers = None

    def apply_overlay(
        self, canonical: Mapping[str, TagItem], candidates: Mapping[str, TagItem]
    ) -> None:
        """Layer the persisted state on top of the seed."""
        for tag in canonical.values():
            self.drop_candidate(tag.slug)
            self.register_canonical(tag)
        for cand in candidates.values():
            if cand.slug not in self.canonical:
                self.candidates[cand.slug] = cand
                self._candidate_matchers = None

    def synonym_matchers(self) -> Matchers:
        if self._synonym_matchers is None:
            self._synonym_matchers = compile_synonym_matchers(self._synonym_index)
        return self._synonym_matchers

    def candidate_matchers(self) -> Matchers:
        if self._candidate_matchers is None:
            self._candidate_matchers = compile_candidate_matchers(self.candidates)
        return self._candidate_matchers
