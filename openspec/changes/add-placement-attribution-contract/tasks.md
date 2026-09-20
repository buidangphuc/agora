# Tasks

## 1. Code — platform-core & Go services
- [x] Add `placement_id = 10`, `impression_id = 11`, `model_version = 12` to `platform.analytics.v1.TrackingEvent` in `platform-core/packages/proto/platform/analytics/v1/analytics.proto`.
- [x] Re-vendor proto in `team-gateway` and `team-analytics`.
- [x] Update `team-gateway/internal/edge/collector.go` to parse `placementId`, `impressionId`, `modelVersion` and map into `analyticsv1.TrackingEvent`.
- [x] Update `team-analytics/internal/warehouse/warehouse.go` and `team-analytics/internal/consumer/tracking.go` to include attribution fields.
- [x] Update `team-frontend/src/lib/track.ts` to include attribution fields in `TrackPayload`.
- [x] Unit tests in `team-gateway` and `team-analytics` verifying attribution persistence.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-placement-attribution-contract --strict`).
- [x] Run Go unit tests for `team-gateway` and `team-analytics`.
