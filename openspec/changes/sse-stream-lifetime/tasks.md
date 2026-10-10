## 1. Code track (team-gateway)

- [ ] 1.1 team-gateway: `authorizeRoom` returns the resolved principal; `ServeHTTP` watches it with `watchStream`, writes the final `unauthenticated` SSE event at `exp` or revocation, closes, unsubscribes; verify unit tests: expiry cut, revocation cut, final event then EOF with no later data, client-close writes no final event, public room unaffected, watcher goroutine and broker subscription released; tests fail without the change; `make check`

## 2. E2E track (platform-e2e; `ssl_` step modules; team-gateway FEATURES.yaml)

- [ ] 2.1 FEATURES.yaml acceptance lines (team-gateway) with status `planned` and note `needs rebuild`; verify `make -C platform-e2e features-check`
- [ ] 2.2 `security/ssl_sse_lifetime.feature` + `ssl_sse_steps.py`: expiry cut, revocation cut (private gateway, `STREAM_REVOCATION_CHECK_SECONDS=1`), public room unaffected; verify ruff, black and collection

## 3. Review and verify

- [ ] 3.1 `openspec validate sse-stream-lifetime --strict`, `make -C platform-e2e features-check`
- [ ] 3.2 After rebuild of the gateway: run the feature green (integrator)

## 4. Archive

- [ ] 4.1 `openspec archive sse-stream-lifetime`
