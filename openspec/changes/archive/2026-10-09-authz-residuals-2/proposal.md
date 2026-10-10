## Why

Two authorization residuals survived `port-edge-authz-residuals`:
- `ForceFailSaga` and the other admin overrides on team-order are gated by the generic `admin` role marker, not by a
  scope that belongs to the order domain. team-order already half-accepts an `order.admin` scope in code, but team-identity
  never issues it and no spec names it.
- The gateway verifies a bearer token only when a Connect stream opens. `StreamChat` can stay open after the token's
  `exp` has passed or its session was revoked, so a logged-out or expired caller keeps receiving data.

## What Changes

- **team-identity**: the admin role is issued a new scope `order.admin`, next to `admin`. Buyer and seller never get it.
- **team-order**:
  - `ForceFailSaga` requires both `admin` and `order.admin` (the existing gate plus the new scope).
  - Every other admin override on an order RPC (`UpdateOrderStatus`, `GetSagaState`, returns, shipment, order view) is
    granted by `order.admin` alone; the bare `admin` marker no longer opens order data. Owner and seller paths are
    unchanged. `CreateOrder` stays USER-only.
- **team-gateway**: a server stream ends with `unauthenticated` once its token's `exp` passes, and within
  `STREAM_REVOCATION_CHECK_SECONDS` of its session being added to the in-memory revocation denylist the gateway already
  keeps. The only Connect stream today is `ChatService/StreamChat`. `adminProcedures` is unchanged.
- **platform-e2e**: scenarios for all of the above, prefix `ar2_`.

Repos touched: team-identity, team-order, team-gateway, platform-e2e. **No proto change.**

**BREAKING:**
- An admin token issued before this change has no `order.admin`; until the admin logs in again, `ForceFailSaga` and the
  admin order overrides answer 403 (tokens live at most `JWT_TTL`).
- An open `StreamChat` is cut at token expiry.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `auth`: the admin role is issued `order.admin`.
- `order-read-access`: `ForceFailSaga` needs `order.admin` as well as `admin`; admin overrides across order RPCs need
  `order.admin`.
- `edge-stream-and-http-policy`: a stream ends when its token expires or its session is revoked.

## Non-goals

- Server-push of revocation to the gateway (it keeps the denylist it has; see design D3).
- `/api/events/live` (SSE) token lifetime. It is a plain-HTTP route, not a Connect stream; listed as a follow-up.
- Stream max duration and concurrent-stream caps.
- Any proto change or new service-to-service scope.

## Impact

- **New env var (team-gateway):** `STREAM_REVOCATION_CHECK_SECONDS` (default `5`). The default needs no compose or
  gitops edit; set it only to tune.
- Admins must re-login after team-identity is redeployed to obtain `order.admin`.
- e2e `ForceFailSaga` scenarios already use the seeded admin, so they keep working once the identity image is rebuilt.
