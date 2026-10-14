Paths relative to `platform-core/`. E2E track: none (pure contract; behaviour is e2e-tested in identity-session-refresh and frontend-session-continuity).

## 1. Code track
- [x] 1.1 Add `refresh_token = 4` and `refresh_expires_at = 5` to `AuthResult` in `packages/proto/platform/identity/v1/identity.proto`; update the message comment (covers: Fields exist with the agreed numbers, Boundary sessions disabled).
- [x] 1.2 Add `rpc RefreshSession`, `RefreshSessionRequest`, `RefreshSessionResponse` (covers: Service and messages exist, Lint passes).
- [x] 1.3 Write the RPC doc comment: public, not retried, `Unauthenticated` for unknown/expired/revoked/reused, reuse revokes the session, empty/malformed token is `Unauthenticated`, and grace semantics: replaying the immediately previous token within 10 s inclusive returns a fresh access token for the same sid with EMPTY `refresh_token`; after 10 s it is reuse (`Unauthenticated`, session revoked) (covers: Error contract is documented, Empty or malformed token, Grace replay semantics).
- [x] 1.4 Run `make check` (lint + breaking) (covers: Change is backward compatible, Existing RPCs are untouched, Lint passes).

## 2. Generation and verification
- [x] 2.1 Run `buf generate`; add committed test `packages/proto/platform/identity/v1/authresult_roundtrip_test.go` (round-trip of Go `AuthResult` with fields 4-5 unset, plus field-number assertion) (covers: Fields exist, Boundary sessions disabled).
- [x] 2.2 Confirm Go, Python, JVM and TS generated outputs expose the new symbols (covers: Service and messages exist in every generated language).

## 3. E2E track
- [x] 3.1 None: no user-facing behaviour in this slice.
