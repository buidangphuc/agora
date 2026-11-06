## 1. Wording and admin gate

- [x] 1.1 Rewrite the gateway-service-runtime edge.request requirement to name `principal_kind` and `principal_fp`
- [x] 1.2 Rewrite the go-platform-sdk logx requirement to name `principal_fp` and forbid the raw id
- [x] 1.3 Promotion admin RPC comments say scopes `admin` and `promotion.admin`; copies synced
- [x] 1.4 `WriteAuditEventRequest` admin-path comment names team-promotion's scopes; copies synced
- [x] 1.5 Admin app voucher and campaign pages and actions gate on `admin` and `promotion.admin` (red tests first)
- [x] 1.6 `openspec validate scope-and-log-wording --strict`
