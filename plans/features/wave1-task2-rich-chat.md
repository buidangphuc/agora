# [W1] F2 Rich chat messages (team-chat)

## Role
SE

## Objective
Implement the RPC(s) below in **team-chat**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-chat/)
- team-chat/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: SendMessage / GetThreadMessages in internal/{handler,service,repository}

## Contracts you implement
Extend `ChatMessage` with `MessageType` enum {TEXT, LISTING_CARD, QUICK_REPLY} + optional `listing_id`/`payload`; `SendMessage` accepts message_type+payload; add `ListQuickReplies` (seller templated replies).

## Reference implementation
Mirror **SendMessage / GetThreadMessages in internal/{handler,service,repository}** in team-chat: same handler/service/repository layering, same test layout, new domain. Migration: add message_type/payload columns to chat_messages; quick_replies table.

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-chat && buf generate && go test ./...

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
