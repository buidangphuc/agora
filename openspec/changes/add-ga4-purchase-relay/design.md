# Design

## Clarifications
### Round 1 — 2026-10-12
- Q1 [boundary] Mapping expires; Forged OrderPaidEvent from another principal is rejected; Purchase with missing transaction id is not mapped to GA4: these cannot be driven end to end on the local stack (30-day TTL; local Redpanda has no ACLs; the UI always creates an order id). → **Verified by the named unit test or manifest review (`**VERIFIED BY**` lines); FEATURES entries are `not-testable`** (default accepted)
- Q2 [outcome] Existing scenarios that assert a browser `purchase` in dataLayer (`journeys/buyer_full_funnel_journey.feature`, `frontend/ui_checkout_order.feature` "purchase fires once") contradict this change. → **Updated in this change to assert no browser `purchase` in dataLayer while the beacon and warehouse row remain** (default accepted)
- Q3 [trigger/actor] Runtime names the spec leaves open. → **Probes `/healthz` and `/readyz` on port 8085 (no other route); local container `agora-ga4-relay-svc`; `GA4_ENDPOINT` overridable for the e2e MP mock; `ga4_relay_*_total` counters are not scraped in e2e (no metrics route, per "No inbound endpoint beyond probes") and are asserted in unit tests; e2e observes DLQ contents and later relayed orders instead** (default accepted)

### Round 2 — 2026-10-12
- Q4 [data] The relay DLQ stores the verbatim `OrderPaidEvent` including `buyer_id`, and consent denial or an erasure request does not purge DLQ copies. → **Keep the verbatim record (replay needs it); retention 7 days enforced at topic create and with `alter-config`; documented in the proposal Data classification for DPO review** (human)
