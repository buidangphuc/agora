# Tasks

## Code track
- [ ] 1.1 Add unexported `keyFingerprint(key []byte) string` (first 12 hex of SHA-256) in `team-engagement/internal/consumer/consumer.go`; format identical to team-domain `key_fp`. (Engagement key redaction: fingerprint is exactly 12 hex and stable; empty key)
- [ ] 1.2 Replace `slog.String("key", string(key))` with `slog.String("key_fp", keyFingerprint(key))` at the three log sites in `processRecord`. (handler failure; DLQ parking; DLQ produce failure)
- [ ] 1.3 Confirm DLQ `Produce` and handler still receive raw key. (DLQ parking; successful records and key unchanged)
- [ ] 1.4 Log-capture tests (slog JSON handler into a buffer, fake DLQ) named in each scenario's VERIFIED BY; write red first. (all six scenarios)
- [ ] 1.5 grep the repo for any other consumer logging `key`; none expected beyond consumer.go. (all)

## E2E track
- [ ] 2.1 Compose e2e: publish a poison record keyed by a seller id to engagement.events, wait for DLQ parking, assert container logs contain `key_fp` and not the seller id, and the DLQ record key equals the id. (Engagement key redaction: DLQ parking logs fingerprint; handler failure logs fingerprint)
