# ADR-0004: Local registry now, Changesets, packages ship source

Status: accepted, 2026-10-29 (confirmed by the user at the start of P6).

## Decision

- Distribution uses a private npm registry. For now it is Verdaccio in Docker bound to 127.0.0.1 (`infra/registry/`), with anonymous publish. Before any real distribution a shared registry with authenticated publish and scoped tokens in CI secrets replaces it; only the registry URL (`publishConfig.registry`, consumer `.npmrc`) changes.
- Versions and changelogs come from Changesets. Policy lives in `docs/governance/`.
- `tokens`, `ui` and `api-client` ship TypeScript source (tokens also `dist/tokens.css`). Consumers list them in `transpilePackages`. `config` and the template stay private.

## Why

The project is local first and has no reachable company registry; Verdaccio exercises the full publish and install path today. Changesets enforces a bump and a changelog per change and rewrites internal ranges. Shipping source avoids a build pipeline while there is one consumer framework.

## Reopen when

A company registry is available (switch URL, require auth), a non-Next consumer appears (then compile the packages), or release volume makes a manual publish step a bottleneck (automate in CI).
