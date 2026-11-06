# [W2-T2] frontend-referral-invite

## Role
feature-run worker (change `openspec/changes/frontend-referral-invite`)

## Objective
Implement OpenSpec change `frontend-referral-invite` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md and the change's design.md.
- Invite link share, /s/[code] route handler with agora_ref cookie, register passes referral_code, referral page shows xu totals and reward statuses, redeem rule text.
- depends_on: referral-attribution-reward (runtime only; code depends only on the merged contract)

## Write-set (EXCLUSIVE)
- team-frontend/src/components/ui/format.ts
- team-frontend/src/components/ui/format.test.ts
- team-frontend/src/lib/gateway/referral.ts
- team-frontend/src/lib/gateway/referral.test.ts
- team-frontend/src/features/account/referral/**
- team-frontend/src/app/(shop)/account/referral/**
- team-frontend/src/app/(shop)/s/[code]/**
- team-frontend/src/app/(shop)/register/**
- team-frontend/src/features/auth/actions.ts
- team-frontend/src/features/auth/actions.test.ts
- team-frontend/src/features/auth/RegisterForm.tsx
- team-frontend/src/features/auth/forms.test.tsx
- team-frontend/FEATURES.yaml
- team-frontend/README.md
- platform-e2e/tests/e2e/features/frontend/referral_invite.feature
- platform-e2e/tests/e2e/step_definitions/referral_invite_steps.py
- platform-e2e/tests/e2e/step_definitions/test_referral_invite.py

## Verify
- `cd team-frontend && npm run proto && npm run check && npm run build`
- e2e: deferred (local stack busy)
