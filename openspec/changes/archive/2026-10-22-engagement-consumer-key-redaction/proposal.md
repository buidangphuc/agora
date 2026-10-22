# Proposal: engagement-consumer-key-redaction

initiative: privacy-audit-hardening

## Why
`team-engagement/internal/consumer/consumer.go` (`processRecord`, lines 171, 183, 189) logs the raw Kafka key on handler failure, DLQ produce failure and DLQ parking. Events such as `SellerFollowed`/`SellerUnfollowed` are keyed by the seller (user) id, so ops logs carry user ids (PII). Initiative decision Q7 (locked 2026-10-18): log a key fingerprint instead.

## What changes
- Every log line in team-engagement consumers that printed `key` prints `key_fp` instead: the first 12 lowercase hex characters of SHA-256(key bytes). The `key` attribute is removed from those lines.
- Same field name and format as team-domain slice `domain-consumer-key-redaction`.
- Kafka message keys, DLQ record keys, partitioning and retry/offset behaviour are unchanged (ordering stays per user).
- An empty or nil key logs the fingerprint of the empty input (`e3b0c44298fc`), never an empty string or panic.
- No proto, RPC or event changes.

## NFR
- Latency: one SHA-256 over a key (tens of bytes) per log line, only on failure paths; negligible.
- No change to availability, retry count, backoff or offset semantics.
- Fingerprint is deterministic so the same key correlates across log lines and pods.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| Kafka key (user/seller id) | personal (pseudonymous id) | routing/ordering only; no longer logged | n/a (removed from logs) | Kafka topic retention, unchanged |
| `key_fp` (12 hex of SHA-256) | derived, low-entropy truncation; not reversible to the id without brute force over known ids | ops correlation of failing records | legitimate interest (operations); DPO to confirm (human item, not decided here) | log retention, unchanged |

## Threat model
- Assets: user/seller ids in application logs.
- Abuse cases: log reader (ops, log vendor, leaked log export) learns which user an event concerns; residual risk: someone holding a candidate id list can hash it and match `key_fp` (accepted; 48-bit truncation, no salt, per Q7).
- Authz rule: none new; no endpoint added. Logs remain access-controlled by the platform.
