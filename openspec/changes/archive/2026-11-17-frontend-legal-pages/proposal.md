## Why

A visitor today cannot read any legal document: no legal route exists in any app, the footer "Chính Sách Trả Hàng" link goes to
`/account/orders` (`team-frontend/src/components/shell/ConsumerShell.tsx`), the seller onboarding "terms" link goes to the buyer home page
(`team-frontend-seller/src/features/seller/SellerOnboarding.tsx`), the sign-up form (`team-frontend/src/features/auth/RegisterForm.tsx`) asks for no
acceptance, and the consent banner (`team-frontend/src/features/consent/ConsentBanner.tsx`) is English and writes only a cookie. Backlog Q24 needs the
user-facing half of server-side proof of consent. The server half (`Register` version fields, consent ledger) is slice `identity-legal-consent`.

initiative: trust-legal

## What changes

- team-frontend: seven static legal pages (Server Components) under Vietnamese slugs `dieu-khoan-su-dung`, `chinh-sach-bao-mat`, `quy-che-hoat-dong`,
  `chinh-sach-doi-tra`, `hang-cam-kinh-doanh`, `giai-quyet-khieu-nai`, `dieu-khoan-nguoi-ban`, rendered from `content/legal/<slug>.md` with a version
  header. Every body is the placeholder "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP"; this slice ships structure and versions only, never legal text.
- Footer: a legal-links column to all seven pages, the "Chính Sách Trả Hàng & Hoàn Tiền (RMA)" link repointed to `/chinh-sach-doi-tra`, and the operator
  block (company name, business registration number, address, Ministry of Industry and Trade registration notice) as placeholders.
- Sign-up: two required checkboxes (Terms, Privacy Policy, each linking to its page) in `RegisterForm`; `registerAction` sends `terms_version` and
  `privacy_version` to `AuthService.Register`. Versions come from `NEXT_PUBLIC_LEGAL_TERMS_VERSION` / `NEXT_PUBLIC_PRIVACY_POLICY_VERSION`
  (build-time, same pattern as `NEXT_PUBLIC_SELLER_TERMS_VERSION`), which must equal the first entry of identity `LEGAL_TERMS_VERSIONS` /
  `PRIVACY_POLICY_VERSIONS`.
- Re-acceptance prompt: a signed-in user whose latest TERMS or PRIVACY_POLICY consent (from `PrivacyService.ListMyConsents`) is missing or not the
  current version sees a blocking Vietnamese dialog; "Đồng ý" calls `PrivacyService.RecordConsent` (source `account`).
- Consent banner: Vietnamese labels. For a signed-in user, saving a choice also calls `RecordConsent` with one record per purpose whose value changed
  (source `banner`); the cookie remains the authority for the gateway and for anonymous visitors, who make no `PrivacyService` call.
- team-frontend-seller: the onboarding terms text links to the seller terms page on the buyer app (`/dieu-khoan-nguoi-ban`); after `BecomeSeller` succeeds
  a `SELLER_TERMS` consent is recorded (source `seller_onboarding`).
- team-frontend: sitemap lists the seven pages; gateway client factory gains the `PrivacyService` client.

Contract consumed (from slices trust-legal-contract and identity-legal-consent, not redefined here): `RegisterRequest.terms_version=5`,
`privacy_version=6`, `PrivacyService.RecordConsent`, `PrivacyService.ListMyConsents`, `ConsentPurpose`, `ConsentRecord`.

## Capabilities

### New Capabilities
- `frontend-legal-pages`: legal pages, footer, sign-up acceptance, re-acceptance prompt, Vietnamese consent banner with ledger write, seller terms link.

### Modified Capabilities
none

## Impact

- team-frontend: `content/legal/*.md`, new route group for the legal pages, `src/features/legal/` (reader, operator placeholders, prompt),
  `src/components/shell/ConsumerShell.tsx` (footer; hot file shared with frontend-account-privacy and seller-kyc-ui, serialised by initiative-build),
  `src/features/auth/{RegisterForm.tsx,actions.ts}`, `src/features/consent/ConsentBanner.tsx`, `src/lib/gateway/client.ts`, `src/app/sitemap.ts`,
  `Dockerfile` build args, `FEATURES.yaml` (by e2e-author).
- team-frontend-seller: `src/features/seller/{SellerOnboarding.tsx,onboarding.ts}`, README env table.
- docker-compose: `NEXT_PUBLIC_*` version values must match identity `LEGAL_TERMS_VERSIONS` / `PRIVACY_POLICY_VERSIONS` (set by identity-legal-consent).
- Existing frontend tests that register or render the footer/banner change; e2e register helpers are owned by identity-legal-consent.

## Non-goals

Legal text itself. `/account/privacy` (consent history, export, deletion: slice frontend-account-privacy). Server-side validation or storage (identity-legal-consent).
Consent records for anonymous visitors and backfill when they later sign in. A banner in the seller app. Moving the buyer `/account/verification` route (seller-kyc-ui).

## Non-functional requirements

- Legal pages are statically rendered at build time: no gateway call per request, TTFB p95 <= 200 ms locally, no client JavaScript required to read them.
- The re-acceptance check adds at most 1 `ListMyConsents` call per browser tab session for a signed-in user and 0 for an anonymous visitor.
- Fail behaviour: if `ListMyConsents` fails the prompt is not shown and browsing continues (the server remains authoritative at `Register`); if the banner's
  `RecordConsent` fails the cookie choice still applies. Sign-up fails closed: no `Register` call without both acceptances.
- UI copy is Vietnamese; every control keyboard-operable with an accessible name; the dialogs trap focus.
- Version strings are at most 32 characters (the `ConsentRecord.version` limit).

## Data classification

| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| Terms / Privacy acceptance (version, time) sent at `Register` | personal (consent proof linked to the new account) | prove acceptance | legal obligation, Decree 13/2023 Art. 9-11 (as cited by the initiative, unverified, NEEDS_HUMAN) | stored by team-identity; this slice stores nothing |
| Banner choices (analytics, marketing, personalization) sent via `RecordConsent` | personal (preference tied to the user id) | prove consent decisions | consent itself | stored by team-identity; cookie `agora_consent` unchanged (180 days) |
| `SELLER_TERMS` acceptance | personal | prove seller terms acceptance | contract | stored by team-identity |
| Operator block and legal bodies | public, non-personal | disclosure duty for e-commerce platforms | legal obligation | static content in the repo |

The frontend persists no consent data of its own beyond the existing cookie and a non-identifying sessionStorage flag (`agora_legal_ok`: version pair
string only). It never logs usernames, user ids or consent payloads.

## Threat model

- Assets: acceptance proof integrity, the consent cookie, session tokens, legal page integrity.
- Abuse cases: a crafted form post omits or forges the acceptance fields (server action re-checks and identity rejects stale versions); a tampered
  hidden version field (identity rejects any non-current version, the user sees an error not a silent success); an anonymous visitor triggers consent
  writes (action returns without any `PrivacyService` call when no session exists); one user's consent shown or written for another (the user id is never
  sent, identity takes it from the principal); legal page injection through content (content files are repo-owned, rendered as text nodes, no raw HTML);
  open redirect via the prompt (the "Đăng xuất" path uses the existing logout action).
- Authz rule: legal pages are public (anonymous allowed); `RecordConsent` and `ListMyConsents` are called only with the caller's own bearer from a
  server action, never from the browser to the gateway with a token in JS.
