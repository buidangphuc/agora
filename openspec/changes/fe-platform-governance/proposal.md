## Why

The packages (`tokens`, `ui`, `api-client`) change together inside one workspace, so nothing yet says what counts as a breaking change, how a component is retired, or how an app outside the workspace gets updates. Without that, the first breaking token change either breaks consumers silently (a renamed token just stops generating CSS) or freezes the platform.

## What Changes

- Written governance in `fe-platform/docs/governance/`: a versioning policy (semver, with a definition of "breaking" for tokens, components, props and the generated client), contribution rules (what a change to a shared package must include), a deprecation policy (deprecate in a minor, remove no earlier than the next major), and a consumer upgrade guide.
- Changesets (`@changesets/cli`) for versioning and changelogs: `.changeset/config.json`, `release:status`, `release:version` and `release:publish` scripts. `tokens`, `ui` and `api-client` become publishable (`private` removed, `files`, `publishConfig`, internal dependency ranges); `config` and the template stay private.
- A private registry for local use: Verdaccio in Docker under `fe-platform/infra/registry/`, bound to 127.0.0.1, with the publish path documented. Moving to a company registry later changes the registry URL only.
- Deprecation is machine-visible: a `@deprecated` JSDoc tag on an export appears in `docs/agents/catalog.json`, so agents stop picking it.
- A consumer app outside the workspace, `fe-platform/examples/consumer`, installed only from the registry, with its own build and a check that every Tailwind class it uses still generates CSS.
- A recorded rehearsal: publish 0.1.0, then a throwaway copy makes one breaking token rename and publishes the major, and the consumer upgrades by following the written policy and migration note. Result in `docs/governance/upgrade-rehearsal.md`; the repository's tokens are not renamed.
- ADR-0004 records the registry and release-tool decisions.
- CI gets a PR step that fails when a published package changed without a changeset (written, not run on GitHub).
- No change to `team-frontend`.

## Capabilities

### New Capabilities
none (platform governance and distribution; no product behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New dev dependency `@changesets/cli`; new files under `.changeset/`, `docs/governance/`, `infra/registry/`, `examples/consumer/`.
- `packages/{tokens,ui,api-client}/package.json` metadata changes; `ui` and `api-client` depend on `tokens`/`next` with real ranges instead of `*`.
- `npm run check` gains nothing new for the main workspace except the catalog's deprecation field; the consumer has its own `npm run check`.
- Verdaccio runs only while testing, then is stopped.

## Non-goals

Publishing to a company or GitHub registry, release automation in CI, package provenance or signing, a documentation site, more than one rehearsed major, backport branches, compiling `ui` or `api-client` to JS (they still ship TypeScript source and consumers list them in `transpilePackages`), and renaming any real token in this repository.

## Non-functional requirements

- The registry listens on 127.0.0.1 only; no credential or token is committed.
- Publishing is an explicit script, never part of `npm run check` or a git hook.
- The consumer's install uses only the registry (no workspace link); its `npm ci` and build pass offline of the monorepo sources.
- `npm run check` stays under 60 s locally.

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Package tarballs and changelogs | internal | registry volume, local | loopback registry; scoped name `@fe-platform` |
| Registry config | internal | n/a | no secrets in the file; dev-only anonymous publish |

No personal data handled.

## Threat model

| Threat | Mitigation |
|---|---|
| Anyone on the network publishes to the local registry | bound to 127.0.0.1; documented as dev-only; the shared registry must require authentication (ADR-0004) |
| Dependency confusion for `@fe-platform/*` | scoped registry mapping in the consumer `.npmrc`; only the scope points at the registry |
| Breaking change ships as a minor | `release:status` in CI plus the written definition of breaking; rehearsal proves the major path |
| Accidental publish from a developer machine | publish script targets the registry from `publishConfig` only and is not wired to any hook |
