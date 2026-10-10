## ADDED Requirements

### Requirement: Tag routes require an authenticated principal

Every tag route of team-ai (`POST /api/v1/ai/tags/classify`, `/classify-sku-hierarchy`, `/explore`, `/promote` and `GET /api/v1/ai/tags`) SHALL require a bearer token that team-ai verifies itself. The read routes (`classify`, `classify-sku-hierarchy`, list) SHALL accept a principal holding scope `ai.classify` or `admin`; the mutating routes (`explore`, `promote`) SHALL accept only a principal holding scope `admin`. A request without a valid token SHALL be refused with 401 and a valid token that lacks the scope with 403, and a refused request SHALL NOT change the taxonomy. A user's JWT SHALL NOT be accepted (only the gateway verifies JWTs).

#### Scenario: Anonymous tag requests are refused

- **WHEN** a caller without an `Authorization` header calls each of the five tag routes, the mutating ones with a valid body
- **THEN** every call answers 401 and the taxonomy is unchanged

#### Scenario: A buyer's token is refused

- **WHEN** a signed-in buyer presents the session JWT the gateway accepts (or any other wrong bearer token) to a read route and to `promote`
- **THEN** both answer 401 and the taxonomy is unchanged

#### Scenario: A read-only service principal cannot mutate the taxonomy

- **WHEN** a service principal holding only `ai.classify` classifies and lists tags, then calls `explore` and `promote`
- **THEN** classify and list answer 200, `explore` and `promote` answer 403, and no candidate is registered or promoted

#### Scenario: An admin principal explores and promotes

- **WHEN** an admin principal explores a batch and promotes the discovered candidate
- **THEN** both answer 200 and the tag is promoted and canonical

#### Scenario: Admin access is off unless configured and strong

- **WHEN** `AUTH_ADMIN_BEARER_TOKEN` is unset, or outside dev/local/test is weak or equal to `AUTH_BEARER_TOKEN`
- **THEN** no principal holds `admin` on the REST routes, and startup refuses the weak or equal configuration
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_routes_authz.py › test_admin_token_unset_grants_no_admin, test_weak_or_shared_admin_token_is_refused_outside_local. Not verifiable end to end: the stack runs with the local environment and a configured admin token (design.md).
