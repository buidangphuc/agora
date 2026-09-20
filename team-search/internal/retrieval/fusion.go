package retrieval

import (
	"sort"
)

// Candidate represents a retrieved listing with its score and provenance.
type Candidate struct {
	ListingID string
	Score     float64
	Rank      int
	Strategy  string
}

// RRF merges ranked candidate lists using Reciprocal Rank Fusion:
// RRF(d) = sum_s ( w_s / (k + r_s(d)) )
// where r_s(d) is 1-based rank position in strategy s.
func RRF(strategyCandidates map[string][]Candidate, k int, weights map[string]float64) []Candidate {
	if k <= 0 {
		k = 60
	}

	scores := make(map[string]float64)
	orderSeen := make([]string, 0)
	provenance := make(map[string][]string)

	for strategy, candidates := range strategyCandidates {
		weight := 1.0
		if w, ok := weights[strategy]; ok && w > 0 {
			weight = w
		}

		for rank, c := range candidates {
			if c.ListingID == "" {
				continue
			}
			if _, seen := scores[c.ListingID]; !seen {
				orderSeen = append(orderSeen, c.ListingID)
			}
			// 1-based rank
			rrfScore := weight / float64(k+rank+1)
			scores[c.ListingID] += rrfScore
			provenance[c.ListingID] = append(provenance[c.ListingID], strategy)
		}
	}

	merged := make([]Candidate, 0, len(scores))
	for _, id := range orderSeen {
		merged = append(merged, Candidate{
			ListingID: id,
			Score:     scores[id],
			Strategy:  "rrf_fused",
		})
	}

	// Sort descending by fused RRF score, breaking ties stably by initial discovery order
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged
}
