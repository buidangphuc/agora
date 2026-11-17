## 1. Code track

### team-analytics
- [x] 1.1 Config keys `IDENTITY_EVENTS_TOPIC`, `ERASURE_CONSUMER_GROUP`, `ERASURE_DLQ_TOPIC`, `ERASURE_TOMBSTONE_SECRET` with validation and strict-env rule; `.env.example` and README section 4 (AUE-16, AUE-17, AUE-18)
- [x] 1.2 `erased_users` table in `warehouse.Schema` and boot DDL, DuckDB only (AUE-12, AUE-13)
- [x] 1.3 `internal/events/erasure.go`: decode `UserErased`, skip other types, DLQ undecodable/empty id with `error` header, fingerprinted logging (AUE-1, AUE-2, AUE-3, AUE-15)
- [ ] 1.4 DuckDB purge in one transaction: tracking re-key incl. anonymous-id stitching, order/engagement/listing re-key, favourite and follow deletes, tombstone insert, idempotent (AUE-4 to AUE-11)
- [x] 1.5 Writers re-key tombstoned users inside the insert transaction for `tracking_events`, `order_facts`, `engagement_facts` (AUE-12)
- [x] 1.6 Wire the consumer in `internal/app` for `WAREHOUSE_DRIVER=duckdb`, commit after purge (AUE-1, AUE-4); README section 2 (Events) and section 3 (Data)
- [x] 1.7 Verify export files carry no erased id after one cycle (AUE-14)

### platform-recsys
- [x] 1.8 `recsys/erasure` module and `python -m recsys.erasure`: confluent-kafka consumer, Redis key and Qdrant deletes, DLQ, settings `ERASURE_CONSUMER_GROUP`, `ERASURE_DLQ_TOPIC`; README section 2 (AUE-19 to AUE-23)

### platform-featurestore
- [x] 1.9 Input views exclude `erased:` keys; dataset output likewise (AUE-25, AUE-26)
- [x] 1.10 Online reconcile of `user_activity` and `user_preferences` in `materialize`, before pointers; README (AUE-24, AUE-26, AUE-27)

### team-ai
- [x] 1.11 Erasure addon on the aiokafka stack, SCAN-based deletes with escaped id match, DLQ `identity.events.team-ai.dlq`, settings and README (AUE-28 to AUE-32)

### Stack
- [x] 1.12 `docker-compose.services.yaml`: three DLQ topics in topic init, consumer env, recsys erasure service, non-root (AUE-33, AUE-34)

## 2. E2E track
- [x] 2.1 Integration test (platform-e2e, non-UI): seed a user across tracking, orders, favourites and chat session, produce `UserErased`, assert warehouse re-key and no raw id (AUE-1, AUE-6, AUE-9, AUE-10)
- [x] 2.2 Integration test: redelivery and poison record paths (AUE-3, AUE-5, AUE-22, AUE-31)
- [x] 2.3 Integration test: Redis and Qdrant erasure across recsys, featurestore and team-ai after export + materialize, DLQs empty (AUE-19, AUE-24, AUE-28, AUE-33)
- [x] 2.4 Integration test: late event after erasure is re-keyed (AUE-12)
- [x] 2.5 Register the capability as `not-testable` UI / integration in team-analytics FEATURES.yaml via the e2e-author (AUE-1)
