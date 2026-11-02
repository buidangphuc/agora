## 1. ADR

- [x] 1.1 Write `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md` (supersedes the "no mapping into team-frontend" decision; three-app model, shared-versus-app split, vendored delivery, per-app sessions and cookie names, admin isolation, ports, merge order); verify it states each decision with its reason and links ADR-0004 and ADR-0005

## 2. Vendor tooling in fe-platform

- [x] 2.1 Add `scripts/vendor.mjs` and the `vendor` / `vendor:check` scripts with `vendor.targets` in `package.json`; verify `npm run vendor` writes the three tarballs into `../team-frontend/vendor/` and a second run changes nothing
- [x] 2.2 Add `vendor:check` to `npm run check`; verify it passes, then fails after an edit to a package source without repacking, then passes after repacking
- [x] 2.3 Add a unit test for the byte-comparison helper (equal, missing, different); verify the test runs in the fe-platform suite

## 3. team-frontend consumes the packages

- [x] 3.1 Add the three `file:./vendor/*.tgz` dependencies, refresh `package-lock.json`, add `transpilePackages`; verify `npm ci` works
- [x] 3.2 Add `src/app/fe-platform-packages.test.ts`; verify it passes and that team-frontend's `npm run check` stays green with no UI change
- [x] 3.3 Clean-copy check: copy `team-frontend/` alone, run `npm ci`, `npm run proto`, `npm run check`, `npm run build` there, and build the Docker image from it if the daemon is available; record the result

## 4. Docs and done-condition

- [x] 4.1 Document the update recipe (`npm run vendor`, commit tarballs and lockfile, changeset) in `fe-platform/docs/agents/project.md` and `team-frontend/README.md`
- [x] 4.2 Done-condition: fe-platform `npm run check` and `npm audit` green after a fresh `npm ci`; team-frontend `npm run check` green; `openspec validate team-frontend-foundation --strict` passes; `repo-doctor` shows no new drift in scope
