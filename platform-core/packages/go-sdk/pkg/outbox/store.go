package outbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

// ErrNotFound is returned when a mark targets a row that does not exist.
var ErrNotFound = errors.New("outbox row not found")

var tableRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,40}$`)

func checkTable(table string) error {
	if !tableRe.MatchString(table) {
		return fmt.Errorf("outbox: invalid table name %q: want a lowercase identifier of at most 41 characters", table)
	}
	return nil
}

// ErrEmptyEventID is returned by EnqueueTx for a Row without an EventID. The
// event id is the idempotency key, so an empty one would be silently dropped.
var ErrEmptyEventID = errors.New("outbox: empty event id")

// Row is one event to enqueue. Payload is published verbatim, keyed by AggregateID.
type Row struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	// RequestID is published as the x-request-id header. When empty, EnqueueTx
	// takes it from the context (reqctx.RequestID).
	RequestID string
	// Topic overrides the relayer's default topic for this row. Empty means the
	// relayer's Topic.
	Topic string
}

// pending is a claimed row.
type pending struct {
	eventID, aggregateType, aggregateID, eventType string
	topic, requestID, traceparent                  string
	payload                                        []byte
	attempts                                       int
	seq                                            int64
}

// Store reads and writes one outbox table.
type Store struct {
	pool  *pgxpool.Pool
	table string
}

// NewStore returns a store over an existing table with the SchemaSQL columns.
// It returns an error when pool is nil or table is not a lowercase identifier
// (at most 41 characters), because the name is placed into SQL.
func NewStore(pool *pgxpool.Pool, table string) (*Store, error) {
	if err := checkTable(table); err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, errors.New("outbox: nil pool")
	}
	return &Store{pool: pool, table: table}, nil
}

// SchemaSQL returns the statements that create an outbox table, for a service
// that has none yet. It returns an error on an invalid table name.
//
// The statements are idempotent, but they only create: on a table made by an
// SDK older than v1 the index on the new seq column fails, because that column
// does not exist, and the table is untouched. Upgrade such a table with
// UpgradeSQL, or by hand with the steps below, which keep the existing events
// in their old order and take no long lock. Stop the v0 relayers (or let them
// drain the pending rows) before switching to v1: a v0 relayer ignores seq.
// Purge published rows first (Store.Purge): they need no ordered numbers, and
// every Backfill batch sorts all rows that are still unnumbered.
//
//  1. Add the columns and the sequence (instant, no rewrite):
//
//     ALTER TABLE <table> ADD COLUMN IF NOT EXISTS topic       TEXT;
//     ALTER TABLE <table> ADD COLUMN IF NOT EXISTS traceparent TEXT;
//     ALTER TABLE <table> ADD COLUMN IF NOT EXISTS seq         BIGINT;
//     CREATE SEQUENCE IF NOT EXISTS <table>_seq_seq OWNED BY <table>.seq;
//
//  2. Number the existing rows by (created_at, event_id), in batches of 10000,
//     and repeat the statement until updated is 0. Each batch takes its numbers
//     from the sequence in that order and locks its rows, so concurrent runs
//     cannot number a row twice or out of order:
//
//     WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
//     (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
//     u AS (UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
//     SELECT count(*) AS updated FROM u;
//
//  3. Switch, in one transaction. It blocks writers to the table briefly, numbers
//     the rows a v0 writer inserted since the last batch the same way, and
//     gives new rows a number. After it commits no row is left without a seq:
//
//     BEGIN;
//     LOCK TABLE <table> IN SHARE ROW EXCLUSIVE MODE;
//     WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
//     (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id) x)
//     UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id;
//     ALTER TABLE <table> ALTER COLUMN seq SET DEFAULT nextval('<table>_seq_seq');
//     COMMIT;
//
//  4. Make seq NOT NULL without a long ACCESS EXCLUSIVE scan. The first
//     statement lets a run that was interrupted start over:
//
//     ALTER TABLE <table> DROP CONSTRAINT IF EXISTS <table>_seq_nn;
//     ALTER TABLE <table> ADD CONSTRAINT <table>_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
//     ALTER TABLE <table> VALIDATE CONSTRAINT <table>_seq_nn;
//     ALTER TABLE <table> ALTER COLUMN seq SET NOT NULL;
//     ALTER TABLE <table> DROP CONSTRAINT <table>_seq_nn;
//
//  5. Build the indexes and drop the unused v0 one. CREATE INDEX CONCURRENTLY
//     cannot run in a transaction; if one fails, DROP the invalid index and
//     run it again:
//
//     CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_status_idx ON <table> (status);
//     CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_agg_idx ON <table> (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
//     CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_pending_idx ON <table> (seq) WHERE status = 'pending';
//     DROP INDEX CONCURRENTLY IF EXISTS <table>_claim_idx;
//
// seq orders events: created_at is the transaction start, so events enqueued in
// one transaction tie on it. The old principal column is no longer written and
// may stay or be dropped. Rows a v0 relayer parked as failed block their
// aggregates under v1: requeue or delete them before deploying v1
// (Store.RequeueParked).
func SchemaSQL(table string) (string, error) {
	if err := checkTable(table); err != nil {
		return "", err
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %[1]s (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    topic          TEXT,
    request_id     TEXT        NOT NULL DEFAULT '',
    traceparent    TEXT,
    seq            BIGSERIAL,
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS topic       TEXT;
ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS traceparent TEXT;
CREATE INDEX IF NOT EXISTS %[1]s_status_idx ON %[1]s (status);
CREATE INDEX IF NOT EXISTS %[1]s_agg_idx ON %[1]s (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX IF NOT EXISTS %[1]s_pending_idx ON %[1]s (seq) WHERE status = 'pending';
`, table), nil
}

// Upgrade is the migration of a table created by an SDK older than v1, as
// ordered steps. UpgradeSQL documents them; the SchemaSQL godoc explains each.
// Stop the v0 relayers first, Purge published rows, then run:
//
//	Prepare, each statement on its own;
//	Backfill until its first result column (updated) is 0;
//	Switch, all statements in one transaction;
//	Finish, each statement on its own (the index statements cannot run inside
//	a transaction).
//
// Do not run Backfill after Switch: no row is left without a seq by then.
type Upgrade struct {
	// Prepare adds the columns and the sequence.
	Prepare []string
	// Backfill numbers one batch of rows, oldest first, from the sequence and
	// returns "updated" rows in its single result row; run it until updated is 0.
	// Concurrent runs are safe: each batch locks its rows.
	Backfill string
	// Switch locks the table against writers, numbers every row that is still
	// without a seq, and gives new rows a number by default. Run its statements
	// in one transaction.
	Switch []string
	// Finish makes seq NOT NULL, creates the indexes and drops claim_idx. It can
	// be run again after an interruption.
	Finish []string
}

// UpgradeSQL returns the v0 to v1 migration steps of SchemaSQL for table.
func UpgradeSQL(table string) (Upgrade, error) {
	if err := checkTable(table); err != nil {
		return Upgrade{}, err
	}
	f := func(s string) string { return strings.ReplaceAll(s, "<table>", table) }
	return Upgrade{
		Prepare: []string{
			f(`ALTER TABLE <table> ADD COLUMN IF NOT EXISTS topic       TEXT`),
			f(`ALTER TABLE <table> ADD COLUMN IF NOT EXISTS traceparent TEXT`),
			f(`ALTER TABLE <table> ADD COLUMN IF NOT EXISTS seq         BIGINT`),
			f(`CREATE SEQUENCE IF NOT EXISTS <table>_seq_seq OWNED BY <table>.seq`),
		},
		Backfill: f(`WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
  (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id LIMIT 10000 FOR UPDATE) x),
u AS (UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id AND t.seq IS NULL RETURNING 1)
SELECT count(*) AS updated FROM u`),
		Switch: []string{
			f(`LOCK TABLE <table> IN SHARE ROW EXCLUSIVE MODE`),
			f(`WITH b AS MATERIALIZED (SELECT event_id, nextval('<table>_seq_seq') AS n FROM
  (SELECT event_id FROM <table> WHERE seq IS NULL ORDER BY created_at, event_id) x)
UPDATE <table> t SET seq = b.n FROM b WHERE t.event_id = b.event_id`),
			f(`ALTER TABLE <table> ALTER COLUMN seq SET DEFAULT nextval('<table>_seq_seq')`),
		},
		Finish: []string{
			f(`ALTER TABLE <table> DROP CONSTRAINT IF EXISTS <table>_seq_nn`),
			f(`ALTER TABLE <table> ADD CONSTRAINT <table>_seq_nn CHECK (seq IS NOT NULL) NOT VALID`),
			f(`ALTER TABLE <table> VALIDATE CONSTRAINT <table>_seq_nn`),
			f(`ALTER TABLE <table> ALTER COLUMN seq SET NOT NULL`),
			f(`ALTER TABLE <table> DROP CONSTRAINT <table>_seq_nn`),
			f(`CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_status_idx ON <table> (status)`),
			f(`CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_agg_idx ON <table> (aggregate_type, aggregate_id, seq) WHERE status <> 'published'`),
			f(`CREATE INDEX CONCURRENTLY IF NOT EXISTS <table>_pending_idx ON <table> (seq) WHERE status = 'pending'`),
			f(`DROP INDEX CONCURRENTLY IF EXISTS <table>_claim_idx`),
		},
	}, nil
}

// EnqueueTx inserts a pending row inside tx, so it commits or rolls back with the
// caller's business write. Enqueuing an event id that already exists is a no-op,
// so a retried business write neither fails nor duplicates the event. An empty
// EventID is rejected with ErrEmptyEventID.
//
// The W3C traceparent of ctx is stored with the row and republished as a
// header, so the consumer's trace continues the producer's. An empty
// Row.RequestID defaults to the request id in ctx.
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, r Row) error {
	if r.EventID == "" {
		return ErrEmptyEventID
	}
	if r.RequestID == "" {
		r.RequestID = reqctx.RequestID(ctx)
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	q := `INSERT INTO ` + s.table + `
		(event_id, aggregate_type, aggregate_id, event_type, payload, topic, request_id, traceparent, status, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''), 'pending', 0, now(), now())
		ON CONFLICT (event_id) DO NOTHING`
	if _, err := tx.Exec(ctx, q, r.EventID, r.AggregateType, r.AggregateID, r.EventType, r.Payload, r.Topic, r.RequestID, carrier.Get("traceparent")); err != nil {
		return fmt.Errorf("enqueue outbox event %q: %w", r.EventID, err)
	}
	return nil
}

// Purge deletes published rows older than olderThan and returns how many it
// removed. Pending and parked rows are never deleted. Run it periodically; the
// table otherwise grows without bound.
func (s *Store) Purge(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM `+s.table+`
		WHERE status = 'published' AND published_at < now() - ($1 * interval '1 second')`, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("purge outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

// claimCols is the column list scanned into a pending row.
const claimCols = `event_id, aggregate_type, aggregate_id, event_type, payload, COALESCE(topic, ''), request_id, COALESCE(traceparent, ''), attempts, seq`

func scanPending(row pgx.Row) (pending, error) {
	var p pending
	err := row.Scan(&p.eventID, &p.aggregateType, &p.aggregateID, &p.eventType, &p.payload, &p.topic, &p.requestID, &p.traceparent, &p.attempts, &p.seq)
	return p, err
}

// claimableCond is true for a row a relayer may take now: pending, due and not
// leased by another relayer.
const claimableCond = `status = 'pending' AND available_at <= now() AND (locked_until IS NULL OR locked_until <= now())`

// claim leases up to batch rows for lock and returns them grouped by
// aggregate, each group in seq order, with the aggregate the next scan should
// start after (nil to start from the beginning).
//
// Order is per aggregate: only an aggregate's head (its oldest unpublished row)
// may be claimed, and only when the head is pending, due and unleased. An
// aggregate whose head is parked, backing off or leased by another relayer is
// blocked, and its backlog is never looked at. A claimed head brings along its
// run: the rows right behind it that are claimable too, so one pass moves a
// long backlog of one aggregate by up to batch rows instead of one.
//
// The heads are found with a recursive skip scan over the partial index
// (aggregate_type, aggregate_id, seq) WHERE status <> 'published': one index
// probe per aggregate, never one per row, so work grows with the number of
// aggregates that have unpublished rows, not with their backlog. Heads are
// visited in aggregate key order starting after the cursor, then wrapping to the
// start; the scan is lazy and stops once batch claimable heads were found, so a
// busy low key cannot starve a high one. Heads are locked with FOR UPDATE SKIP
// LOCKED, so concurrent relayers never share an aggregate: a relayer that does
// not get the head does not touch the run.
func (s *Store) claim(ctx context.Context, batch int, lock time.Duration, after *aggKey) (_ []pending, next *aggKey, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("claim outbox rows: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	lockSecs := lock.Seconds()

	ids, next, err := s.findHeads(ctx, tx, batch, after)
	if err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, next, tx.Commit(ctx)
	}

	headRows, err := tx.Query(ctx, `
		UPDATE `+s.table+` SET locked_until = now() + ($2::float8 * interval '1 second')
		WHERE event_id IN (
			SELECT event_id FROM `+s.table+` WHERE event_id = ANY($1) AND `+claimableCond+`
			FOR UPDATE SKIP LOCKED)
		RETURNING `+claimCols, ids, lockSecs)
	if err != nil {
		return nil, nil, fmt.Errorf("lock outbox heads: %w", err)
	}
	var heads []pending
	for headRows.Next() {
		p, err := scanPending(headRows)
		if err != nil {
			headRows.Close()
			return nil, nil, fmt.Errorf("scan outbox row: %w", err)
		}
		heads = append(heads, p)
	}
	headRows.Close()
	if err := headRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("lock outbox heads: %w", err)
	}

	out := heads
	if perHead := (batch+len(heads)-1)/max(len(heads), 1) - 1; perHead > 0 && len(heads) > 0 {
		runs, err := s.runs(ctx, tx, heads, perHead, lockSecs)
		if err != nil {
			return nil, nil, err
		}
		out = out[:0:0]
		for i, h := range heads {
			out = append(out, h)
			out = append(out, runs[i]...)
		}
		if len(out) > batch {
			extra := out[batch:]
			out = out[:batch]
			if err := s.unlock(ctx, tx, extra); err != nil {
				return nil, nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("claim outbox rows: %w", err)
	}
	return out, next, nil
}

// findHeads returns the event ids of up to batch claimable heads, scanning
// after the cursor first and wrapping to the start, and the aggregate of the
// last head found (nil when there is none, so the next scan starts over).
func (s *Store) findHeads(ctx context.Context, tx pgx.Tx, batch int, after *aggKey) (ids []string, last *aggKey, err error) {
	scan := func(mode headMode, limit int, args ...any) error {
		rows, err := tx.Query(ctx, headsSQL(s.table, mode), append([]any{limit}, args...)...)
		if err != nil {
			return fmt.Errorf("find outbox heads: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var k aggKey
			if err := rows.Scan(&id, &k.typ, &k.id); err != nil {
				return fmt.Errorf("scan outbox head: %w", err)
			}
			ids, last = append(ids, id), &k
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("find outbox heads: %w", err)
		}
		return nil
	}
	if after == nil {
		return ids, last, scan(scanAll, batch)
	}
	if err := scan(scanAfter, batch, after.typ, after.id); err != nil {
		return nil, nil, err
	}
	if len(ids) < batch {
		if err := scan(scanThrough, batch-len(ids), after.typ, after.id); err != nil {
			return nil, nil, err
		}
	}
	return ids, last, nil
}

// headMode bounds the head scan by the cursor.
type headMode int

const (
	scanAll     headMode = iota // every aggregate
	scanAfter                   // aggregates after the cursor ($2, $3)
	scanThrough                 // aggregates up to and including the cursor ($2, $3)
)

// headsSQL is the skip scan that finds up to $1 claimable aggregate heads, as
// (event_id, aggregate_type, aggregate_id). It is a function of the table so a
// test can EXPLAIN exactly what claim runs.
//
// The scan after a cursor starts at an index boundary. The scan up to a cursor
// (the wrap-around) must not use an upper bound in the index probe: a row
// comparison bounds only the leading column, so the probe past the last wanted
// aggregate would read the whole tail of the index, backlog included. It stops
// instead from the outside, by not stepping from a head at or past the cursor,
// and drops the one head it read beyond it.
func headsSQL(table string, mode headMode) string {
	const key = `(aggregate_type, aggregate_id)`
	var seedBound, stepFilter, endWhere string
	switch mode {
	case scanAfter:
		seedBound = ` AND ` + key + ` > ($2, $3)`
	case scanThrough:
		stepFilter = ` WHERE (h.aggregate_type, h.aggregate_id) < ($2, $3)`
		endWhere = ` AND ` + key + ` <= ($2, $3)`
	}
	headSQL := `SELECT event_id, aggregate_type, aggregate_id, seq, status, available_at, locked_until FROM ` + table + `
		WHERE status <> 'published'`
	return `
		WITH RECURSIVE heads AS (
			(` + headSQL + seedBound + ` ORDER BY aggregate_type, aggregate_id, seq LIMIT 1)
			UNION ALL
			SELECT n.* FROM heads h CROSS JOIN LATERAL (
				` + headSQL + ` AND ` + key + ` > (h.aggregate_type, h.aggregate_id)
				ORDER BY aggregate_type, aggregate_id, seq LIMIT 1) n` + stepFilter + `
		)
		SELECT event_id, aggregate_type, aggregate_id FROM heads WHERE ` + claimableCond + endWhere + ` LIMIT $1`
}

// runs finds, per head, up to limit rows right behind it that are claimable,
// leases them and returns them per head (same index as heads). A run stops at
// the first row that is not claimable, so order is never skipped.
func (s *Store) runs(ctx context.Context, tx pgx.Tx, heads []pending, limit int, lockSecs float64) ([][]pending, error) {
	types, aggs, seqs := make([]string, len(heads)), make([]string, len(heads)), make([]int64, len(heads))
	for i, h := range heads {
		types[i], aggs[i], seqs[i] = h.aggregateType, h.aggregateID, h.seq
	}
	rows, err := tx.Query(ctx, `
		SELECT h.ord, f.ok, f.event_id, f.aggregate_type, f.aggregate_id, f.event_type, f.payload, f.topic, f.request_id, f.traceparent, f.attempts, f.seq
		FROM unnest($1::text[], $2::text[], $3::bigint[]) WITH ORDINALITY AS h(t, a, seq, ord)
		CROSS JOIN LATERAL (
			SELECT (`+claimableCond+`) AS ok, event_id, aggregate_type, aggregate_id, event_type, payload,
			       COALESCE(topic, '') AS topic, request_id, COALESCE(traceparent, '') AS traceparent, attempts, seq
			FROM `+s.table+`
			WHERE aggregate_type = h.t AND aggregate_id = h.a AND status <> 'published' AND seq > h.seq
			ORDER BY seq LIMIT $4) f
		ORDER BY h.ord, f.seq`, types, aggs, seqs, limit)
	if err != nil {
		return nil, fmt.Errorf("find outbox runs: %w", err)
	}
	runs := make([][]pending, len(heads))
	stopped := make([]bool, len(heads))
	for rows.Next() {
		var (
			ord int
			ok  bool
			p   pending
		)
		if err := rows.Scan(&ord, &ok, &p.eventID, &p.aggregateType, &p.aggregateID, &p.eventType, &p.payload, &p.topic, &p.requestID, &p.traceparent, &p.attempts, &p.seq); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan outbox run: %w", err)
		}
		i := ord - 1
		if stopped[i] || !ok {
			stopped[i] = true
			continue
		}
		runs[i] = append(runs[i], p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find outbox runs: %w", err)
	}

	var all []string
	for _, r := range runs {
		for _, p := range r {
			all = append(all, p.eventID)
		}
	}
	if len(all) == 0 {
		return runs, nil
	}
	locked, err := tx.Query(ctx, `
		UPDATE `+s.table+` SET locked_until = now() + ($2::float8 * interval '1 second')
		WHERE event_id = ANY($1) AND `+claimableCond+` RETURNING event_id`, all, lockSecs)
	if err != nil {
		return nil, fmt.Errorf("lock outbox runs: %w", err)
	}
	got := map[string]bool{}
	for locked.Next() {
		var id string
		if err := locked.Scan(&id); err != nil {
			locked.Close()
			return nil, fmt.Errorf("scan locked outbox row: %w", err)
		}
		got[id] = true
	}
	locked.Close()
	if err := locked.Err(); err != nil {
		return nil, fmt.Errorf("lock outbox runs: %w", err)
	}
	// A row that could not be leased ends its run; leased rows behind it go back.
	for i, r := range runs {
		j := 0
		for j < len(r) && got[r[j].eventID] {
			delete(got, r[j].eventID)
			j++
		}
		runs[i] = r[:j]
	}
	var cut []string
	for id := range got {
		cut = append(cut, id)
	}
	if err := s.unlockIDs(ctx, tx, cut); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *Store) unlock(ctx context.Context, tx pgx.Tx, rows []pending) error {
	ids := make([]string, len(rows))
	for i, p := range rows {
		ids[i] = p.eventID
	}
	return s.unlockIDs(ctx, tx, ids)
}

func (s *Store) unlockIDs(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE `+s.table+` SET locked_until = NULL WHERE event_id = ANY($1) AND status = 'pending'`, ids); err != nil {
		return fmt.Errorf("unlock outbox rows: %w", err)
	}
	return nil
}

// release ends the lease on rows that were claimed but not attempted, so the
// next pass can take them at once instead of after the lease expires.
func (s *Store) release(ctx context.Context, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `UPDATE `+s.table+` SET locked_until = NULL WHERE event_id = ANY($1) AND status = 'pending'`, eventIDs); err != nil {
		return fmt.Errorf("release outbox rows: %w", err)
	}
	return nil
}

// deferRows ends the lease on rows that were claimed but cannot be attempted
// and holds them back until at, without counting an attempt.
func (s *Store) deferRows(ctx context.Context, eventIDs []string, at time.Time) error {
	if len(eventIDs) == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `UPDATE `+s.table+` SET locked_until = NULL, available_at = $2 WHERE event_id = ANY($1) AND status = 'pending'`, eventIDs, at); err != nil {
		return fmt.Errorf("defer outbox rows: %w", err)
	}
	return nil
}

func (s *Store) markPublished(ctx context.Context, eventID string) error {
	q := `UPDATE ` + s.table + `
		SET status = 'published', published_at = now(), locked_until = NULL, error = NULL
		WHERE event_id = $1`
	tag, err := s.pool.Exec(ctx, q, eventID)
	if err != nil {
		return fmt.Errorf("mark outbox %q published: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// markFailed records a failed publish: attempts+1, the error and the lease
// released. With a non-nil availableAt the row stays pending behind that
// backoff gate; with nil it is parked as failed.
func (s *Store) markFailed(ctx context.Context, eventID, reason string, availableAt *time.Time) error {
	var q string
	args := []any{eventID, reason}
	if availableAt != nil {
		q = `UPDATE ` + s.table + `
			SET status = 'pending', attempts = attempts + 1, error = $2, available_at = $3, locked_until = NULL
			WHERE event_id = $1`
		args = append(args, *availableAt)
	} else {
		q = `UPDATE ` + s.table + `
			SET status = 'failed', attempts = attempts + 1, error = $2, locked_until = NULL
			WHERE event_id = $1`
	}
	tag, err := s.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark outbox %q failed: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Stats is a snapshot of the unpublished rows of a table.
type Stats struct {
	// Pending counts rows waiting to be published, including those in BackingOff.
	Pending int64
	// BackingOff counts pending rows waiting out a retry delay.
	BackingOff int64
	// Parked counts rows parked as failed by a permanent record error.
	Parked int64
}

// Blocked is the number of aggregates held back by a row that cannot go out
// now: a parked row or a row backing off is always the head of its aggregate.
func (s Stats) Blocked() int64 { return s.BackingOff + s.Parked }

// Stats counts the unpublished rows. Export Parked as a gauge and alert on it:
// a parked row blocks its aggregate until Requeue or RequeueParked. It reads every unpublished
// row, so call it on a metrics interval, not per request.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'pending' AND available_at > now()),
		       count(*) FILTER (WHERE status = 'failed')
		FROM `+s.table+` WHERE status <> 'published'`).Scan(&st.Pending, &st.BackingOff, &st.Parked)
	if err != nil {
		return Stats{}, fmt.Errorf("outbox stats: %w", err)
	}
	return st, nil
}

// Requeue puts a parked or backing-off row back in line at once: status
// pending, attempts reset to 0, no delay, no lease, error cleared. Its aggregate
// resumes from this row. It returns ErrNotFound when the row does not exist or
// is already published (a published row is never sent again).
func (s *Store) Requeue(ctx context.Context, eventID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE `+s.table+`
		SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL
		WHERE event_id = $1 AND status <> 'published'`, eventID)
	if err != nil {
		return fmt.Errorf("requeue outbox %q: %w", eventID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RequeueFilter selects the parked rows RequeueParked puts back in line. Both
// fields narrow the selection; the zero filter selects every parked row.
type RequeueFilter struct {
	// Topic matches the row's own Row.Topic exactly. Rows that publish to the
	// relayer's default topic have none, so select them by ErrorContains.
	Topic string
	// ErrorContains matches rows whose recorded publish error contains this
	// text, case-sensitively.
	ErrorContains string
}

// RequeueParked requeues every parked (failed) row that matches the filter, as
// Requeue does for one row: status pending, attempts reset to 0, no delay, no
// lease, error cleared. Published rows and rows that are only backing off are
// left alone. It returns the number of rows requeued. Use it after fixing the
// cause of a parked burst, for example to put back every row that a broker size
// limit rejected, or to clear the failed rows a v0 relayer left behind.
func (s *Store) RequeueParked(ctx context.Context, f RequeueFilter) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE `+s.table+`
		SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL
		WHERE status = 'failed'
		  AND ($1 = '' OR topic = $1)
		  AND ($2 = '' OR strpos(COALESCE(error, ''), $2) > 0)`, f.Topic, f.ErrorContains)
	if err != nil {
		return 0, fmt.Errorf("requeue parked outbox rows: %w", err)
	}
	return tag.RowsAffected(), nil
}
