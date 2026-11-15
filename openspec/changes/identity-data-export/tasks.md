## 1. Code track

- [ ] 1.1 Tests first in `team-identity/internal/rpc/export_test.go`, red before the handler exists: E1, E2, E3, E4, E6, E7, E8, E9, E10, E11, E13, E15
- [ ] 1.2 `team-identity/internal/store`: read-only export methods with named columns, newest-first order, `LIMIT 1000` for sessions and login history, one read-only transaction (E1, E2, E9, E10, E13)
- [ ] 1.3 `team-identity/internal/store/export_pg_test.go` with `GO_SDK_REQUIRE_DOCKER=1`: reference-account latency p95 (E12) and the 1000/1001 boundary against Postgres (E9, E10)
- [ ] 1.4 `team-identity/internal/rpc/export.go`: `PrivacyHandler.ExportMyData`, `auth.RequireUser` first, JSON builder, size shrink loop, counts-only log line (E1-E11, E13, E15)
- [ ] 1.5 `team-identity/internal/authz/coverage_test.go` and the RPC table: register `ExportMyData` as user-only (E5, E6)
- [ ] 1.6 `team-gateway/internal/edge/privacy_export_test.go` red first, then the forwarder method with read-retry and `Cache-Control: no-store`; anonymous refused before forward (E5, E14)
- [ ] 1.7 Gates through `scripts/asdlc/gate_digest.py` for team-identity and team-gateway; `openspec validate identity-data-export --strict`

## 2. E2E track

- [x] 2.1 `platform-e2e/tests/e2e/features/identity/data_export.feature` with scenarios E1, E2, E3, E5, E7, E9, E14 written against real RPCs through the gateway; steps registered in the identity steps module (E1, E2, E3, E5, E7, E9, E14)
- [x] 2.2 Seed helpers in the step file: create users, addresses, login events through real Login calls, a deletion request through `PrivacyService.RequestAccountDeletion` (E1, E3, E9)
- [ ] 2.3 Run the feature on the `mainx` stack with only team-identity, team-gateway, Postgres and Kafka up (E1-E14)
