## 1. Code track

### team-engagement
- [x] 1.1 Tests first (red): store test (Docker, `GO_SDK_REQUIRE_DOCKER=1`) `erase_pg_test.go` for anonymised reviews/Q&A, `hide_reason`/`hidden_by` clearing, deleted activity tables, other-user isolation, replay, rollback (scenarios: "A review keeps rating and text under an anonymised author", "A question and its answers keep their text under an anonymised author", "The listing rating is unchanged", "The moderation reason of an erased author is cleared", "An erased moderator no longer appears as hidden_by", "Other users' content is untouched", "Favourites, follows and reports filed by the user are deleted", "An erased seller loses their followers", "Collections, history, check-ins and loyalty are deleted", "Reports about the erased user's content stay open", "A replayed event changes nothing", "A store failure rolls back and is retried")
- [x] 1.2 Tests first (red): handler test `internal/events/identity_test.go` for malformed envelope, empty user id, other event types, no raw id in logs (scenarios: "A malformed event goes to the DLQ", "Other identity events are ignored", "The erased id is not logged")
- [x] 1.3 Implement `Erase(ctx, userID)` in `internal/store` (one transaction; re-key and anonymise, delete, clear) (scenarios: all store scenarios of 1.1)
- [x] 1.4 Implement the `UserErased` consumer in `internal/events/identity.go`, config `IDENTITY_EVENTS_TOPIC` + `KAFKA_ERASURE_CONSUMER_GROUP` (default `team-engagement-erasure`), DLQ `identity.events.engagement.dlq`, wired in `internal/app` (scenarios: "A malformed event goes to the DLQ", "Other identity events are ignored", "Erasure completes within one hour")

### team-chat
- [x] 1.5 Tests first (red): store test `erase_pg_test.go` for placeholder text, sender name, emptied payload, thread preview, counterparty and other-user isolation, replay, rollback (scenarios: "The user's messages are replaced and the thread is kept", "A rich message payload is emptied", "The thread preview no longer shows erased text", "Other users' messages are untouched", "A replayed event changes nothing", "A store failure rolls back and is retried")
- [x] 1.6 Tests first (red): handler test for malformed, empty id, other types, no raw id or text in logs (scenarios: "A malformed event goes to the DLQ", "Other identity events are ignored", "Message text and the erased id are not logged")
- [x] 1.7 Implement `Erase` in `internal/store/chat.go` and the consumer in `internal/events/identity.go`, config, wiring, group `team-chat-erasure`, DLQ `identity.events.chat.dlq` (scenarios: all chat scenarios)

### team-notification
- [x] 1.8 Tests first (red): store test `erase_pg_test.go` for deletes, isolation, unknown user, replay, rollback (scenarios: "The user's notification data is deleted", "Other users' notifications are untouched", "A user with no notification data is a no-op", "A replayed event changes nothing", "A store failure rolls back and is retried")
- [x] 1.9 Tests first (red): handler test for malformed, empty id, other types, no raw id in logs (scenarios: "A malformed event goes to the DLQ", "Other identity events are ignored", "The erased id is not logged")
- [x] 1.10 Implement `Erase` in `internal/store` and the consumer in `internal/events/identity.go`, config, wiring, group `team-notification-erasure`, DLQ `identity.events.notification.dlq` (scenarios: all notification scenarios)

### Compose
- [x] 1.11 `docker-compose.services.yaml`: add the three DLQ topics to the topic init line and the topic/group env to the three services (additive edit; hot file, serialised by initiative-build) (scenarios: "A malformed event goes to the DLQ" in all three specs)

## 2. E2E track (platform-e2e)

- [x] 2.1 Feature `engagement/user_erasure.feature`: publish a `UserErased` event through the identity outbox path on `mainx` and assert review/Q&A anonymisation, rating unchanged, hide fields, deleted activity, seller followers, isolation, replay, 1 h bound (scenarios: all engagement scenarios marked platform-e2e)
- [x] 2.2 Feature `chat/user_erasure.feature`: placeholder text, rich payload, thread preview, counterparty view, isolation, replay (scenarios: chat scenarios marked platform-e2e)
- [x] 2.3 Feature `notification/user_erasure.feature`: deletion, isolation, replay (scenarios: notification scenarios marked platform-e2e)
- [x] 2.4 Topic-source steps: produce `UserErased` to another topic and call a user-principal RPC naming another user, assert no change (scenario "Erasure is reachable only through the identity topic" in all three specs)
- [x] 2.4b DLQ steps: produce a malformed record to `identity.events` and assert it lands in each `identity.events.<svc>.dlq` and that the DLQs are empty after a normal erasure (scenarios: "A malformed event goes to the DLQ" in all three specs)
- [x] 2.5 `openspec validate content-user-erasure --strict`, team-engagement / team-chat / team-notification gates, pre-commit-ci
