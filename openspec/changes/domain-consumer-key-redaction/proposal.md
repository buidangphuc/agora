## Why

When a team-domain Kafka handler fails, `internal/consumer/consumer.go` logs `slog.String("key", string(key))` on the retry warning (line 171), the DLQ-produce error (186) and the DLQ-park warning (192). On `identity.events` the record key is the user id (the identity relayer keys by AggregateID: `SessionRevoked`, `UserStatusChanged`), so ops logs print raw user ids. The initiative decision (Q7) is to log a SHA-256 fingerprint instead, which keeps correlation across log lines without the id.

initiative: privacy-audit-hardening

## What changes

- `team-domain/internal/consumer/consumer.go`: a small unexported helper `keyFingerprint(key []byte) string` returns the first 12 lowercase hex characters of SHA-256(key). The three log calls replace `slog.String("key", ...)` with `slog.String("key_fp", keyFingerprint(key))`. No log attribute named `key` carrying the record key remains.
- The change sits in the shared `processRecord`, so it covers every consumer team-domain builds with `consumer.New` (today only the identity consumer in `internal/bootstrap/identity_consumer.go`; any later topic gets it for free).
- Kafka record keys, DLQ keys and payloads are unchanged (ordering stays per user). Log levels and the other attributes (`attempt`, `attempts`, `err`) are unchanged.
- An empty or nil key logs the fingerprint of the empty input (`e3b0c44298fc`), never an empty string, so a keyless record is recognisable.
- No new RPC, event, proto, config or dependency.

## Non-functional requirements

- Latency: one SHA-256 over a short key per log line, only on the failure path; the success path computes nothing.
- Availability: no behavioural change to retry, DLQ or offset-advance logic.
- Limits: fingerprint is fixed 12 hex chars (48 bits). It is a correlation aid, not a security control: user ids with low entropy could be brute-forced offline from a fingerprint. Accepted by the Q7 decision.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| record key on `identity.events` (user id) | personal (pseudonymous id) | partition ordering only; no longer logged | n/a for logs after this change | Kafka topic retention (unchanged) |
| `key_fp` log field | pseudonymous derivative, not directly identifying | correlate retry, park and DLQ lines of one record | legitimate interest (operations) | log-store retention (unchanged) |
| handler `err` text | may embed ids from wrapped errors | diagnosis | legitimate interest | log-store retention (unchanged) |

## Threat model

- Assets: user ids; ops log store.
- Abuse cases: (1) anyone with log read access harvests user ids from failing-event lines, prevented by logging only the fingerprint; (2) reversing a fingerprint, mitigated by truncation to 12 hex of a hash and accepted residual risk for guessable ids; (3) a future consumer reintroduces a raw key log, prevented by a test that fails when the raw key appears in any captured line.
- Authz rule: none; no endpoint or permission changes. Out of scope: ids inside handler error text (`err` attribute), flagged as a follow-up.
