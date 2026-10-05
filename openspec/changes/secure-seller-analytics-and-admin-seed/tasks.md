## 1. Code — team-analytics

- [x] 1.1 Add `requireSellerAccess` and call it first in `GetSellerFunnel`, `GetRevenueBreakdown` and `GetDemandForecast`; verify unit tests for owner, other seller (PermissionDenied), anonymous (Unauthenticated) and admin, with `go build/vet/test ./...`
- [x] 1.2 Confirm by grep that no service principal calls these RPCs; record the result in the commit body

## 2. Code — team-identity

- [ ] 2.1 Config `SEED_ADMIN_ENABLED`, `SEED_ADMIN_USERNAME`, `SEED_ADMIN_PASSWORD` with fail-fast validation; `main.go` seeds only when enabled; verify config tests (disabled, enabled without password, short password, valid) and the env-example sync test
- [ ] 2.2 README: document the seed env and how to rotate or remove a pre-existing `admin` / `admin123` user; verify the README section exists

## 3. Config — compose and gitops

- [ ] 3.1 Local compose enables seeding with a dev password from env; e2e `users.json` reads the admin password from env with the same dev default; verify the compose file parses and `features.py --strict` passes
- [ ] 3.2 platform-gitops check that fails on `SEED_ADMIN_ENABLED=true` or a `SEED_ADMIN_PASSWORD` literal in team-identity manifests; verify it passes on the current manifests and fails on a crafted bad sample

## 4. E2E — platform-e2e

- [ ] 4.1 Scenarios: seller reads own funnel; seller cannot read another seller's revenue (403); anonymous forecast (401); admin reads any seller; FEATURES.yaml entries; verify green against the agora stack and flip to `automated`
- [ ] 4.2 Run `openspec validate secure-seller-analytics-and-admin-seed --strict`; verify it is valid
