# Specification: Edge consent stamping on tracking events

## ADDED Requirements

### Requirement: The edge stamps consent from the cookie and never from the body
`POST /api/track` SHALL parse the `agora_consent` cookie of the request and SHALL set `TrackingEvent.consent` on every published event to a `ConsentState{analytics, marketing, personalization, version}` derived only from that cookie; SHALL NOT read consent from the beacon body or any other header. A missing, malformed (including missing or non-numeric `ts`, non-0/1 flag, oversized over 256 bytes), duplicate-keyed, or unknown-version cookie SHALL yield `version=0` and all flags false, and SHALL NOT cause the request to fail.

#### Scenario: Valid cookie is stamped on each event
- **WHEN** a batch of 3 beacons arrives with `Cookie: agora_consent=v=1&a=1&m=0&p=1&ts=1760000000`
- **THEN** the 3 published events each carry `consent` with analytics true, marketing false, personalization true, version 1

#### Scenario: Rejected buyer is flagged personalization-denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=v=1&a=0&m=0&p=0&ts=1760000000`
- **THEN** the response is 202, the event is published and carries `consent.personalization=false` and `consent.version=1`

#### Scenario: Body consent is ignored
- **WHEN** a beacon body contains `"consent":{"personalization":true}` and the cookie has `p=0`
- **THEN** the published event has `consent.personalization=false`

#### Scenario: No cookie stamps version 0
- **WHEN** a beacon arrives with no `agora_consent` cookie
- **THEN** the response is 202 and the event carries `consent.version=0` with analytics, marketing and personalization false

#### Scenario: Non-0/1 flag degrades to denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=v=1&a=1&m=1&p=7&ts=1760000000`
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Unknown version degrades to denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=v=9&a=1&m=1&p=1&ts=1760000000`
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Duplicate key degrades to denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=a=1&a=0&v=1&m=0&p=0&ts=1760000000`
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Non-numeric ts degrades to denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=v=1&a=1&m=1&p=1&ts=x`
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Missing ts degrades to denied
- **WHEN** a beacon arrives with `Cookie: agora_consent=v=1&a=1&m=1&p=1`
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Oversized cookie degrades to denied
- **WHEN** the `agora_consent` value is 257 bytes (otherwise valid)
- **THEN** the event is published with `consent.version=0` and all flags false and the response is 202

#### Scenario: Cookie of exactly 256 bytes is accepted
- **WHEN** the `agora_consent` value is a valid `v=1` cookie padded with an extra unknown key to exactly 256 bytes
- **THEN** the event carries the flags from the cookie with `consent.version=1`

#### Scenario: Consent header does not stand in for the cookie
- **WHEN** the request has header `X-Consent: v=1&a=1&m=1&p=1&ts=1` and no cookie
- **THEN** the event carries `consent.version=0` with all flags false

#### Scenario: Stamping adds no latency beyond budget
- **WHEN** the existing handler benchmark (N=1000 batches of 100 events) runs with a valid consent cookie
- **THEN** p99 stays under 50 ms

### Requirement: The edge copies the GA client id only under analytics consent
The edge SHALL copy the beacon's `gaClientId` into `TrackingEvent.ga_client_id` (client-id part only, no `GA1.1.` prefix) only WHEN the stamped consent has `analytics=true` and the value is at most 64 characters, and SHALL otherwise leave `ga_client_id` empty and still publish the event and return 202.

#### Scenario: Client id is copied under analytics consent
- **WHEN** a beacon with body `"gaClientId":"123.456"` arrives with `Cookie: agora_consent=v=1&a=1&m=0&p=0&ts=1760000000`
- **THEN** the published event has `ga_client_id="123.456"` and the response is 202
- **VERIFIED BY**: `team-gateway/internal/edge/consent_cookie_test.go`

#### Scenario: Client id is blank without analytics consent
- **WHEN** a beacon with body `"gaClientId":"123.456"` arrives with `a=0` in the cookie, or with no `agora_consent` cookie
- **THEN** the published event has `ga_client_id=""`, is still published and the response is 202
- **VERIFIED BY**: `team-gateway/internal/edge/consent_cookie_test.go`

#### Scenario: Over-long client id is blanked
- **WHEN** a beacon with a 65-character `gaClientId` arrives with `a=1` in the cookie (a 64-character value is copied unchanged)
- **THEN** the published event has `ga_client_id=""`, is still published and the response is 202
- **VERIFIED BY**: `team-gateway/internal/edge/consent_cookie_test.go`
