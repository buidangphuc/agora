## Decisions

- Target is the authenticated principal only; the request has no user id.
- Atomic write: one DB transaction sets role, terms columns and inserts the outbox row (same pattern as `SetStatus`/`RevokeSession` enqueue functions). The status check and role write happen under a row lock so a concurrent `SetUserStatus(RESTRICTED)` and `BecomeSeller` serialise; ACTIVE is re-read inside the transaction.
- Token: `SignWithSession(user, scopes from ScopesForUser(roles incl. seller, status), sid, status, ttl)` with the caller's `sid`; no `RotateRefresh`, so the refresh token is unchanged and `AuthResult.RefreshToken` is empty.
- `SELLER_TERMS_VERSIONS` is a comma list; the first entry is current and is the only accepted value, compared byte for byte (no trim, no case folding). Older entries exist for record only.
- An existing seller who is not ACTIVE is refused before the idempotent path.
- Gateway: authenticated, user-type principal, no extra scope (like `CreateHandoffCode`), not in the public-RPC list.
- The seller app sends its configured current version (`SELLER_TERMS_VERSION`), which must equal the first `SELLER_TERMS_VERSIONS` entry; the gap row G15 removal belongs to slice `seller-onboarding-ui`.
