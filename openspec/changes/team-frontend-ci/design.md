## Context

See proposal.md. `fe-platform-ci.yml` is the pattern: checkout, setup-node 22 with npm cache keyed on the lockfile, `npm ci`, `npm run check`, build. `team-frontend` needs one extra step before `check`: its generated Connect stubs (`src/generated`) are gitignored and made by `npm run proto` (buf with the local `protoc-gen-es` plugins from `node_modules/.bin`). The Dockerfile expects `src/generated` to exist in the context (the compose flow runs `buf generate` first), and has no `.dockerignore`.

## Decisions

1. **Two jobs, one workflow:** `check` (npm ci, proto, check, build) and `image` (npm ci --ignore-scripts, proto, `docker build team-frontend`). Splitting keeps a lint failure from hiding an image failure and lets the image job run in parallel.
2. **Triggers:** `pull_request` and `push` to `main` with `paths: ["team-frontend/**", ".github/workflows/team-frontend-ci.yml"]`, plus `workflow_dispatch`. A change to `fe-platform/**` that alters packages also changes `team-frontend/vendor/**` (the platform gate forces repacking), so the app workflow runs when needed.
3. **Image job proves the vendored build:** it builds from the `team-frontend/` directory only; no path outside it is available to the daemon, which is exactly the condition of the real deployment build. No image is pushed.
4. **`.dockerignore`:** `node_modules`, `.next`, `.git`, `*.tsbuildinfo`; `src/generated` stays in the context because the Dockerfile has no proto step.
5. **No token-script change:** verified that `scripts/check-tokens.mjs` only rejects raw hex, `rgb()`, arbitrary `[...]` values and inline colours; fe-platform utilities are plain class names, so nothing needs allowing until B1 changes the palette.
6. **fe-platform-ci paths:** add `team-frontend/vendor/**` so a hand-edited tarball is caught by `vendor:check`.
7. **Validation without GitHub:** parse the YAML, then execute each step's command in a clean copy of `team-frontend/` (including the `docker build` with the new `.dockerignore`), and check `actionlint` if available.

## Risks / Trade-offs

- [The workflow cannot be proven on GitHub from here] -> every step is run locally in the same order; the first PR run is the real test and the report says so.
- [`.dockerignore` could hide a file the build needs] -> the Docker build is rerun after adding it and `npm run build` must succeed in the image.
- [Marking the checks required is a human setting] -> noted in the report; not done here.
