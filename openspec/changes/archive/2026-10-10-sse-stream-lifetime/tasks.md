## 1. Code track (team-gateway)

- [x] 1.1 team-gateway: `authorizeRoom` returns the resolved principal; `ServeHTTP` watches it with `watchStream`, writes the final `unauthenticated` SSE event at `exp` or revocation, closes, unsubscribes; verify unit tests: expiry cut, revocation cut, final event then EOF with no later data, client-close writes no final event, public room unaffected, watcher goroutine and broker subscription released; tests fail without the change; `make check`

## 2. E2E track (platform-e2e; `ssl_` step modules; team-gateway FEATURES.yaml)

- [x] 2.1 FEATURES.yaml acceptance lines (team-gateway) with status `planned` and note `needs rebuild`; verify `make -C platform-e2e features-check`
- [x] 2.2 `security/ssl_sse_lifetime.feature` + `ssl_sse_steps.py`: expiry cut, revocation cut (private gateway, `STREAM_REVOCATION_CHECK_SECONDS=1`), public room unaffected; verify ruff, black and collection

## 3. Review and verify

- [x] 3.1 `openspec validate sse-stream-lifetime --strict`, `make -C platform-e2e features-check`
- [x] 3.2 After rebuild of the gateway: run the feature green (integrator)

## 4. Archive

- [x] 4.1 `openspec archive sse-stream-lifetime`

## Evidence (2026-10-10)

- team-gateway `make check` passes. Its expiry, revocation and nothing-after-cut unit tests fail without the change.
- After rebuilding the gateway, test_ssl_sse_lifetime (3) and test_ar2_stream_lifetime (3) passed 6/6, twice.
