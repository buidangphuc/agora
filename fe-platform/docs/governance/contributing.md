# Contributing to shared packages

A change to `packages/tokens`, `packages/ui` or `packages/api-client` must include:

1. The change itself, passing `npm run check` (lint, types, boundaries, disables, tokens, proto, catalog, tests).
2. `npm run catalog` output committed when a component, prop or token changed.
3. A changeset: `npx changeset` (pick the packages and the bump from `versioning.md`). Check: `npm run release:status`.
4. For a breaking change, a `## Migration` section in the changeset.
5. A missing variant or state is added to the component, never styled around in an app.

Review: the change owner is whoever owns `fe-platform` (see `CODEOWNERS`); the reviewer confirms the bump size, the migration text and that no disable comment was added.

Releasing (owner only): `npm run release:version`, review the diff and CHANGELOGs, commit, then `npm run release:publish` against the registry in `infra/registry/README.md`.
