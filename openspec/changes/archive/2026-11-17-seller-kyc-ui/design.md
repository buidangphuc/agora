## Context

Contract names come from slice trust-legal-contract. Seller app already has per-request server-only gateway modules (see `src/lib/gateway/promotion.ts`) and Server Actions returning `ActionResult`.

## Decisions

- Browser-direct upload: Server Action `createKycUploadUrlAction(contentType, sizeBytes)` returns `{ uploadUrl, docRef }`; a client component does
  `fetch(uploadUrl, { method: "PUT", headers: { "Content-Type": type }, body: file })` (length bound by the signature, so the browser-set
  Content-Length must equal `size_bytes`). Sequential uploads, so a failure leaves no half-state beyond orphaned objects, which the bucket
  lifecycle in verification-kyc-documents owns.
- State kept per attempt: `File[]` and issued `docRef`s live in component state; a retry after failure reuses refs already uploaded
  (their PUT succeeded) and re-requests only failed ones.
- Status text and reason-code texts live in one map in `src/features/seller/verification/status.ts`; unknown code gives the generic text.
- Payout error mapping in the Server Action layer: gateway error code `FAILED_PRECONDITION` maps to the verification message, `UNAVAILABLE`
  to the retry message; messages are decided by code, not by parsing server text.
- Wallet notice reads status with one extra `GetVerificationStatus`; failure there hides the notice (the server gate stays authoritative).
- Buyer removal is deletion, not a redirect: KYC exists only in the seller app (initiative decision).
- Shop badge is fetched in the shop page `Promise.all`, `catch` returns `false`.
