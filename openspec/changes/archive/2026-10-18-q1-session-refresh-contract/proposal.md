## Why
Shoppers are hard-logged-out after the 1 h access token expires. The identity proto has no way to exchange a long-lived credential for a new access token. This slice adds the wire contract only (additive, buf-breaking safe) so identity, gateway and frontend slices can build against it.

initiative: q1-session-continuity

## What changes
In `platform-core/packages/proto/platform/identity/v1/identity.proto`:
- `AuthResult` gains `string refresh_token = 4` (opaque; empty when the server does not issue a refresh token: sessions disabled, or a grace replay) and `google.protobuf.Timestamp refresh_expires_at = 5`. `Login` and `Register` responses carry them through the existing `AuthResult`.
- `AuthService` gains `rpc RefreshSession(RefreshSessionRequest) returns (RefreshSessionResponse)`.
- New messages `RefreshSessionRequest { string refresh_token = 1; }` and `RefreshSessionResponse { AuthResult result = 1; }`.
- Doc comments state: public RPC (no Principal), not retried, errors are `Unauthenticated` for unknown, expired, revoked or reused tokens, and for an empty or malformed token (same code as unknown). Grace: replaying the immediately previous refresh token within 10 s (inclusive) returns a fresh access token for the same sid with an empty `refresh_token`; after 10 s it is reuse, returns `Unauthenticated` and revokes the session.
- Generated code (`buf generate`) is refreshed per the repo's normal flow. No behaviour is implemented here.

Out of scope: minting, rotation, storage, gateway routing, frontend (other slices).

## NFR
- Additive change only: `buf breaking` against HEAD passes; existing clients that ignore fields 4-5 keep working.
- Contract-level target for implementers: RefreshSession p95 < 100 ms at identity (initiative NFR).
- No new availability or limit requirements in this slice.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `AuthResult.refresh_token` | credential (secret) | authenticate a returning shopper | contract | 30 days sliding, session lifetime; only its SHA-256 hash is stored server-side. Must never be logged or sent to analytics |
| `AuthResult.refresh_expires_at` | non-personal metadata | let the client set cookie maxAge | contract | transient |
| `RefreshSessionRequest.refresh_token` | credential (secret) | as above | contract | transient on the wire, mirrored by a 30-day client cookie; not logged |

## Threat model
- Assets: refresh token (bearer secret), session.
- Abuse: stolen/replayed refresh token; token leakage via logs; unauthenticated brute force of the public RPC.
- Authz rule: RefreshSession is public (no Principal, like Login/Register); the token itself is the credential. Contract demands a single `Unauthenticated` for every failure class (no oracle distinguishing unknown/expired/revoked/reused). 256-bit random tokens make guessing infeasible. Reuse handling lives in identity-session-refresh.
