# [W1-T4] Listing Q&A UI (team-frontend)

## Role
SE

## Objective
Surface the already-built listing Q&A backend: on the listing detail page, buyers can ask a question, sellers can answer, and existing questions+answers are listed. Thin UI mirroring the reviews section. Typecheck + tests green.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/engagement.ts — add wrappers: askQuestion, answerQuestion, listQuestionsByListing (edit; append only)
- team-frontend/src/app/listing/[id]/** — mount a QASection on the detail page (edit; add the mount + any server action, minimal)
- team-frontend/src/features/listing/qa/** — new QASection + QuestionForm components + colocated tests (create)

## Read-only dependencies
- Generated stubs: src/generated/platform/engagement/* (has AskQuestion/AnswerQuestion/ListQuestionsByListing)
- Reference: src/features/review/ReviewSection.tsx (list + submit pattern) + src/lib/gateway/reviews.ts.
- listing/[id]/page.tsx already mounts ReviewSection — mount QASection the same way.

## Contracts you consume (already in gateway + backend)
```
AskQuestion(listing_id, question_text) -> ProductQuestion
AnswerQuestion(question_id, answer_text, is_shop_reply) -> ProductAnswer
ListQuestionsByListing(listing_id, page) -> [ProductQuestion{ answers[] }]
```

## Acceptance criteria
- [ ] Listing page lists questions with their answers (loading + empty handled simply)
- [ ] Authenticated user can ask a question; result appears; unauth users see a login prompt (mirror how ReviewSection gates)
- [ ] Answer path present (seller `is_shop_reply=true`)
- [ ] gateway wrappers server-only, per-request client from session (mirror reviews.ts)
- [ ] `npm run typecheck` clean; `npm run test -- src/features/listing` green (≥3 tests)

## Review (different agent)
SE rubric. Gateway module edit → contract-boundary-reviewer; require CLEAN.

## Verify
cd team-frontend && npm run typecheck && npm run test -- src/features/listing

## Out of scope
- Do NOT touch src/lib/gateway/orders.ts / payment.ts, account routes, or layout.tsx.
- Do NOT touch the ListingGrid or other features/listing/* files beyond the new qa/ subdir + the page mount.
- Do NOT edit team-gateway or backend repos. engagement.ts is YOURS exclusively this wave.
