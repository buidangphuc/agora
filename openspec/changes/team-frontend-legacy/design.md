## Context

B1 kept `brand`, `mall`, `shopee` and `gray` so call sites compiled; B2 moved every use to semantic or Tier 1 names. A grep of non-test source for those names (plus `border-t-gray`) is now empty apart from the `Badge` `mall` variant, which is not a colour class.

## Decisions

1. Remove the colour groups, the `gray` alias and the three shopee shadows from the config; remove the dead `.shopee-discount-tag` CSS (no component uses it).
2. Tests move from "recoloured" to "absent": `tokens.test.ts` checks the config has none of `gray`, `brand`, `mall`, `shopee` and no shadow named `shopee*`; `palette.test.ts` stays the source gate and now covers `border-t|r|b|l|x|y-` forms.
3. Verification: built CSS has no `gray`, `brand` or `shopee` utility; vitest, biome, `check-tokens`, build and the B1 e2e colour scenarios.

## Risks / Trade-offs

- [A class removed from the config now compiles to nothing] -> the source gate and the empty grep show none remain; the build CSS check confirms.
