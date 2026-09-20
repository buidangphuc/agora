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
	Rerank(ctx context.Context, query string, candidateIDs []string) ([]string, error)
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

// Rerank sends candidates to cross-encoder and returns re-ordered candidate IDs.
func (c *HTTPRerankClient) Rerank(ctx context.Context, query string, candidateIDs []string) ([]string, error) {
	if len(candidateIDs) <= 1 {
		return candidateIDs, nil
	}

	payload, err := json.Marshal(rerankRequest{
		Query: query,
		Texts: candidateIDs, // In production, this can pass titles/texts; for ID re-scoring it uses candidate representations
	})
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rerank failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}

	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response: %w", err)
	}

	if len(parsed.Results) == 0 {
		return candidateIDs, nil
	}

	// Sort results by cross-encoder score desc
	sort.Slice(parsed.Results, func(i, j int) bool {
		return parsed.Results[i].Score > parsed.Results[j].Score
	})

	reordered := make([]string, 0, len(candidateIDs))
	seen := make(map[string]struct{}, len(candidateIDs))
	for _, item := range parsed.Results {
		if item.Index >= 0 && item.Index < len(candidateIDs) {
			id := candidateIDs[item.Index]
			reordered = append(reordered, id)
			seen[id] = struct{}{}
		}
	}
	// Append any missed candidate IDs
	for _, id := range candidateIDs {
		if _, ok := seen[id]; !ok {
			reordered = append(reordered, id)
		}
	}

	return reordered, nil
}
