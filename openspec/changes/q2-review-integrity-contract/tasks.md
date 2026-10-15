Paths relative to the repo root. E2E track: none (pure contract; behaviour is e2e-tested in engagement-review-integrity and frontend-review-integrity).

## 1. Code track
Write-set: `platform-core/packages/proto/platform/engagement/v1/engagement.proto`
- [ ] 1.1 Add `rpc HideReview`, `HideReviewRequest { review_id = 1; reason = 2; }` and empty `HideReviewResponse` under "Reviews & Ratings" (covers: Service and messages exist in every generated language, Request field numbers are fixed, Lint passes).
- [ ] 1.2 Write the RPC doc comment: admin-only (`Unauthenticated` anonymous, `PermissionDenied` authenticated non-admin), `NotFound`, `InvalidArgument` (reason 1-500 chars after trim), idempotent, `Unavailable` (review stays visible), `Internal` after audit write (covers: Error and authz contract is documented).
- [ ] 1.3 Update comments only on `CreateReviewRequest.order_id`, `Review.verified_purchase`, and `review_count` of both summaries, using the exact text in the spec (covers: Rule comments are present, Existing messages keep their wire shape).
- [ ] 1.4 Run `make -C platform-core check` (lint + breaking) (covers: Change is backward compatible, Lint passes).

## 2. Generation, verification and copies
Write-set: `platform-core/packages/proto/platform/engagement/v1/hidereview_roundtrip_test.go`, generated output per the repo's normal flow, `team-engagement/proto/platform/engagement/v1/engagement.proto`, `team-gateway/proto/platform/engagement/v1/engagement.proto`, `team-frontend/proto/platform/engagement/v1/engagement.proto`
- [ ] 2.1 Run `buf generate`; add the committed Go test `hidereview_roundtrip_test.go`: field-number assertions for `HideReviewRequest`, empty `HideReviewResponse`, 500-char and empty reason round-trip, unchanged numbers for `order_id`, `verified_purchase`, `review_count`, `HideReviewRequest` having exactly two fields, and an assertion that the proto text contains the four rule comments and the HideReview error codes (covers: Request field numbers are fixed, Round-trip of a boundary reason, Existing messages keep their wire shape, Rule comments are present, Anonymous and non-admin callers are documented with distinct codes).
- [ ] 2.2 Confirm Go, Python, JVM and TS generated outputs expose the new symbols (covers: Service and messages exist in every generated language).
- [ ] 2.3 Copy the proto byte-identical into the three consumer `proto/` trees, run `cmp` against the source and `buf lint` in each consumer proto dir (covers: Copies match the source, Consumer copies lint and generate).

## 3. E2E track
- [ ] 3.1 None: no user-facing behaviour in this slice.
