## ADDED Requirements

### Requirement: ConsentState message
The analytics proto SHALL define `message ConsentState` with `bool analytics = 1`, `bool marketing = 2`, `bool personalization = 3` and `int32 version = 4`, where `version = 0` means no consent cookie was present and SHALL be read as all three flags denied.

#### Scenario: ConsentState fields present with stable numbers
- **WHEN** the descriptor of `platform.analytics.v1.ConsentState` is inspected
- **THEN** it has exactly 4 fields with names, types and numbers `analytics=1 bool`, `marketing=2 bool`, `personalization=3 bool`, `version=4 int32`

#### Scenario: Missing consent reads as denied
- **WHEN** a `TrackingEvent` is decoded with no `consent` set
- **THEN** reading `consent.analytics`, `consent.marketing`, `consent.personalization` and `consent.version` yields false, false, false and 0

#### Scenario: Version zero with granted flags is not trusted
- **WHEN** the comment on `ConsentState.version` in analytics.proto is read
- **THEN** it contains the text `version = 0 is treated as all denied regardless of flags`

#### Scenario: Unknown or negative version is denied
- **WHEN** the comment on `ConsentState.version` in analytics.proto is read
- **THEN** it contains the text `negative or unknown version (known: 1) is treated as all denied`, so a consumer reading `version = -1` or `version = 2` treats analytics, marketing and personalization as denied

### Requirement: TrackingEvent carries GA client id and consent
`TrackingEvent` SHALL add `string ga_client_id = 25` and `ConsentState consent = 26` without altering any existing field, and the proto comments SHALL state that `consent` is stamped by the edge from the `agora_consent` cookie and never from the request body, and that `ga_client_id` is empty without analytics consent.

#### Scenario: New fields round trip
- **WHEN** a `TrackingEvent` with `ga_client_id="123.456"` and `consent={analytics:true, marketing:false, personalization:true, version:1}` is serialised and parsed
- **THEN** the parsed event equals the original in all fields

#### Scenario: Old payload stays readable
- **WHEN** bytes serialised by the previous `TrackingEvent` (fields 1-24 only) are parsed with the new schema
- **THEN** parsing succeeds, `ga_client_id` is empty, `consent` is unset and fields 1-24 are unchanged

#### Scenario: New payload readable by old schema
- **WHEN** an event with fields 25 and 26 set is parsed with the previous schema
- **THEN** parsing succeeds and fields 1-24 are unchanged

#### Scenario: Field numbers do not collide
- **WHEN** `buf breaking --against main` runs on the proto package
- **THEN** it exits 0 and `buf lint` exits 0

#### Scenario: Field 25 and 26 are frozen
- **WHEN** a scratch copy of analytics.proto changes the name or type of field 25 or field 26 and `buf breaking --against main` runs on it
- **THEN** `buf breaking` exits non-zero

#### Scenario: Client id comment states format and blanking
- **WHEN** the comment on `TrackingEvent.ga_client_id` is read
- **THEN** it contains `without the GA1.1. prefix` and `blanked by consumers when consent.analytics is false`

#### Scenario: Client id without analytics consent is blanked, event kept
- **WHEN** the comment on `TrackingEvent.ga_client_id` is read
- **THEN** it contains `event is kept`, documenting that consumers blank the id and do not drop the event

### Requirement: Channel funnel query
`AnalyticsQueryService` SHALL define admin-only `rpc GetChannelFunnel(GetChannelFunnelRequest) returns (GetChannelFunnelResponse)`. The request SHALL carry `from`, `to` (Timestamp) and optional `string seller_id`; the response SHALL carry `repeated ChannelFunnelRow rows` where each row has `source`, `medium`, `campaign` and funnel counts `impressions`, `views`, `adds`, `begin_checkouts`, `purchases` (int64). The RPC comment SHALL state scope `admin`; enforcement is by the serving service.

#### Scenario: Channel funnel messages present
- **WHEN** the descriptor of `platform.analytics.v1` is inspected
- **THEN** `GetChannelFunnelRequest` has `seller_id`, `from`, `to`, `ChannelFunnelRow` has `source`, `medium`, `campaign`, `impressions`, `views`, `adds`, `begin_checkouts`, `purchases` with int64 counts, and the service method exists

#### Scenario: Scope documented
- **WHEN** the comment on `GetChannelFunnel` is read
- **THEN** it contains `Admin-only (scope "admin")`

#### Scenario: Non-admin is rejected
- **WHEN** a caller without scope `admin` (a seller or buyer, even with its own `seller_id`) calls `GetChannelFunnel` on a service implementing it
- **THEN** the call returns PERMISSION_DENIED and no rows

#### Scenario: Empty seller means all sellers
- **WHEN** an admin calls `GetChannelFunnel` with empty `seller_id`
- **THEN** rows aggregate over every seller; with `seller_id` set they cover only that seller

#### Scenario: Invalid date range
- **WHEN** an admin calls `GetChannelFunnel` with `from` after `to`
- **THEN** the call returns INVALID_ARGUMENT

#### Scenario: Empty range returns no rows
- **WHEN** an admin calls `GetChannelFunnel` over a range with no events
- **THEN** the response has zero rows and no error

#### Scenario: Buf checks pass with new RPC
- **WHEN** `buf lint` and `buf breaking --against main` run
- **THEN** both exit 0 (the RPC and messages are additive)
