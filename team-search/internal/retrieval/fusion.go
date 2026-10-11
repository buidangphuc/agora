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
	// Stock is carried from whichever leg returned the hit (D3); nil = unknown.
	Stock *int32
	// Text is the rerank document (title + description); empty when unknown.
	Text string
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
	stock := make(map[string]*int32)
	text := make(map[string]string)

	// Visit strategies in a fixed order: ranging over the map would make provenance and the
	// first-seen text/stock depend on Go's random map iteration.
	strategies := make([]string, 0, len(strategyCandidates))
	for strategy := range strategyCandidates {
		strategies = append(strategies, strategy)
	}
	sort.Strings(strategies)

	for _, strategy := range strategies {
		candidates := strategyCandidates[strategy]
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
			if stock[c.ListingID] == nil && c.Stock != nil {
				stock[c.ListingID] = c.Stock
			}
			if text[c.ListingID] == "" {
				text[c.ListingID] = c.Text
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
			Stock:     stock[id],
			Text:      text[id],
		})
	}

	// Sort descending by fused RRF score; equal scores are ordered by listing id so the same query
	// always yields the same order (stable paging, reproducible ties).
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].ListingID < merged[j].ListingID
	})

	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged
}
