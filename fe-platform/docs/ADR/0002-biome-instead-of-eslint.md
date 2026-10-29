# ADR-0002: Biome as the single linter and formatter

Status: accepted, 2026-10-29. Deviates from the planning notes, which assume ESLint and Stylelint.

## Decision

Biome (pinned in `package.json`) formats and lints. Rules Biome cannot express are covered by repo scripts in `scripts/` (import boundaries, disable-comment accounting; the token check arrives in P2). ESLint is added only for a rule with no Biome equivalent, and each addition is recorded here.

## Why

agora's `team-frontend` already uses Biome 1.9; one tool means one fast feedback command for agents and identical local and CI results.

## Rule-name verification

The notes warn that rule names are unverified. Findings against the installed Biome 1.9.4:

- `lint/a11y/*` rules exist in the recommended set (for example `useButtonType`, `useKeyWithClickEvents`, `noSvgWithoutTitle`), covering the clickable-`div` and missing-key-handler cases that `jsx-a11y` targets.
- Biome 1.9.4 ships `nursery/noRestrictedImports` (nursery, so its options may change) and `nursery/useImportRestrictions`. Verified in the installed `configuration_schema.json`. P3 enables `noRestrictedImports` with the banned UI libraries once the allowed set exists; layering rules (public vs internal, packages vs apps) are enforced now by `scripts/check-boundaries.mjs`. ESLint is not needed so far.

## Consequences

Disable comments (`biome-ignore`) are counted and must carry a reason; see `scripts/check-disables.mjs`.
