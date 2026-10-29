## 1. Policy and decision records

- [x] 1.1 Write `docs/governance/versioning.md`, `contributing.md` and `deprecation.md` (definition of breaking per package, changeset required, gates to run, deprecate-then-remove rule, review owner); verify each rule is checkable by a command or a reviewer question and that `AGENTS.md` links them
- [x] 1.2 Write ADR-0004 (local Verdaccio now, authenticated shared registry before real distribution, Changesets, packages ship source) with a reopen condition; verify the ADR index lists it

## 2. Changesets and packaging

- [x] 2.1 Add `@changesets/cli`, `.changeset/config.json` and the `release:status|version|publish` scripts; verify `npm run release:status` passes on an unchanged tree and fails after editing `packages/ui` without a changeset, then passes with one
- [x] 2.2 Make `tokens`, `ui`, `api-client` publishable (`private` removed, `files`, `publishConfig`, license, internal ranges `^0.1.0`); verify `npm pack --dry-run -w <each>` lists sources (and `dist/tokens.css`) but no tests, and `npm ci` plus `npm run check` still pass

## 3. Deprecation in the catalog

- [x] 3.1 Copy a `@deprecated` JSDoc tag into the catalog entry as `deprecated`; verify a catalog test with a fixture export, that current entries have no `deprecated` key, and `catalog:check` stays green

## 4. Registry

- [x] 4.1 Add `infra/registry/` (docker-compose bound to 127.0.0.1:4873, `config.yaml`, README with start, stop and publish); verify `docker compose up -d` serves the registry and `npm ping --registry http://127.0.0.1:4873` succeeds
- [x] 4.2 Publish the three packages (0.1.0, then tokens 0.1.1 through `npm run release:version` and `npm run release:publish`); verify `npm view @fe-platform/ui version --registry ...` returns 0.1.0 and installing the three into an empty directory resolves from the registry

## 5. Consumer app

- [x] 5.1 Create `examples/consumer` (own lockfile, `.npmrc` scoped registry, Next page with Button, Link, Table and `border-border-subtle`, `transpilePackages`); verify `npm ci` installs the packages from the registry and `next build` passes
- [x] 5.2 Add the consumer class check (every Tailwind class used must generate CSS) to its `npm run check`; verify green on 0.1.0 and red when a used class does not exist

## 6. Upgrade rehearsal

- [x] 6.1 In a throwaway worktree rename `border-subtle` to `border-soft` in tokens with a major changeset and a Migration section, version and publish 2.0.0 (and the dependent `ui` bump); verify the registry lists the new versions
- [x] 6.2 In the consumer, install the new versions, confirm the class check fails, apply the documented migration, confirm `npm run check` and the build pass; record commands, outputs and timings in `docs/governance/upgrade-rehearsal.md`
- [x] 6.3 Remove the rehearsal versions from the local registry and delete the throwaway worktree; verify the registry lists only the pre-rehearsal versions (tokens 0.1.0 and 0.1.1, ui 0.1.0) and the repository tokens still contain `border-subtle`

## 7. CI, docs and done-condition

- [x] 7.1 Add the PR-only `release:status` step to `.github/workflows/fe-platform-ci.yml` with full-history checkout; verify the YAML parses and the command matches the script
- [x] 7.2 Update `docs/agents/project.md`, `AGENTS.md` and the `fe-platform` skill to mention changesets and the registry; verify the check order in project.md still matches `npm run check`
- [ ] 7.3 Done-condition: consumer upgraded across one breaking token change following the written policy (6.2), `npm run check` exits 0 after a fresh `npm ci`, no new disable comments, `npm audit` clean, registry stopped, `openspec validate fe-platform-governance --strict` passes
- [ ] 7.4 e2e track: none (no user-facing capability, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift
