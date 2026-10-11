package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
)

// OutboxRetention is how long published outbox rows are kept before the relayer
// deletes them (matches team-order).
const OutboxRetention = 7 * 24 * time.Hour

// relayLockKey is the advisory-lock id that lets only one relayer replica
// publish at a time, so rows leave in seq order.
const relayLockKey int64 = 0x656e675f6f7574 // "eng_out"

// OutboxMessage is one outbox row as the relayer sees it.
type OutboxMessage struct {
	Seq           int64
	EventID       string
	Type          string // payload's full proto name
	Key           string // listing id, or seller id for follows
	Payload       []byte // serialized fact message (no envelope)
	PrincipalID   string
	PrincipalType string
	RequestID     string
	OccurredAt    time.Time
}

// enqueueFact writes one fact row inside the caller's transaction. The envelope
// principal is the caller, taken from the incoming principal metadata (resolved by
// the auth interceptor); the type is the payload's full name.
func enqueueFact(ctx context.Context, tx pgx.Tx, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal fact: %w", err)
	}
	var pid, ptype string
	if p, ok := interceptor.PrincipalFromContext(ctx); ok && p != nil {
		pid, ptype = p.GetId(), p.GetType().String()
	}
	reqID, _ := interceptor.RequestIDFromContext(ctx)
	_, err = tx.Exec(ctx, `
INSERT INTO outbox (event_id, type, key, payload, principal_id, principal_type, request_id, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		uuid.New(), string(msg.ProtoReflect().Descriptor().FullName()), key, payload, pid, ptype, reqID)
	if err != nil {
		return fmt.Errorf("enqueue fact: %w", err)
	}
	return nil
}

// PrincipalOf rebuilds the envelope principal from a stored row; nil when the row
// carries none.
func (m OutboxMessage) PrincipalOf() *commonv1.Principal {
	if m.PrincipalID == "" && m.PrincipalType == "" {
		return nil
	}
	return &commonv1.Principal{
		Id:   m.PrincipalID,
		Type: commonv1.PrincipalType(commonv1.PrincipalType_value[m.PrincipalType]),
	}
}

// PgOutbox reads and trims the outbox for the relayer.
type PgOutbox struct{ pool *pgxpool.Pool }

func NewPgOutbox(pool *pgxpool.Pool) *PgOutbox { return &PgOutbox{pool: pool} }

// Relay runs one relay cycle: it deletes published rows older than retention, then
// hands up to batch unpublished rows to publish in seq order and stamps each one
// published. It stops at the first publish error (so order is kept), keeps the
// marks already earned, and returns that error. Only one replica relays at a time.
func (o *PgOutbox) Relay(ctx context.Context, batch int, retention time.Duration,
	publish func(context.Context, OutboxMessage) error) (int, error) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin relay: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, relayLockKey).Scan(&locked); err != nil {
		return 0, fmt.Errorf("relay lock: %w", err)
	}
	if !locked {
		return 0, nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM outbox WHERE published_at IS NOT NULL AND published_at < now() - $1::interval`,
		fmt.Sprintf("%d milliseconds", retention.Milliseconds())); err != nil {
		return 0, fmt.Errorf("trim outbox: %w", err)
	}

	rows, err := tx.Query(ctx, `
SELECT seq, event_id::text, type, key, payload, principal_id, principal_type, request_id, occurred_at
FROM outbox WHERE published_at IS NULL ORDER BY seq LIMIT $1`, batch)
	if err != nil {
		return 0, fmt.Errorf("select unpublished: %w", err)
	}
	var msgs []OutboxMessage
	for rows.Next() {
		var m OutboxMessage
		if err := rows.Scan(&m.Seq, &m.EventID, &m.Type, &m.Key, &m.Payload,
			&m.PrincipalID, &m.PrincipalType, &m.RequestID, &m.OccurredAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox: %w", err)
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate outbox: %w", err)
	}

	var done []int64
	var pubErr error
	for _, m := range msgs {
		if pubErr = publish(ctx, m); pubErr != nil {
			break
		}
		done = append(done, m.Seq)
	}
	if len(done) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE seq = ANY($1)`, done); err != nil {
			return 0, fmt.Errorf("mark published: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit relay: %w", err)
	}
	return len(done), pubErr
}
