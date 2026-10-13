"""Emergent attribute extraction: wattage, battery capacity, storage, RAM and materials."""

import re

from app.modules.business.tag_classifier.schemas import FacetGroup, TagItem, TagStatus

_WATTAGE = re.compile(r"\b(\d{2,3})\s*w\b")
_BATTERY = re.compile(r"\b(\d{4,5})\s*mah\b")
_STORAGE = re.compile(r"\b(\d{1,3})\s*(gb|tb)\b")
_RAM = re.compile(r"\b(\d{1,2})\s*gb\s*ram\b")

# (keyword, name, slug, facet group, default category)
MATERIAL_KEYWORDS: list[tuple[str, str, str, FacetGroup, str]] = [
    (
        "linen",
        "Vải Linen Tự Nhiên",
        "vai-linen-tu-nhien",
        FacetGroup.MATERIAL,
        "cat-fashion",
    ),
    (
        "satin",
        "Lụa Satin Cao Cấp",
        "lua-satin-cao-cap",
        FacetGroup.MATERIAL,
        "cat-fashion",
    ),
    (
        "denim",
        "Chất liệu Denim Bền",
        "chat-lieu-denim-ben",
        FacetGroup.MATERIAL,
        "cat-fashion",
    ),
    (
        "titan",
        "Khung Titanium",
        "khung-titanium",
        FacetGroup.MATERIAL,
        "cat-electronics",
    ),
    (
        "ceramic",
        "Gốm Sứ Ceramic",
        "gom-su-ceramic",
        FacetGroup.MATERIAL,
        "cat-home",
    ),
]
_MATERIALS = [
    (re.compile(r"\b" + kw + r"\b"), name, slug, fg, cat)
    for kw, name, slug, fg, cat in MATERIAL_KEYWORDS
]


def _candidate(
    name: str, slug: str, facet_group: FacetGroup, category_id: str, confidence: float
) -> TagItem:
    return TagItem(
        tag_id=f"cand-{slug}",
        name=name,
        slug=slug,
        facet_group=facet_group,
        category_id=category_id,
        status=TagStatus.EXPLORING,
        confidence=confidence,
        is_canonical=False,
        occurrence_count=1,
    )


def extract_emergent_patterns(norm_text: str, category_id: str) -> list[TagItem]:
    """Candidates found in accent-stripped, lowercased text, in team-ai's order."""
    default_category = category_id or "cat-electronics"
    candidates: list[TagItem] = []

    for match in _WATTAGE.finditer(norm_text):
        watt = match.group(1)
        candidates.append(
            _candidate(
                f"Công suất {watt}W",
                f"cong-suat-{watt}w",
                FacetGroup.POWER,
                default_category,
                0.88,
            )
        )

    for match in _BATTERY.finditer(norm_text):
        cap = match.group(1)
        candidates.append(
            _candidate(
                f"Dung lượng {cap}mAh",
                f"dung-luong-{cap}mah",
                FacetGroup.CAPACITY,
                default_category,
                0.90,
            )
        )

    for match in _STORAGE.finditer(norm_text):
        size, unit = match.group(1), match.group(2).upper()
        candidates.append(
            _candidate(
                f"Bộ nhớ {size}{unit}",
                f"bo-nho-{size.lower()}{unit.lower()}",
                FacetGroup.CAPACITY,
                default_category,
                0.92,
            )
        )

    for match in _RAM.finditer(norm_text):
        size = match.group(1)
        candidates.append(
            _candidate(
                f"RAM {size}GB",
                f"ram-{size}gb",
                FacetGroup.RAM,
                default_category,
                0.94,
            )
        )

    for pattern, name, slug, facet_group, cat in _MATERIALS:
        if pattern.search(norm_text):
            candidates.append(
                _candidate(name, slug, facet_group, category_id or cat, 0.85)
            )

    return candidates
