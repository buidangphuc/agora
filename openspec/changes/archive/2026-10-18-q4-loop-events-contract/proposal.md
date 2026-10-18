## Why
The Q4 loop-instrumentation initiative must record sign up, login, follow shop, referral redeem, daily check-in, alert subscribe and AI surface interactions as first-party tracking events. `EventType` has no value for them, so the gateway cannot accept them and the warehouse would store them as `unspecified`. This slice adds the additive enum values and the proto copies the consumers need. Behaviour lives in the later slices.

initiative: q4-loop-instrumentation

## What changes
In `platform-core/packages/proto/platform/analytics/v1/analytics.proto`, enum `EventType` gains seven values, with comments that document the property keys:
- `EVENT_TYPE_SIGN_UP = 15` (properties: `method`), `EVENT_TYPE_LOGIN = 16` (`method`), `EVENT_TYPE_FOLLOW_SELLER = 17` (`seller_id`), `EVENT_TYPE_REFERRAL_REDEEM = 18` (no properties; the code is never sent), `EVENT_TYPE_DAILY_CHECK_IN = 19` (`streak`, optional), `EVENT_TYPE_ALERT_SUBSCRIBE = 20` (`alert_type`; `listing_id` set), `EVENT_TYPE_AI_INTERACTION = 21` (properties: exactly `surface`, `action`; allowlisted, no free text).
- Values 0-14 keep name and number. No message changes.
- The proto is copied byte-identical into `proto/platform/analytics/v1/` of team-gateway, team-analytics and team-frontend (existing copies, updated). Consumers regenerate in their own slices and never edit the copy.
- A committed Go test `platform-core/packages/proto/platform/analytics/v1/loopevents_compat_test.go` (same approach as `consent_compat_test.go` and the Q3 `auditquery_roundtrip_test.go`): enum numbers, round-trip, old-bytes parse, comment assertions.

Out of scope: gateway allowlist enforcement (gateway-loop-event-intake), warehouse naming (analytics-loop-event-storage), UI emission, `WireTrackBeacon` (unchanged), `ai.proto`, `request_id`/`model_version` on AI events.

## NFR
- Additive only: `buf breaking` against HEAD passes; consumers that do not know values 15-21 keep working (proto3 open enum preserves unknown values on the wire).
- No new service, port, RPC or dependency. No runtime latency impact (enum values only).
- Compat: a TrackingEvent written with a new value and read by an old consumer parses without error and exposes the numeric value.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `EventType` values 15-21 | non-personal (enum labels) | classify behavioural events | contract | n/a |
| `properties.method`, `alert_type`, `surface`, `action`, `streak` (documented keys) | non-personal, no free text | describe the activation/loop/AI action | the existing beacon path's consent-snapshot rule; no new basis introduced | follow `analytics.events` / warehouse policy (unchanged) |
| `properties.seller_id`, `listing_id` | business identifiers | attribute the action to a shop/listing | same | same |
| referral code, prompts, AI replies, titles, review text | MUST NOT be sent | documented as forbidden in the comments | n/a | n/a |

No new personal data is defined; the principal id and anonymous/session ids already live in the existing envelope.

## Threat model
- Assets: the warehouse event vocabulary; the guarantee that AI events carry no free text and referral codes never leave the browser.
- Abuse: a producer overloads `properties` with free text or a referral code (mitigated by the contract comments, gateway allowlist for `ai_interaction` in gateway-loop-event-intake, no referral property defined); an enum renumbering breaks stored events (mitigated by `buf breaking` and descriptor assertions); a consumer copy drifts and decodes values as unknown (mitigated by the `cmp` check).
- Authz rule: no new RPC and no authz change. Collector access and consent handling are unchanged.
