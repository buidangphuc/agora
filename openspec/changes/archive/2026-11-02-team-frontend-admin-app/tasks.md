## 1. Platform

- [x] 1.1 Vendor script iterates over app targets and skips missing apps; tests; verify `npm run check` in `fe-platform`

## 2. Admin app

- [x] 2.1 Scaffold `team-frontend-admin` (package, lockfile, configs, Dockerfile, `.dockerignore`, proto, README); verify `npm run check`
- [x] 2.2 Admin session (`@fe-platform/session` admin preset), middleware, login without registration, scope gate with "Không có quyền"; verify the session, middleware, login and layout tests
- [x] 2.3 `robots.txt` and `X-Robots-Tag`; verify the header and robots tests
- [x] 2.4 Move the five screens, `features/admin`, the metrics route and the resolve action with their tests; copy the helpers they need; admin shell nav; verify the moved tests pass

## 3. Buyer app isolation

- [x] 3.1 Delete the admin routes, features, gateway modules and links; add the source-scan test; verify `npm run check` and `npm run build` in `team-frontend`

## 4. Infra and e2e

- [x] 4.1 Compose service, gateway CORS and trusted proxies, AGENTS.md, repo-doctor, CI workflow; verify `repo_doctor.py`
- [x] 4.2 `ADMIN_BASE_URL`, admin page objects and steps, 404 scenario, FEATURES.yaml paths; verify module imports, scenario collection and `features.py --strict`

## 5. Verification

- [x] 5.1 `npm run check`, `npm run build`, `docker build` in `team-frontend-admin`
- [x] 5.2 `openspec validate team-frontend-admin-app --strict`
