## Why
Review integrity (Q2) needs an admin "hide review" operation and a documented rule that reviews are verified and tied to a completed order. The engagement proto has neither. This slice adds the wire contract only (additive, buf-breaking safe) so the engagement, gateway and frontend slices can build against it.

initiative: q2-review-integrity

## What changes
In `platform-core/packages/proto/platform/engagement/v1/engagement.proto`:
- `EngagementService` gains `rpc HideReview(HideReviewRequest) returns (HideReviewResponse)` under "Reviews & Ratings". The RPC comment states: admin-only (`Unauthenticated` for anonymous callers, `PermissionDenied` for authenticated non-admins), `NotFound` for an unknown id, `InvalidArgument` for an empty review_id or a reason that is empty or over 500 chars after trim, idempotent (hiding an already hidden review returns OK), `Unavailable` when the audit sink is unavailable (the review stays visible), `Internal` if the hide fails after the audit write. The 1-500 reason limit is enforced in engagement-review-integrity.
- New messages `HideReviewRequest { string review_id = 1; string reason = 2; }` (reason required, 1-500 chars after trim) and empty `HideReviewResponse {}`.
- Comment-only updates (wire unchanged): `CreateReviewRequest.order_id` ("required: a COMPLETED order of the caller containing listing_id"; empty is `InvalidArgument`, ineligible is `FailedPrecondition`, duplicate is `AlreadyExists`, verifier down is `Unavailable`); `Review.verified_purchase` ("true for every review created after q2; false = legacy, excluded from ratings"); `review_count` in `GetListingRatingSummaryResponse` and `GetShopRatingSummaryResponse` ("verified, non-hidden reviews").
- The proto file is copied byte-identical into `team-engagement/proto`, `team-gateway/proto`, `team-frontend/proto` (same practice as Q1). Consumers regenerate and never edit it.

Out of scope: any behaviour (engagement slice), gateway policy (engagement slice), UI (frontend slice), unhide, a `ReviewHidden` event.

## NFR
- Additive only: `buf breaking` against HEAD passes; clients that ignore the new RPC keep working; no field number or type changes.
- Contract-level target for implementers: HideReview p95 < 300 ms with audit up (non-gating, initiative NFR).
- No new availability or limit requirements in this slice, other than the 500-char reason limit documented in the contract.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `HideReviewRequest.reason` | free text written by staff (pii-light) | moderation accountability | legitimate interest / legal obligation (Decree 85/2021 complaint handling) | lifetime of the review; copied to the audit event, which follows the team-audit policy |
| `HideReviewRequest.review_id` | non-personal identifier | target of the hide | contract | transient on the wire |
| admin actor id (not on the wire; taken from the Principal) | personal (staff id) | accountability | legitimate interest | as above |

No new buyer PII. Comment-only changes add no data.

## Threat model
- Assets: review integrity, listing and shop star ratings, the moderation audit trail.
- Abuse: a non-admin hides competitors' reviews; hiding without an audit record; oversized or injected reason text; probing review ids.
- Authz rule: HideReview requires an admin Principal, enforced at the gateway and again in engagement (documented in the RPC comment; enforcement lives in engagement-review-integrity). The contract does not let the caller supply the actor id, so the actor cannot be spoofed.
