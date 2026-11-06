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
