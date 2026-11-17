## Context

Slice frontend-legal-pages, owner team-frontend (also touches team-frontend-seller). Depends on identity-legal-consent for runtime behaviour; contract names
from trust-legal-contract. All gateway calls go through server actions and `makeClients(token)` (Rule 1: gateway is the only outbound path).

## Decisions

- **Routes.** One route group `src/app/(shop)/(legal)/[slug]/page.tsx` with `generateStaticParams` over the seven slugs and `dynamicParams = false`, so any
  other single-segment path keeps its existing behaviour (404) and static routes win. Pages inherit `ConsumerShell` (footer, header).
- **Content.** `content/legal/<slug>.md`: a small front-matter block (`title`, `version`, `updated`) plus a body. Parsed by a 30-line reader in
  `src/features/legal/` using `node:fs` at build time, rendered as headings and paragraphs (text nodes). No markdown dependency is added (initiative: no
  new dependency in this slice). The bodies are the placeholder line until the legal owner supplies text.
- **Versions.** Front-matter `version` of `dieu-khoan-su-dung` and `chinh-sach-bao-mat` must equal `NEXT_PUBLIC_LEGAL_TERMS_VERSION` and
  `NEXT_PUBLIC_PRIVACY_POLICY_VERSION` (a unit test enforces equality so the page the user reads is the version recorded). Defaults: `2026-11` for
  both, which must equal the first compose entry of identity `LEGAL_TERMS_VERSIONS` / `PRIVACY_POLICY_VERSIONS`. The Dockerfile gets both as build args
  (the seller app's equivalent is documented as not a build arg; this slice does it properly for the buyer app).
- **Footer operator block.** `src/features/legal/operator.ts` exports four placeholder fields, each "[DO BỘ PHẬN PHÁP CHẾ CUNG CẤP]". The footer renders
  them; the MoIT notice area is a placeholder box, no logo asset is invented.
- **Sign-up.** `RegisterForm` adds two checkboxes (names `acceptTerms`, `acceptPrivacy`) and `registerAction` sends the build-time versions. The action
  refuses (returns a field error, makes no `Register` call) when either checkbox value is absent, so a forged post cannot skip the UI check. Mapping:
  `INVALID_ARGUMENT` naming `terms_version` or `privacy_version` shows "Điều khoản đã được cập nhật. Vui lòng tải lại trang và đồng ý lại."
- **Re-acceptance.** A client component `LegalReacceptance` mounted in `ConsumerShell` calls the server action `getLegalAcceptance()` once per tab session
  (sessionStorage `agora_legal_ok` = `<terms>|<privacy>` after a satisfied check). The action returns `{signedIn:false}` without any gateway call when there
  is no session; otherwise it pages `ListMyConsents` (newest first, page size 100, at most 5 pages) until both a TERMS and a PRIVACY_POLICY row are found.
  Prompt shown when either is missing or `version` differs from current. The dialog is blocking on every route except the legal pages themselves (so the
  user can read before agreeing) and offers "Đồng ý" and "Đăng xuất". A failed check shows nothing.
- **Banner ledger.** `ConsentBanner.commit` stays synchronous for the cookie (existing spec `consent` unchanged), then fires the server action
  `recordBannerConsent(prev, next)`. The action no-ops without a session. Changed purposes = those whose boolean differs from the cookie read just before
  the write; with no prior cookie (version 0) all three purposes are sent. Version string `"1"` (cookie format version). A failure is swallowed in the UI.
- **Seller.** `acceptSellerTerms` (server action in `onboarding.ts`) calls `PrivacyService.RecordConsent` with SELLER_TERMS after `BecomeSeller` succeeds;
  a failure there does not undo seller status (the acceptance is already stored on the user row by `BecomeSeller`), it is logged as a count only.
  The link uses `buyerAppUrl("/dieu-khoan-nguoi-ban")` with `target=_blank`.

## Risks

- Version drift between `NEXT_PUBLIC_*` and identity config makes every sign-up fail `INVALID_ARGUMENT`; mitigated by the explicit error text, the README
  env table and the compose values set together. Same trap exists today for seller terms.
- The blocking dialog can hide content behind it; it is limited to a signed-in user with stale acceptance.

## Migration

No data migration. Existing users are prompted on next page load after identity's rollout because they have no TERMS/PRIVACY rows.
