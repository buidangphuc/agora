# [W1-T1] Chat message search backend (team-chat)

## Role
SE

## Objective
`SearchMessages` works end-to-end in team-chat: given a query string, return the requesting user's chat messages whose `content` matches (ILIKE), paginated, scoped to threads the caller participates in. Unit tests green with `go test ./...`.

## Write-set (EXCLUSIVE — nothing outside team-chat/)
- team-chat/proto/** — vendor the updated chat.proto from platform-core (edit)  *(if the repo vendors proto; else skip — Wave 0 already updated platform-core)*
- team-chat/internal/repository/chat.go — add `SearchMessages(ctx, userID, query, page)` PG query (edit)
- team-chat/internal/service/chat.go — add SearchMessages service method (edit)
- team-chat/internal/handler/chat.go — add SearchMessages handler (edit)
- team-chat/internal/repository/chat_test.go, service/chat_test.go, handler/chat_test.go — tests (edit)
- team-chat/internal/generated/** — regenerated via `make proto`/buf (do not hand-edit)

## Read-only dependencies
- platform-core/packages/proto/platform/chat/v1/chat.proto (contract, Wave 0)
- Existing `ListThreads`/`GetThreadMessages` in the same 3 files = the reference implementation to mirror.

## Contracts you implement
```proto
rpc SearchMessages(SearchMessagesRequest) returns (SearchMessagesResponse);
message SearchMessagesRequest  { string query = 1; platform.common.v1.PageRequest page = 2; }
message SearchMessagesResponse { repeated ChatMessage messages = 1; platform.common.v1.PageResponse page = 2; }
```

## Reference implementation
Mirror `GetThreadMessages` (repo query + service + handler + auth from interceptor principal) — same files, same test layout. The query joins messages→threads and filters `(buyer_id = :uid OR seller_id = :uid) AND content ILIKE '%'||:q||'%'`, newest first, cursor-paginated like the existing list methods.

## Acceptance criteria
- [ ] Returns only messages in threads where the caller is buyer or seller (never leaks others' messages)
- [ ] Empty/blank query → empty result (no error), not a full-table scan return
- [ ] Pagination cursor honored (mirror existing PageRequest/PageResponse handling)
- [ ] ≥3 tests: match found, no-match empty, cross-user isolation
- [ ] `go test ./...` green

## Review (different agent)
Rubric = SE section of references/experts.md. Touches a new authed RPC + service boundary → also route to auth-scope-reviewer and contract-boundary-reviewer; require CLEAN.

## Verify
cd team-chat && (make proto 2>/dev/null || true) && go test ./...

## Out of scope
- Do NOT add per-message read receipts or edit ChatMessage fields.
- Do NOT touch team-gateway (forwarder = integration wave) or team-frontend.
- Do NOT edit platform-core proto (already done in Wave 0).
