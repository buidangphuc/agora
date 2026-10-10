## Why

team-ai's REST routes `/api/v1/ai/tags/*` (`classify`, `classify-sku-hierarchy`, `explore`, `promote`, `GET /tags`) have no
authentication. `promote` (and `explore`) mutate the shared taxonomy that team-search's indexer reads through
`ClassifyTags`, so any caller that can reach the port can rewrite the filter facets of the whole catalogue. The gRPC
twin `ClassifyTags` is already gated (scope `ai.classify` and a service principal); the REST surface is not.

## What Changes

- **team-ai**: every tag route requires a bearer token verified like team-ai's other REST routes (`AUTH_BEARER_TOKEN`,
  constant-time compare; no JWT verification, ADR-0003/0006 keep that at the gateway).
  - Read routes (`classify`, `classify-sku-hierarchy`, `GET /tags`): principal with scope `ai.classify` or `admin`.
  - Mutating routes (`explore`, `promote`): principal with scope `admin`.
  - New optional setting `AUTH_ADMIN_BEARER_TOKEN` (+ `AUTH_ADMIN_SUBJECT`): a second token that resolves to a service
    principal with scopes `admin` and `ai.classify`. The existing `AUTH_BEARER_TOKEN` keeps resolving to the
    `AUTH_ROLES` scopes (set `AUTH_ROLES=ai.classify` for a read-only service caller). Unset admin token = no admin
    access at all (fail closed); outside dev/local/test it must be strong and different from `AUTH_BEARER_TOKEN`.
  - Anonymous, a wrong token and a user's JWT are refused `401` (team-ai holds no JWKS, so a JWT is just a wrong
    token); a valid token without the needed scope is `403`.
- **platform-e2e**: the `tax_` steps authenticate; new negative scenarios (anonymous, buyer JWT, read-only service).
- Not exposed through the gateway (see design D1).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tag-taxonomy-enrichment`: ADDED requirement "Tag routes require an authenticated principal".

## Impact

- Code: `team-ai/app/modules/platform/identity/auth.py`, `app/api/v1/ai/{router,dependencies}.py`, `app/core/config/platform.py` and `__init__.py`, `.env.example`, README, existing tag API tests.
- Compose (integrator): team-ai env `AUTH_BEARER_TOKEN`, `AUTH_ROLES`, `AUTH_ADMIN_BEARER_TOKEN` (design.md).
- E2E: `tax_steps.py`, `tax_support.py`, `ai/tag_classifier_taxonomy.feature`, `team-ai/FEATURES.yaml`.

## Non-goals

- No gateway route, no proto change, no new RPC, no JWT verification in team-ai.
- No per-user audit trail of promotions; no persistence (that is `tag-taxonomy-persistence`).
- The other HTTP AI routes (`/assistant`, `/magic-listing`, `/chat-copilot`) are untouched.
