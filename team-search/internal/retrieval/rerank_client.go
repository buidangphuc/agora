package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// RerankClient defines the port for cross-encoder reranking.
type RerankClient interface {
	Rerank(ctx context.Context, query string, docs []RerankDoc) ([]string, error)
}

// RerankDoc is one candidate: its listing ID and the text the cross-encoder scores.
type RerankDoc struct {
	ID   string
	Text string
}

// HTTPRerankClient calls platform-modelserve /rerank.
type HTTPRerankClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPRerankClient constructs an HTTP rerank client.
func NewHTTPRerankClient(baseURL string, timeout time.Duration) *HTTPRerankClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &HTTPRerankClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type rerankRequest struct {
	Query string   `json:"query"`
	Texts []string `json:"texts"`
}

type rerankResultItem struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

type rerankResponse struct {
	Results []rerankResultItem `json:"results"`
}

// parseRerankResponse accepts TEI's bare list [{"index":i,"score":s}] and the
// {"results":[...]} wrapper.
func parseRerankResponse(body []byte) ([]rerankResultItem, error) {
	var bare []rerankResultItem
	if err := json.Unmarshal(body, &bare); err == nil {
		return bare, nil
	}
	var wrapped rerankResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Results, nil
}

// Rerank sends the candidates' text to the cross-encoder and returns the IDs
// re-ordered by score (highest first), mapping scores back by index.
func (c *HTTPRerankClient) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]string, error) {
	ids := make([]string, len(docs))
	texts := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
		texts[i] = d.Text
	}
	if len(docs) <= 1 {
		return ids, nil
	}

	payload, err := json.Marshal(rerankRequest{Query: query, Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rerank", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http rerank call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank failed with status %d: %s", resp.StatusCode, string(body))
	}

	results, err := parseRerankResponse(body)
	if err != nil {
		return nil, fmt.Errorf("unmarshal rerank response: %w", err)
	}
	if len(results) == 0 {
		return ids, nil
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })

	reordered := make([]string, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, item := range results {
		if item.Index < 0 || item.Index >= len(ids) {
			continue
		}
		if _, dup := seen[item.Index]; dup {
			continue
		}
		seen[item.Index] = struct{}{}
		reordered = append(reordered, ids[item.Index])
	}
	for i, id := range ids {
		if _, ok := seen[i]; !ok {
			reordered = append(reordered, id)
		}
	}
	return reordered, nil
}
