## Context

`team-engagement/internal/events/listing.go` and `team-domain/internal/events/identity.go` show the pattern: a
`consume.Handler` that unmarshals `EventEnvelope`, filters by `env.GetType()`, wraps malformed input in
`consume.Permanent` and returns store errors so `consume.Consumer` retries (5 attempts) and then publishes to the
DLQ topic. The same pattern is used three times here.

## Decisions

- Event filter: handle only `platform.identity.v1.UserErased`; every other type on `identity.events` returns nil.
  Empty `user_id` is `consume.Permanent`.
- One `Erase(ctx, userID)` store method per service, one transaction, only `UPDATE`/`DELETE` statements keyed by the
  event's user id. Re-running matches no row or rewrites identical values, so it is idempotent without a
  processed-events table.
- Consumer groups: `team-engagement-erasure`, `team-chat-erasure`, `team-notification-erasure` (env
  `KAFKA_ERASURE_CONSUMER_GROUP`), distinct from existing groups so offsets are independent. Topic env `IDENTITY_EVENTS_TOPIC`
  (compose already sets it for other services). DLQ topic is `identity.events.<svc>.dlq` with `<svc>` = `engagement`,
  `chat`, `notification`.
- Author anonymisation in engagement: `reviews.user_name` = "Người dùng đã xóa", `reviews.user_id` = `deleted`;
  `product_questions.user_id` and `product_answers.user_id` = `deleted`. A fixed value (not a per-user hash) is used
  because a stable per-user pseudonym would let readers link the erased user's posts, and a keyed hash needs a secret
  that does not exist locally. `uq_reviews_order_listing_verified` is on `(order_id, listing_id)`, not on user, so
  re-keying cannot collide. `review_helpful_votes` has PK `(review_id, user_id)`; the user's votes are deleted before
  any re-key, and the stored `helpful_count` is left unchanged.
- `hidden_by`: `UPDATE reviews SET hidden_by = '' WHERE hidden_by = $1`; `hidden_at` is kept so a hidden review stays hidden.
  `hide_reason`: cleared on rows where `user_id = $1` (the erased user's reviews) in the same statement that re-keys them.
- `UserErased.roles` is read only to log counts; the statements run regardless of role (follows are deleted for
  both `user_id` and `seller_id`), so a user who was both buyer and seller is fully handled.
- Chat: `UPDATE chat_messages SET content = 'Tin nhắn đã bị xóa', sender_name = 'Người dùng đã xóa', payload = '' WHERE sender_id = $1`;
  `sender_id` and thread `buyer_id`/`seller_id` are kept so the counterparty keeps a coherent thread (identity has
  already anonymised the id's owner). `chat_threads.last_message_text` is replaced with the same placeholder on
  threads where the user is a party and the last message was sent by the user.
- Notification: delete from `notifications`, `alert_subscriptions`, `notification_prefs` by `user_id`;
  `processed_events` and `listing_*` tables hold no user data.
- Other engagement tables keyed by user (`view_history`, `collections` + `collection_items` through the existing
  cascade or explicit delete, `checkins`, `loyalty_accounts`): deleted. This goes one step beyond the locked list
  (favorites, follows, reports) because they are plainly personal data keyed by the user id; flagged in open issues.
- Engagement outbox: erasure emits no event (ratings are unchanged, so `listing_rating` facts stay valid).
- Logging: `erased user hash=<h> tables=<n> rows=<n>` only, using the go-sdk key fingerprint.

## Risks

- Large users: a single transaction over many rows; mitigated by NFR (<= 5 s at 1000 rows) and indexes already on
  `user_id` columns; if a user exceeds this the consumer retries and the DLQ catches it.
- Moderator erasure: clearing `hidden_by` loses who hid a review; accepted per the initiative spec, team-audit holds the immutable copy.
