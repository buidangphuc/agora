## Why

Specs and proto comments still describe rules the code no longer follows:

- The gateway and the SDK log handler stopped writing the raw principal id (`edge.request` carries `principal_fp` and `principal_kind`; `logx.NewHandler` adds `principal_fp`), but `gateway-service-runtime` and `go-platform-sdk` still name `principal` and `principal_id`.
- team-promotion admin RPCs require both `admin` and `promotion.admin` (`requireAdmin` in `team-promotion/internal/rpc/common.go`), but the promotion proto comments and the `WriteAuditEventRequest` admin-path comment still say scope `admin` only, and `promotion-admin-contract` and `audit-query-contract` quote that wording.

## What Changes

- `gateway-service-runtime`: `edge.request` names `principal_kind` and, for a user or service principal, `principal_fp`; it never carries the raw id.
- `go-platform-sdk`: `logx` records carry `principal_fp` (keyed under `LOG_KEY_FP_SECRET`) instead of `principal_id`.
- `promotion-admin-contract`: the 7 admin RPC comments state scopes `admin` and `promotion.admin`.
- `audit-query-contract`: the admin-path rule names team-promotion's `admin` and `promotion.admin`.
- `promotion-admin-vouchers` and `flash-sale-admin`: the admin app voucher and campaign screens and server actions require both `admin` and `promotion.admin`, matching team-promotion; a token with only `admin` gets the forbidden screen instead of a gateway 403.
- Proto comments in `promotion.proto` and `audit.proto` updated and copied byte-identical to every consumer. No wire change.

## Capabilities

### New Capabilities

### Modified Capabilities
- `gateway-service-runtime`: edge.request log fields.
- `go-platform-sdk`: identifiers added by the logx handler.
- `promotion-admin-contract`: admin RPC comment wording.
- `audit-query-contract`: admin-path rule wording.
- `promotion-admin-vouchers`: admin voucher screens gate on `admin` and `promotion.admin`.
- `flash-sale-admin`: admin campaign screens gate on `admin` and `promotion.admin`.

## Impact

team-frontend-admin: `src/lib/gateway/scopes.ts` (`isPromotionAdmin`) used by the 5 voucher/campaign pages and their 3 action modules. Proto comments only, no wire change. Data classification: unchanged.

## Non-functional requirements

- No wire change: `buf breaking` against main stays clean; proto copies stay byte-identical to `platform-core`.
- The admin app adds no request: the scope check reads the session cookie already decoded by `getPrincipal`.

## Data classification

Internal. No new personal data is collected, stored, logged or sent. The specs stop describing a raw user id in logs, matching the code.

## Threat model

- Elevation of privilege: a token holding `admin` without `promotion.admin` could open voucher and campaign screens; the gateway and team-promotion already refuse its calls, and the admin app now refuses it first. The server actions re-check the scopes, so a hand-crafted action call is refused before any request.
- Information disclosure: unchanged; the specs now forbid the raw principal id in logs, which the code already enforces.

