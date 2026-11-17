## Context

`/account/security` is the template: server page, `redirectGuestToLogin`, `settle()` reads, `AccountShell`, client
buttons using `usePendingAction` and `ActionResult`. `src/lib/gateway/sessions.ts` shows the per-request gateway
client built from the `session` cookie.

## Decisions

- One page, four cards (consents, history, export, deletion) so each read can fail independently with its own retry,
  like the security page. Reads: `ListMyConsents` (page 20) and `GetAccountDeletionStatus` through `settle()`.
- Consent save is ledger first: the action calls `RecordConsent` with only the purposes whose value differs from the
  latest recorded value, source `account`, version `"1"` (the cookie format version); only on success the client writes
  the cookie via `writeConsent` and dispatches the existing `agora:consent:change` event. Why: a cookie that says
  granted without a stored record would be unprovable; on failure the cookie stays unchanged. No change means no call.
- The effective "current" value per purpose is the newest record for that purpose; if none exists the cookie value is
  shown and a save records the difference against `false`.
- Export is a server action returning `{filename, content}` (string, <= 1 MiB) and the client turns it into a `Blob`
  download. Why not a route handler: a failing GET would navigate the user to a raw error body; the action lets the
  page show an inline Vietnamese error. The filename is `agora-du-lieu-<YYYY-MM-DD>.json` from the response
  `generated_at`.
- Deletion is a modal with a password field. The password goes only into the server action; the modal clears it on
  close and on any result. A blocker (`FAILED_PRECONDITION`) renders in the modal as the fixed prefix "Chưa thể xóa
  tài khoản: " plus the gateway message, which team-identity limits to the category (open orders, open returns, wallet
  balance, pending payouts) with no ids or amounts. All other codes map to fixed texts.
- After a successful request the page shows the pending panel (erase time, cancel button, note that other devices were
  signed out and that the user may keep using this session until then). Pending state changes revalidate
  `/account/privacy`.
- No client-side retries of write calls; read failures use `RetryButton`.

## Error mapping (never the raw message, except the blocker category)

| gateway code | action | text |
|---|---|---|
| Unauthenticated | all | "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." |
| PermissionDenied | request deletion | "Mật khẩu không đúng." |
| InvalidArgument | request deletion (empty password) / consent | "Vui lòng nhập mật khẩu." / "Lựa chọn không hợp lệ." |
| FailedPrecondition | request deletion | prefix plus category message |
| FailedPrecondition | cancel | "Không có yêu cầu xóa tài khoản đang chờ." |
| Unavailable | request deletion | "Chưa kiểm tra được đơn hàng và số dư của bạn. Vui lòng thử lại sau." |
| other | each action | fixed per-action failure text |
