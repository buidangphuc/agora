# Design

## Clarifications
### Round 1 — 2026-10-15
- Q4 [outcome] which ResolveDispute transitions are audited and with what metadata? → **every status change is audited, including to `investigating`; metadata `status` + trimmed `resolution` note + `order_id` accepted** (human)

Note: an `investigating` dispute is not closed, so the current service allows investigating to investigating again; each allowed call writes exactly one event.
