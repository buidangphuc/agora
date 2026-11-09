# [W1-T4] frontend-seo

## Role
feature-run worker (change `openspec/changes/frontend-seo`)

## Objective
Implement OpenSpec change `frontend-seo` test-first per its tasks.md (SEO-1..SEO-45); context in plans/discovery-seo/00-spec.md and the change's design.md.
- `src/lib/seo.ts`, typed root metadata, `generateMetadata` for home, `/listing/[id]`, `/shop/[id]` (request-cached `loadStorefront`) and `/search` (metadata export only); noindex layouts for private segments and `(checkout)`; `robots.ts`, `sitemap.ts` (+ `lib/gateway/sitemap.ts`, `SearchHit.seller_id`), `icon.svg`; brand strings; seller/admin root robots + title templates and suffix removal; `NEXT_PUBLIC_SITE_URL` documented.
- depends_on: discovery-seo-contract (merged; sitemap needs `SearchHit.seller_id`)
- base: `feat/discovery-seo` after wave 0 merged
- hot files: `src/app/(shop)/search/page.tsx` is shared with search-attribution (wave 2) and frontend-discovery-sort-rating (wave 4); touch only the metadata export.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/**
- team-frontend/src/lib/seo.ts
- team-frontend/src/lib/seo.test.ts
- team-frontend/src/lib/gateway/sitemap.ts
- team-frontend/src/lib/gateway/sitemap.test.ts
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/src/features/student/StudentRentalView.tsx
- team-frontend/src/features/student/StudentRentalView.test.tsx
- team-frontend/.env.example
- team-frontend/README.md
- team-frontend/FEATURES.yaml
- team-frontend-seller/src/app/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- team-frontend-admin/src/app/**
- team-frontend-admin/README.md
- team-frontend-admin/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/frontend/seo.feature
- platform-e2e/tests/e2e/step_definitions/seo_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seo.py

## Verify
- `cd team-frontend && npm run proto && npm run check && npm run build`
- `cd team-frontend-seller && npm run proto && npm run check`
- `cd team-frontend-admin && npm run proto && npm run check`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_seo.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; rebuild team-frontend, seller :3001 and admin :3002 first)
