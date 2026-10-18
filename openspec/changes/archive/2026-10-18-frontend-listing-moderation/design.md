# Design

## Clarifications

### Round 1 — 2026-10-16

- A9 [state] Moderation UI: Not-allowed transition is explained: from which states may an admin REJECT, BAN or APPROVE? → **REJECT and BAN from PENDING_REVIEW and PUBLISHED; APPROVE only from PENDING_REVIEW; REJECTED and BANNED are never reversed (any other decision is FailedPrecondition, shown as the not-allowed message)** (human)
- A8 [state] Moderation UI: A save that ends pending is explained: is an edit of a PUBLISHED listing re-screened? → **Yes, every seller content edit of a PUBLISHED listing is re-screened and a flagged edit ends PENDING_REVIEW; the UI reports it as waiting for review** (human)
- A10 [error] Moderation UI: Delete of a banned or held listing is refused: may a seller delete a BANNED or HELD listing? → **No, DeleteListing answers FailedPrecondition; the UI shows a fixed message, never err.message** (human)
- A11 [data] Moderation UI: Screen flags never reach a seller surface: who may read screen_flags? → **Admins only; the owner sees reason_code and note only (the service strips flags for the owner; the seller mapper drops them as defence in depth)** (human)
- A19 [error] Moderation UI: Open report cap is explained: is there a cap on open reports per reporter, and how long is the close note? → **Max 20 OPEN reports per reporter (ResourceExhausted, shown as the cap message); ResolveReport note required, 1 to 500 characters after trim** (human)
- A20 [authz] Moderation UI: Banned listing is read-only: what does a restricted seller see on seller pages? → **A "Tài khoản bị hạn chế" notice instead of the /login redirect loop, owned by frontend-user-enforcement; this slice does not change the seller guard** (human)
- X1 [ownership] all scenarios: how are scenario titles kept unique across engagement-listing-reports, frontend-listing-moderation and frontend-user-enforcement? → **Every scenario title in this slice is prefixed "Moderation UI: "** (human)
- X2 [dependency] Moderation UI: Seller sees a clean listing as selling: does the mock screen let clean seed listings publish? → **Yes, the mock screen default allows clean text, so existing e2e seeds still auto-publish and only the marker term is flagged** (human)
- X3 [dependency] all frontend calls: does every RPC this slice calls have a gateway forwarder in a named slice? → **Yes: ModerateListing and ListModerationQueue in domain-listing-moderation; CreateReport, ListReports and ResolveReport in engagement-listing-reports; this slice adds no route** (human)
