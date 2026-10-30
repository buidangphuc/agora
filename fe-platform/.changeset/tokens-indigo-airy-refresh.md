---
"@fe-platform/tokens": major
---

New visual direction (ADR-0005): the primary scale moves from orange to indigo, neutrals become a cooler scale, `promo`, `danger` and `success` are recoloured, radius `3xl` and `shadow-card`/`shadow-pop` are added, controls use `min-h-touch`. Token data holds raw numbers and structured shadows; `@fe-platform/tokens/tokens.json` is new, as are the `touch` min-height, shared breakpoints and `pt-safe`/`pb-safe` utilities.

## Migration

No token, class or export name changes. Appearance changes: brand colour, surfaces, radii, shadows and control heights. Consumers that hard-coded the old brand hex outside the token system must update it; consumers using only token classes need no code change.
