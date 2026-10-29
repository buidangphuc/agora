## Context

See proposal.md. ADR-0001 chose versioned packages and said a private registry would be set up in P6. Today the three packages are `private`, version 0.1.0, ranges `*` between them, and `ui` and `api-client` ship TypeScript source (the template lists them in `transpilePackages`). Docker is available locally; no company registry is reachable from this project, and the project is local first. The Obsidian notes frame governance as strict versus loose, modular versus integrated, centralised versus distributed; this platform is strict (gates enforce rules), modular and centralised (one owner), so the policy is short and mechanical.

## Goals / Non-Goals

**Goals:** a written definition of breaking and a deprecation path; real versioning and publishing through a registry; a consumer that proves the upgrade path across one breaking token change.
**Non-Goals:** company registry, release CI, signing, docs site, compiling packages.

## Decisions

1. **Changesets** for versions and changelogs. Alternative: manual bumps, rejected as error-prone. `.changeset/config.json` sets `access: restricted`, `baseBranch: main`, `updateInternalDependencies: patch`, and ignores `@fe-platform/config` and `@fe-platform/app`. Every PR that changes a published package adds a changeset file; `release:status` (`changeset status --since=main`) fails otherwise.
2. **What is breaking** (versioning policy):
   - Tokens: removing or renaming a semantic token, primitive key or component class-map key; changing a semantic token to a different primitive family (for example `action-primary` from red to blue) is breaking; nudging within a family that still passes the contrast gate is minor.
   - ui: removing or renaming a component, a prop or a variant/size value; making an optional prop required; changing a `client`/`server` classification.
   - api-client: anything the proto generation reports as breaking in a consumed message or RPC, removing an export, changing `ApiError` kinds.
   - Adding any of the above is minor; fixes without API change are patch.
3. **Deprecation:** mark with a `@deprecated <replacement and removal version>` JSDoc tag in a minor release, keep working, list it in the changelog; remove only in the next major and at least one minor later. `scripts/catalog.mjs` copies the tag text into the catalog entry as `deprecated`, so agents and reviewers see it.
4. **Packaging.** `tokens`, `ui`, `api-client`: remove `private`, add `files` (`src`, `dist` for tokens, excluding tests and fixtures), `publishConfig.registry` set to `http://localhost:4873`, `license` as UNLICENSED. Internal ranges become `^0.1.0` (changesets rewrites them). TypeScript source stays the artifact; the consumer sets `transpilePackages` (documented in the upgrade guide).
5. **Registry:** Verdaccio 5 in Docker, `infra/registry/docker-compose.yaml` publishing only `127.0.0.1:4873`, `config.yaml` allowing anonymous read/publish for `@fe-platform/*` and proxying everything else to npmjs for installs only. Anonymous publish is acceptable only because it is loopback and dev-only; ADR-0004 states the production requirement (authenticated publish, scoped tokens in CI secrets). Storage is a named Docker volume so versions survive restarts.
6. **Publish path:** `npm run release:version` (changeset version, then refresh the lockfile), review and commit, then `npm run release:publish` (`changeset publish`, which uses each package's `publishConfig`). Neither runs from check, CI or hooks.
7. **Consumer app** at `examples/consumer` (outside `workspaces`, own lockfile, `.npmrc` mapping `@fe-platform:registry`): a Next page using Button, Link, Table and one hand-written token class (`border-border-subtle`). `npm run check` there runs a class check (Tailwind compile of the consumer's sources plus the installed packages' class maps; every used class must generate CSS) and `next build`. This is what turns a silent token rename into a failure.
8. **Rehearsal** (not a real rename): publish 0.1.0 from the repository; in a throwaway git worktree rename `border-subtle` to `border-soft` in tokens, add a major changeset with a Migration section, version and publish; in the consumer run `npm install @fe-platform/tokens@latest @fe-platform/ui@latest`, observe the class check fail, apply the documented migration, observe check and build pass. Record commands, outputs and timings in `docs/governance/upgrade-rehearsal.md`. Remove the 2.0.0 versions from the local registry afterwards so the registry matches the repository.
9. **CI:** one PR-only step in `fe-platform-ci.yml` (checkout with full history) running `npm run release:status`. It is written and syntax-checked locally but not run on GitHub.
10. **ADR-0004** records: Verdaccio local registry now, authenticated shared registry before real distribution; Changesets; packages ship source.

## Risks / Trade-offs

- [Source-shipping packages force consumers to transpile] → documented; revisit if a non-Next consumer appears.
- [Anonymous publish on loopback] → dev-only, stated in the ADR and config; production needs authentication.
- [Rehearsal leaves 2.0.0 in the registry] → explicit cleanup step and check that the registry lists only 0.1.0 afterwards.
- [Changeset gate blocks docs-only PRs touching package folders] → config ignores `README`/test patterns via `changeset status` on package source only; if noisy, narrow the path filter.
- [Verdaccio image pull needs network] → one-time; recorded in the verification output.

## Migration Plan

Additive. Rollback: remove `.changeset/`, `docs/governance/`, `infra/registry/`, `examples/consumer`, the CI step and restore `private: true` and `*` ranges.

## Open Questions

none (Verdaccio local and Changesets confirmed by the user 2026-10-29).
