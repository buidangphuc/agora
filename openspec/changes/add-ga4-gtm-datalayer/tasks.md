# Tasks: Add GA4 / GTM Data Layer Pattern

- [x] Contract update in `platform-core/packages/proto/platform/analytics/v1/analytics.proto` <!-- id: contract-proto -->
- [x] Vendor proto to `team-gateway`, `team-analytics`, `team-frontend` and run buf generate <!-- id: proto-vendor -->
- [ ] Implement `src/lib/analytics/` module in `team-frontend` with dataLayer, queue, map, dispatcher <!-- id: fe-datalayer -->
- [ ] Migrate `team-frontend` call sites to batched ecommerce dispatcher <!-- id: fe-callsites -->
- [ ] Update `team-gateway` collector with GA4 aliases, ecommerce fields and batch limits <!-- id: gw-collector -->
- [ ] Add idempotent schema migration, GA4 view and funnel query in `team-analytics` <!-- id: wh-migration -->
- [ ] Update `platform-recsys` event weights and columns <!-- id: recsys-weights -->
- [ ] Add E2E BDD feature scenarios in `platform-e2e` <!-- id: e2e-tests -->
