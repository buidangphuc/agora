# Design

## Clarifications
### Round 1 — 2026-10-15
- Field numbers, semantics and comments follows the Locked Contracts section of `plans/q3-audit-log/00-spec.md`, except `actor_id`/`target_type` stay non-optional (orchestrator decision; empty = unset).
- Existing `actor_id = 1` and `target_type = 2` stay plain proto3 strings (empty = no filter). Making them `optional` would change their presence semantics for existing clients and risks `buf breaking`; the new `target_id` and `action` use `optional` as in the Locked contract. Servers must treat an unset and an empty value the same for filtering.
- Human decision: uniform admin-path rule. Every call by a caller holding `admin` (team-order: `order.admin`) to a catalogue RPC is audited, including an admin who owns the resource. A caller without that scope (e.g. a seller on their own order) is never audited. The AGENTS.md rule and the catalogue comment use this wording, with no "not the owner" qualifier.
- `created_after`/`created_before` are message-typed, so unset is already distinguishable.
- Proto copies are partial trees per repo; each consumer already has `platform/common/v1/common.proto`, which `audit.proto` imports.
