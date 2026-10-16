package repository_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/repository"
)

func at(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

// Seed rows in chronological order (the in-memory repo orders newest-first by
// reverse insertion). e3 and e4 share a timestamp: id DESC breaks the tie.
var filterSeed = []*auditv1.AuditEvent{
	{Id: "e1", ActorId: "adm-1", Action: "kyc.review", TargetType: "order", TargetId: "o-1", CreatedAt: timestamppb.New(at(10))},
	{Id: "e2", ActorId: "adm-1", Action: "kyc.reviewed", TargetType: "order", TargetId: "o-1", CreatedAt: timestamppb.New(at(11))},
	{Id: "e3", ActorId: "adm-2", Action: "review.hide", TargetType: "order", TargetId: "o-2", CreatedAt: timestamppb.New(at(12))},
	{Id: "e4", ActorId: "adm-1", Action: "order.force_fail", TargetType: "order", TargetId: "o-1", CreatedAt: timestamppb.New(at(12))},
	{Id: "e5", ActorId: "adm-1", Action: "kyc.review", TargetType: "listing", TargetId: "l-1", CreatedAt: timestamppb.New(at(13))},
}

type filterCase struct {
	name      string
	f         repository.QueryFilter
	wantIDs   []string
	wantTotal int64
}

var filterCases = []filterCase{
	{"no filter", repository.QueryFilter{}, []string{"e5", "e4", "e3", "e2", "e1"}, 5},
	{"target type and id narrow to one target",
		repository.QueryFilter{TargetType: "order", TargetID: "o-1"}, []string{"e4", "e2", "e1"}, 3},
	{"target id alone", repository.QueryFilter{TargetID: "o-2"}, []string{"e3"}, 1},
	{"action exact match excludes prefix and suffix variants",
		repository.QueryFilter{Action: "kyc.review"}, []string{"e5", "e1"}, 2},
	{"filters combine with AND",
		repository.QueryFilter{ActorID: "adm-1", Action: "order.force_fail", TargetID: "o-1"}, []string{"e4"}, 1},
	{"AND leaves nothing when one filter misses",
		repository.QueryFilter{ActorID: "adm-2", Action: "order.force_fail", TargetID: "o-1"}, nil, 0},
	{"lower bound inclusive, upper bound exclusive",
		repository.QueryFilter{CreatedAfter: at(10), CreatedBefore: at(12)}, []string{"e2", "e1"}, 2},
	{"only created_after", repository.QueryFilter{CreatedAfter: at(12)}, []string{"e5", "e4", "e3"}, 3},
	{"only created_before is exclusive", repository.QueryFilter{CreatedBefore: at(11)}, []string{"e1"}, 1},
	{"equal bounds match nothing", repository.QueryFilter{CreatedAfter: at(11), CreatedBefore: at(11)}, nil, 0},
	{"time range combined with action",
		repository.QueryFilter{Action: "kyc.review", CreatedAfter: at(11)}, []string{"e5"}, 1},
	{"filter values are bound parameters (action)",
		repository.QueryFilter{Action: "x' OR '1'='1"}, nil, 0},
	{"filter values are bound parameters (target id)",
		repository.QueryFilter{TargetID: "o-1' OR '1'='1"}, nil, 0},
	{"no match", repository.QueryFilter{TargetID: "nope"}, nil, 0},
	{"total ignores limit and offset",
		repository.QueryFilter{Action: "kyc.review", Limit: 1, Offset: 1}, []string{"e1"}, 2},
	{"filtered second page",
		repository.QueryFilter{TargetType: "order", TargetID: "o-1", Limit: 2, Offset: 2}, []string{"e1"}, 3},
}

func runFilterCases(t *testing.T, repo repository.AuditRepository) {
	t.Helper()
	ctx := context.Background()
	for _, ev := range filterSeed {
		cp := proto.Clone(ev).(*auditv1.AuditEvent)
		cp.Metadata = map[string]string{}
		if _, err := repo.Append(ctx, cp); err != nil {
			t.Fatalf("seed %s: %v", ev.Id, err)
		}
	}
	for _, c := range filterCases {
		t.Run(c.name, func(t *testing.T) {
			evs, total, err := repo.Query(ctx, c.f)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			got := []string{}
			for _, e := range evs {
				got = append(got, e.GetId())
			}
			want := c.wantIDs
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ids = %v, want %v", got, want)
			}
			if total != c.wantTotal {
				t.Fatalf("total = %d, want %d", total, c.wantTotal)
			}
		})
	}
}

// Scenarios: Target id and target type narrow to one target; Action filter is an
// exact match; Filters combine with AND; Time range includes the lower bound and
// excludes the upper bound; Filter values are bound parameters.
func TestRepositoryFilterInMemory(t *testing.T) {
	runFilterCases(t, repository.NewInMemoryAuditRepo())
}

// Scenario: Audit query: In-memory repository matches Postgres. The Postgres half runs only
// when TEST_DATABASE_URL is set and must produce the same ids and totals as
// the same table does in memory. Rows are inserted with explicit created_at
// because Append owns created_at via NOW().
func TestRepositoryFilterParity(t *testing.T) {
	pool := pgPool(t)
	applyMigration(t, pool, "001_*.up.sql")
	applyMigration(t, pool, "002_*.up.sql")
	ctx := context.Background()
	for _, ev := range filterSeed {
		if _, err := pool.Exec(ctx,
			`INSERT INTO audit_events (id, actor_id, action, target_type, target_id, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			ev.Id, ev.ActorId, ev.Action, ev.TargetType, ev.TargetId, ev.CreatedAt.AsTime()); err != nil {
			t.Fatalf("seed %s: %v", ev.Id, err)
		}
	}
	repo := repository.NewPostgresAuditRepo(pool)
	for _, c := range filterCases {
		t.Run(c.name, func(t *testing.T) {
			evs, total, err := repo.Query(ctx, c.f)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			got := []string{}
			for _, e := range evs {
				got = append(got, e.GetId())
			}
			want := c.wantIDs
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ids = %v, want %v", got, want)
			}
			if total != c.wantTotal {
				t.Fatalf("total = %d, want %d", total, c.wantTotal)
			}
		})
	}
}
