package docs

import (
	"os"
	"strings"
	"testing"
)

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Scenario: Review hidden event: effective hide enqueues one fact.
func TestReadme_ListsReviewHiddenAmongPublishedFacts_EffectiveHideEnqueuesOneFact(t *testing.T) {
	for _, line := range strings.Split(readme(t), "\n") {
		if strings.Contains(line, "`ReviewCreated`") && strings.Contains(line, "`ReviewHidden`") {
			return
		}
	}
	t.Fatal("README facts list must include `ReviewHidden` next to `ReviewCreated`")
}

// Scenario: Review hidden event: effective hide enqueues one fact (deploy order note).
func TestReadme_NotesAnalyticsFirstDeployOrder_EffectiveHideEnqueuesOneFact(t *testing.T) {
	for _, line := range strings.Split(readme(t), "\n") {
		l := strings.ToLower(line)
		if strings.Contains(line, "ReviewHidden") && strings.Contains(l, "analytics") && strings.Contains(l, "before") {
			return
		}
	}
	t.Fatal("README must note that team-analytics deploys before team-engagement for ReviewHidden")
}
