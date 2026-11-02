## 1. Code track (team-gateway, platform-core)
- [ ] 1.1 Embed `sdkconfig.LogKeyFP` in gateway config (as team-identity does); no new env name (GW-FP-9, GW-FP-10, GW-FP-10b)
- [ ] 1.2 Build `kafkax.NewFingerprinter(cfg.LogKeyFP.Secret)` at boot and inject into `Edge`; exit non-zero on error (GW-FP-9)
- [ ] 1.3 Add helper `principalLogAttrs(p)` returning `principal_fp` + `principal_kind` (anonymous omits fp); use it in `logRequest` (interceptors.go) and the HTTP wrapper (http.go); remove `"principal"` keys (GW-FP-1..5, 8)
- [ ] 1.4 Audit other gateway log sites (`logUpstreamError`, rate-limit, stream paths) for raw ids; fix any (GW-FP-7, GW-FP-11)
- [ ] 1.5 Update `edge_logging_test.go` expectations (`principal` -> `principal_fp`,`principal_kind`) and add log-capture tests (GW-FP-1..8, 4a, 4b, 6a-6c); config tests (GW-FP-9, 10, 10b, 11)
- [ ] 1.6 Declare `LOG_KEY_FP_SECRET` in `.env.example`, README env table, `docker-compose.services.yaml` (local value); confirm `platform-gitops/platform/vault-config/vault-config.yaml` seeds team-gateway's bag and `secrets.enabled` is on in `envs/services/team-gateway.yaml` and `argocd/apps/team-gateway.yaml`; no literal (GW-FP-12a, 12b, 12c)
- [ ] 1.7 Add `principal` to `forbidden-keys` in `platform-core/packages/go-sdk/.golangci.yml`; run lint across all Go repos, expect zero findings (GW-FP-13a, GW-FP-13b)

## 2. E2E track
- [ ] 2.1 Add a step reading `docker logs` of the gateway container (reuse `container_logs` helper) asserting `principal_fp` format, `principal_kind=user`, absence of the test user's id (GW-FP-14)
- [ ] 2.2 Note in e2e-author handoff: gateway container must start with `LOG_KEY_FP_SECRET` from compose (GW-FP-12a)
