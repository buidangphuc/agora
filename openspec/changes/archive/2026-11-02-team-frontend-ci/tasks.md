## 1. Workflow and ignore file

- [x] 1.1 Add `.github/workflows/team-frontend-ci.yml` with the `check` and `image` jobs; verify it parses as YAML and that `actionlint` (not installed here, so yq parsed the YAML instead) reports nothing
- [x] 1.2 Add `team-frontend/.dockerignore`; verify the image still builds from a clean copy with the file present
- [x] 1.3 Add `team-frontend/vendor/**` to the `fe-platform-ci.yml` paths; verify the YAML parses

## 2. Local rehearsal

- [x] 2.1 In a clean copy of `team-frontend/` run `npm ci`, `npm run proto`, `npm run check`, `npm run build`, then `docker build`; verify every step exits 0 and record it

## 3. Docs and done-condition

- [x] 3.1 Note the workflow and the manual "required check" step in `fe-platform/docs/agents/project.md`
- [x] 3.2 Done-condition: `openspec validate team-frontend-ci --strict` passes, fe-platform and team-frontend `npm run check` green, `repo-doctor` shows no new drift; the report states the workflow has not run on GitHub
