package docs

import (
	"os"
	"strings"
	"testing"
)

func rowContaining(t *testing.T, prefix string) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("no README row starting with %q", prefix)
	return ""
}

// task 3: README Consume row for engagement.events lists ReviewHidden / review_hidden.
func TestTask3_ReadmeConsumeRowListsReviewHidden(t *testing.T) {
	row := rowContaining(t, "| Consume | `engagement.events`")
	for _, want := range []string{"ReviewHidden", "review_hidden"} {
		if !strings.Contains(row, want) {
			t.Errorf("Consume row missing %q", want)
		}
	}
}

// task 3: engagement_facts row lists review_id column.
func TestTask3_ReadmeEngagementFactsRowListsReviewID(t *testing.T) {
	row := rowContaining(t, "| `engagement_facts`")
	if !strings.Contains(row, "review_id") {
		t.Errorf("engagement_facts row missing review_id")
	}
}
