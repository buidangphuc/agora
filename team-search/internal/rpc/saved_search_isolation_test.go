package rpc_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/rpc"
	"github.com/buidangphuc/team-search/internal/store"
	"github.com/buidangphuc/team-search/migrations"
)

func migratedPool(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(t.Context(), dsn, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return dsn, pool
}

func isolationRepos(t *testing.T) map[string]rpc.SavedSearches {
	t.Helper()
	_, pool := migratedPool(t)
	return map[string]rpc.SavedSearches{
		"fake":     newFakeSaved(),
		"postgres": store.NewPostgresSavedSearchRepository(pool),
	}
}

// Scenario: A saved search survives a restart (handler level): a new pool, store
// and handler over the same database still list and run what the old one saved.
func TestSavedSearchSurvivesRestart_ASavedSearchSurvivesARestart(t *testing.T) {
	dsn, pool := migratedPool(t)
	ctx := userCtx("buyer_1", "search:read", "search:write")
	first := newHandler(t, &mockIndex{}, store.NewPostgresSavedSearchRepository(pool))
	saved, err := first.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "red bike", FiltersJson: `{"city":"hanoi"}`})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	pool.Close()

	pool2, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool2.Close)
	mi := &mockIndex{}
	second := newHandler(t, mi, store.NewPostgresSavedSearchRepository(pool2))
	list, err := second.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
	if err != nil || list.GetPage().GetTotal() != 1 || list.GetSavedSearches()[0].GetId() != saved.GetSavedSearch().GetId() {
		t.Fatalf("list after restart = %+v err=%v", list, err)
	}
	if list.GetSavedSearches()[0].GetQuery() != "red bike" {
		t.Errorf("query lost: %q", list.GetSavedSearches()[0].GetQuery())
	}
	if _, err := second.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()}); err != nil {
		t.Fatalf("run after restart: %v", err)
	}
	if mi.searchCalls != 1 {
		t.Errorf("run executed %d queries, want 1", mi.searchCalls)
	}
}

// Scenario: Another user's saved search is not found.
func TestSavedSearchOwnerIsolation(t *testing.T) {
	rw := []string{"search:read", "search:write"}
	for name, repo := range isolationRepos(t) {
		repo := repo
		t.Run(name, func(t *testing.T) {
			mi := &mockIndex{}
			h := newHandler(t, mi, repo)
			// Unique ids so a shared database never mixes rows across runs.
			aID, bID := "A-"+uuid.NewString(), "B-"+uuid.NewString()
			a := principalCtx(aID, "user", rw...)
			b := principalCtx(bID, "user", rw...)

			saved, err := h.SaveSearch(a, &searchv1.SaveSearchRequest{Query: "villa", FiltersJson: `{"status":"published"}`})
			if err != nil {
				t.Fatal(err)
			}
			id := saved.GetSavedSearch().GetId()
			if _, err := h.SaveSearch(a, &searchv1.SaveSearchRequest{Query: "second"}); err != nil {
				t.Fatal(err)
			}

			// B lists: empty, total 0.
			bl, err := h.ListSavedSearches(b, &searchv1.ListSavedSearchesRequest{})
			if err != nil || bl.GetPage().GetTotal() != 0 || len(bl.GetSavedSearches()) != 0 {
				t.Fatalf("B list = %+v err=%v", bl, err)
			}
			// B runs A's id: NOT_FOUND and no query executed.
			if _, err := h.RunSavedSearch(b, &searchv1.RunSavedSearchRequest{Id: id}); status.Code(err) != codes.NotFound {
				t.Fatalf("B run: %v", err)
			}
			if mi.searchCalls != 0 {
				t.Fatalf("a foreign run executed %d queries", mi.searchCalls)
			}
			// B deletes A's id: NOT_FOUND and A's row remains.
			if _, err := h.DeleteSavedSearch(b, &searchv1.DeleteSavedSearchRequest{Id: id}); status.Code(err) != codes.NotFound {
				t.Fatalf("B delete: %v", err)
			}
			// An id that never existed answers exactly the same (ownership cannot be probed).
			if _, err := h.RunSavedSearch(b, &searchv1.RunSavedSearchRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
				t.Fatalf("unknown id run: %v", err)
			}
			// A still has both, and page.total counts only A's.
			al, err := h.ListSavedSearches(a, &searchv1.ListSavedSearchesRequest{})
			if err != nil || al.GetPage().GetTotal() != 2 || len(al.GetSavedSearches()) != 2 {
				t.Fatalf("A list = %+v err=%v", al, err)
			}
			// B saves one of their own: A's and B's totals stay independent.
			if _, err := h.SaveSearch(b, &searchv1.SaveSearchRequest{Query: "bob"}); err != nil {
				t.Fatal(err)
			}
			al, _ = h.ListSavedSearches(a, &searchv1.ListSavedSearchesRequest{})
			bl, _ = h.ListSavedSearches(b, &searchv1.ListSavedSearchesRequest{})
			if al.GetPage().GetTotal() != 2 || bl.GetPage().GetTotal() != 1 {
				t.Fatalf("totals A=%d B=%d, want 2 and 1", al.GetPage().GetTotal(), bl.GetPage().GetTotal())
			}
			// A can still run their own.
			if _, err := h.RunSavedSearch(a, &searchv1.RunSavedSearchRequest{Id: id}); err != nil {
				t.Fatalf("A run: %v", err)
			}
		})
	}
}
