# Proposal: GA4 consent contract on TrackingEvent

initiative: ga4-analytics

## Why
The GA4 initiative needs the first-party event stream to carry the buyer's consent choice and the GA client id, so recsys can drop personalisation-denied events, analytics can join GA4 sessions, and the purchase relay can map a buyer to a client id. This slice only defines the wire contract; edge stamping, banner, relay and filters live in later slices.

## What changes
- `platform-core/packages/proto/platform/analytics/v1/analytics.proto`: add to `TrackingEvent` the fields `string ga_client_id = 25` and `ConsentState consent = 26`, and a new `message ConsentState { bool analytics = 1; bool marketing = 2; bool personalization = 3; int32 version = 4; }`.
- Add admin-only RPC `AnalyticsQueryService.GetChannelFunnel(GetChannelFunnelRequest) returns (GetChannelFunnelResponse)` (the existing service in analytics.proto is named `AnalyticsQueryService`; the initiative's "AnalyticsService" is read as this one). Request: `seller_id` (optional, empty = platform-wide), `from`, `to` (Timestamps, mirroring `GetSellerFunnelRequest`). Response: `repeated ChannelFunnelRow rows`, each `{string source=1; string medium=2; string campaign=3; int64 impressions=4; int64 views=5; int64 adds=6; int64 begin_checkouts=7; int64 purchases=8;}` (same step names as `GetSellerFunnelResponse`). Scope `admin` is documented in the proto comment and enforced by the service (implemented in add-ga4-export-import).
- Purely additive: no field renumbered, removed or retyped; existing consumers keep working unchanged.
- Semantics are documented in proto comments: `consent` is stamped by the edge from the `agora_consent` cookie, never from the request body; `version = 0` means no cookie and is treated as all denied; `ga_client_id` is empty without analytics consent and holds the client-id part only (e.g. `123456.789012`, without the `GA1.1.` prefix).
- Vendored copies in team-gateway, team-analytics, team-frontend and platform-recsys-facing repos are refreshed by the consuming slices, not here.

## NFR
- Compatibility: `buf lint` and `buf breaking --against main` pass; old readers ignore the new fields, new readers see defaults for old messages (empty string, unset `consent`).
- Size: the added fields add at most about 60 bytes per event; no change to the 100-event batch or 64 KiB body limits.
- No runtime service, availability or latency impact (schema only); `GetChannelFunnel` performance targets belong to add-ga4-export-import.

## Data classification
| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| `ga_client_id` | pseudonymous identifier (client part of the `_ga` cookie) | attribution join with GA4 export, purchase relay | analytics consent (`a=1`); empty otherwise | 13 months (same as `tracking_events`) |
| `consent` (ConsentState) | consent record, not sensitive by itself | proof of consent; personalisation filter in recsys | necessary / legal obligation (Decree 13/2023 evidence) | 13 months (same as `tracking_events`), kept with each event |

No email, phone or name is added. Setting `ga_client_id` while `consent.analytics` is false is a contract violation; consumers blank it.

## Threat model
- Assets: consent proof integrity; pseudonymous client id.
- Abuse: a client forges `consent.personalization=true` in the body to force personalisation, or sends `ga_client_id` without consent. Mitigation in contract: `consent` is edge-stamped only and body values are ignored (enforced by add-consent-mode in the gateway); consumers BLANK `ga_client_id` when `consent.analytics` is false (or consent is missing, version 0, or unknown) and KEEP the event (it is not dropped).
- Authz: `GetChannelFunnel` is admin-only (scope `admin`); the contract documents the scope, the service enforces it. It returns aggregates by source/medium/campaign only, no per-user data; non-admin callers (including sellers) are rejected, even for their own `seller_id`.
