package handler_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/repository"
)

// Owner isolation through the handler, against the in-memory repository always
// and a real Postgres when SEARCH_TEST_DATABASE_URL is set (the schema is applied
// by `make test-integration` or by the repository test; the statement below is
// the same idempotent migration).
func isolationRepos(t *testing.T) map[string]repository.SavedSearchRepository {
	t.Helper()
	out := map[string]repository.SavedSearchRepository{"memory": repository.NewInMemorySavedSearchRepository()}
	if url := os.Getenv("SEARCH_TEST_DATABASE_URL"); url != "" {
		db, err := sql.Open("pgx", url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mig, err := os.ReadFile("../../migrations/0001_saved_searches.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(mig)); err != nil {
			t.Fatalf("apply migration: %v", err)
		}
		out["postgres"] = repository.NewPostgresSavedSearchRepository(db)
	}
	return out
}

func TestSavedSearchOwnerIsolation(t *testing.T) {
	rw := []string{"search:read", "search:write"}
	for name, repo := range isolationRepos(t) {
		repo := repo
		t.Run(name, func(t *testing.T) {
			mi := &mockIndex{}
			h := handler.NewSearchHandler(mi, repo)
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
