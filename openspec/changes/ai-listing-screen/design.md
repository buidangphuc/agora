# Design

## Clarifications

### Round 1 — 2026-10-16
- X1 [cross-slice] Default mock settings allow ordinary seed listing text: Must the mock screen default ALLOW clean text so existing e2e seeds that publish listings still auto-publish? → **Yes; default `SCREEN_MOCK_MARKER=screen-review-marker`, no seed contains it, clean text is ALLOW** (human)
- A8 [data] Status 0 decodes to empty and is indexed: Must an omitted status (UNSPECIFIED, 0) stay distinguishable from DRAFT and from unknown numbers? → **Yes; the decoder maps 0 to `""` (indexed as legacy) and only other unknown numbers to `"UNKNOWN"` (deleted)** (human)
- A11 [privacy] LLM review verdict is returned with its labels: Who sees the screen labels? → **Admins only; team-ai returns labels to the service caller and stores none, team-domain keeps them admin-only (owner sees reason_code + note)** (human)
- A13 [privacy] Screening does not log listing text: May screen flags travel on ListingChanged events? → **No; ScreenListing output is never emitted on events by team-ai, and ListingChanged carries no flags** (human)
- R1 [privacy] Listing text is truncated and passed as data: Is listing text acceptable in Langfuse traces when `llm_router` is on? → **Treated as internal data; DPO sign-off on Langfuse traces with a real LLM is NEEDS_HUMAN before release; mock (default) sends nothing** (human)
- R2 [retention] Screening does not log listing text: What retention applies to the logged `listing_id` (empty on create)? → **Existing team-ai log retention; no new rule** (human)
- R3 [consistency] Every AIService RPC except ShoppingAssistant has a scope entry: Does the scope table cover every RPC? → **Every RPC except `ShoppingAssistant` (absent on purpose), verified by the existing `test_every_ai_rpc_is_gated_or_explicitly_open`** (human)
