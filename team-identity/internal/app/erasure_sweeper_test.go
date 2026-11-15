package app

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

// Scenario: identity.account-deletion-due-account-is-erased (sweeper wired in cmd/app, runs at start)
func TestErasureSweeperRunsAtStart(t *testing.T) {
	url := migratedURL(t)
	execSQL(t, url, `INSERT INTO users (id, username, password_hash, roles, deletion_requested_at, erase_after) VALUES ('u-erase', 'former-user', 'h', '{buyer}', now() - interval '49 hours', now() - interval '1 hour')`)
	execSQL(t, url, `INSERT INTO users (id, username, password_hash, roles, deletion_requested_at, erase_after) VALUES ('u-wait', 'waiting-user', 'h', '{buyer}', now(), now() + interval '48 hours')`)
	s := newSettings(t, url, deadBroker)
	s.Upstream.OrderAddr = "127.0.0.1:1"
	s.Upstream.PaymentAddr = "127.0.0.1:1"
	s.AccountDeletion.CancelWindow = 48 * time.Hour
	s.AccountDeletion.SweepInterval = time.Hour
	r := start(t, s)
	waitLive(t, r.conn, r.done)

	c := dial(t, url)
	testkit.Eventually(t, 5*time.Second, func() bool {
		var st string
		if err := c.QueryRow(context.Background(), `SELECT status FROM users WHERE id = 'u-erase'`).Scan(&st); err != nil {
			return false
		}
		return st == "deleted"
	})
	if n := queryInt(t, url, `SELECT count(*) FROM identity_outbox_events WHERE event_type = 'platform.identity.v1.UserErased' AND aggregate_id = 'u-erase'`); n != 1 {
		t.Fatalf("UserErased rows for erased user = %d, want 1", n)
	}
	if st := queryString(t, url, `SELECT status FROM users WHERE id = 'u-wait'`); st == "deleted" {
		t.Fatal("not-yet-due account was erased")
	}
}
