package edge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Fixed Jaeger query: the browser supplies none of it (Rule 2, thin proxy).
const (
	jaegerService  = "team-gateway"
	jaegerLookback = "1h"
	jaegerLimit    = "5"
)

// jaegerTracesResponse is the subset of the Jaeger query API
// (GET /api/traces) the cockpit reads. Times are microseconds.
type jaegerTracesResponse struct {
	Data []struct {
		TraceID string `json:"traceID"`
		Spans   []struct {
			SpanID        string `json:"spanID"`
			OperationName string `json:"operationName"`
			StartTime     int64  `json:"startTime"`
			Duration      int64  `json:"duration"`
			References    []struct {
				RefType string `json:"refType"`
				SpanID  string `json:"spanID"`
			} `json:"references"`
		} `json:"spans"`
	} `json:"data"`
}

// recentTraces runs the fixed Jaeger query and shapes each trace into a
// TraceSummary, newest first. It returns nil when JAEGER_QUERY_URL is unset or
// Jaeger is unreachable/misbehaving, so the caller keeps an empty list.
func (h *CockpitHandler) recentTraces(ctx context.Context) []TraceSummary {
	if h.cfg.JaegerQueryURL == "" {
		return nil
	}
	q := url.Values{"service": {jaegerService}, "lookback": {jaegerLookback}, "limit": {jaegerLimit}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(h.cfg.JaegerQueryURL, "/")+"/api/traces?"+q.Encode(), nil)
	if err != nil {
		return nil
	}
	res, err := h.http.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var body jaegerTracesResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil
	}

	uiBase := strings.TrimRight(h.cfg.JaegerUIURL, "/")
	out := make([]TraceSummary, 0, len(body.Data))
	startUs := make([]int64, 0, len(body.Data))
	for _, t := range body.Data {
		if t.TraceID == "" || len(t.Spans) == 0 {
			continue
		}
		ids := make(map[string]bool, len(t.Spans))
		for _, sp := range t.Spans {
			ids[sp.SpanID] = true
		}
		first, last := t.Spans[0].StartTime, t.Spans[0].StartTime+t.Spans[0].Duration
		root := 0
		rootFound := false
		for i, sp := range t.Spans {
			if sp.StartTime < first {
				first = sp.StartTime
			}
			if end := sp.StartTime + sp.Duration; end > last {
				last = end
			}
			// The root is the span whose parent is not in the trace; prefer the
			// earliest such span.
			isRoot := true
			for _, ref := range sp.References {
				if ids[ref.SpanID] {
					isRoot = false
				}
			}
			if isRoot && (!rootFound || sp.StartTime < t.Spans[root].StartTime) {
				root, rootFound = i, true
			}
		}
		ts := TraceSummary{
			TraceID:    t.TraceID,
			Operation:  t.Spans[root].OperationName,
			SpanCount:  len(t.Spans),
			DurationMs: float64(last-first) / 1000,
			StartedAt:  time.UnixMicro(first).UTC().Format(time.RFC3339),
		}
		if uiBase != "" {
			ts.JaegerURL = uiBase + "/trace/" + url.PathEscape(t.TraceID)
		}
		out = append(out, ts)
		startUs = append(startUs, first)
	}
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return startUs[idx[a]] > startUs[idx[b]] })
	sorted := make([]TraceSummary, len(out))
	for i, j := range idx {
		sorted[i] = out[j]
	}
	return sorted
}
