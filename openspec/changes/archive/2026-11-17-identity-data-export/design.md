## Context

`PrivacyService` is declared by `trust-legal-contract` and registered on the identity gRPC server by an earlier
slice (`identity-legal-consent`, extended by `identity-account-deletion`). This slice adds one handler on it. Data
comes from the existing identity tables (`users`, `user_addresses`, `sessions`, `login_history`), the consent ledger
and the deletion columns introduced by the earlier slices; no schema change here.

## Decisions

- Synchronous, single request, JSON built in memory with `encoding/json` (locked: export v1 is synchronous, identity-only).
- Document shape (`schema_version` 1), keys in this order: `schema_version`, `generated_at` (RFC 3339 UTC),
  `account` {`user_id`, `username`, `roles`, `status`, `created_at`, `seller_terms_version`, `seller_terms_accepted_at`},
  `addresses[]`, `sessions[]`, `login_history[]`, `consents[]`, `deletion` {`pending`, `erase_after`},
  `truncated` {`login_history`, `sessions`} booleans. Absent timestamps are `null`.
- Excluded by construction: the store export methods select named columns, never `SELECT *`, so
  `password_hash`, refresh token hashes, handoff codes and `status_reason` cannot appear.
- Size control: sessions and login history are read `ORDER BY created_at DESC LIMIT 1000`; the `truncated` flags say
  when the cap applied. With these caps and 20 addresses the document is well below 1 MiB; if the encoded
  document still exceeds 1 MiB the login history is halved until it fits (flag set). It never returns an error for size.
- Consents are listed newest first, all rows, no paging (a user has at most a few dozen records).
- Response: `data` = JSON bytes, `content_type` = `application/json; charset=utf-8`, `generated_at` equals the
  document's `generated_at`.
- Auth: `auth.RequireUser` as the first statement, like `AddressHandler`; the id is never read from the request.
- Read consistency: all reads run in one read-only transaction so the file is a snapshot. Any error aborts the whole
  call (`UNAVAILABLE` for database connectivity, `INTERNAL` otherwise, through `statusErr`); no partial file.
- Logging: one info line with record counts and byte size; no ids, no content (go-sdk sloglint forbidden keys).
- Gateway: the `PrivacyService` forwarder from earlier slices gets an `ExportMyData` method that uses the read-retry
  path and sets `Cache-Control: no-store` on the response headers. Anonymous callers are refused at the edge with
  UNAUTHENTICATED before any forward.
- Not audited: `AuditService` has no export action in the 26-action catalogue of the initiative; adding one is out of
  scope (see open issues of the slice review).

## Risks

- A later slice adding a column to `users` is not exported automatically; the export test pins the exact key set so
  that change forces a conscious decision.
