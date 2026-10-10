## Context

The probe ran 2026-10-10 on an NVIDIA V100, with torch 2.5.1 and sentence-transformers 3.3.1, using synthetic
query/title pairs:
- 14 related pairs, 6 Vietnamese and 8 English;
- 170 unrelated pairs, with no shared tokens;
- 28 nonsense queries.

| Model | related min / median | unrelated median / p90 / max | nonsense max |
|---|---|---|---|
| BAAI/bge-small-en-v1.5 | 0.697 / 0.819 | 0.514 / 0.600 / 0.666 | 0.636 |
| intfloat/multilingual-e5-small | 0.880 / 0.913 | 0.800 / 0.833 / 0.857 | 0.836 |

## Decisions

- **D1: the default is 0.65 for bge-small-en-v1.5.** This value is 0.047 below the lowest related pair and
  removes all nonsense pairs. It also removes every unrelated pair except the top one (0.666); 0.6 let through
  about 10%.
- **D2: keep the floor configurable and keep the probe.** The gap between the related and unrelated
  distributions is narrow: about 0.03 for both models. The probe must therefore be re-run against real
  query logs before the value is trusted in production.

## Risks

- The sample is small and synthetic.
- Related pairs with less lexical overlap than these, such as synonyms or brand-only queries, may score below
  0.65 and lose their semantic recall. The lexical leg still finds them when they share terms.
