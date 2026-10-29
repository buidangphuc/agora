# Versioning policy

Published packages: `@fe-platform/tokens`, `@fe-platform/ui`, `@fe-platform/api-client`. Versions follow semver and are produced by Changesets (`npm run release:version`).

## What is breaking (major)

Tokens
- Removing or renaming a semantic token, a primitive key or a component class-map key.
- Pointing a semantic token at a different primitive family (for example `action-primary` from red to blue).

ui
- Removing or renaming a component, a prop, or a `variant`/`size` value.
- Making an optional prop required.
- Changing a component between `client` and `server` in the catalog.

api-client
- Removing an export, changing an `ApiError` kind, or a proto change that `buf breaking` reports for a message or RPC the client generates.

## Minor and patch

- Minor: adding a token, component, prop or variant; deprecating something (see `deprecation.md`); moving a token within the same family while every declared contrast pair still passes.
- Patch: fixes with no change to names, props or values consumers rely on.

## Rules

- A breaking change needs a `major` changeset with a `## Migration` section giving the exact rename or replacement.
- A peer or internal dependency range change that forces consumers to upgrade another package is at least minor.
- Consumers pin with caret ranges and upgrade by following `upgrade-guide.md`.

Check: `npm run release:status` fails when a published package changed without a changeset; a reviewer checks the bump size against this page.
