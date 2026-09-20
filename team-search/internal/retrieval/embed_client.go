package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EmbedClient defines the port for vectorizing texts into embeddings.
type EmbedClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// HTTPEmbedClient connects to platform-modelserve or TEI via HTTP.
type HTTPEmbedClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPEmbedClient constructs an HTTP embed client.
func NewHTTPEmbedClient(baseURL string, timeout time.Duration) *HTTPEmbedClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &HTTPEmbedClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type embedRequest struct {
	Texts []string `json:"texts"`
}

type openAIEmbedRequest struct {
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings,omitempty"`
	Data       []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data,omitempty"`
}

// Embed computes dense embedding vector for a single text string.
func (c *HTTPEmbedClient) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, errors.New("empty embedding vector returned")
	}
	return vecs[0], nil
}

// EmbedBatch vectorizes a batch of text strings.
func (c *HTTPEmbedClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	payload, err := json.Marshal(embedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/embed", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http embed call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed call failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	var parsed embedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}

	if len(parsed.Embeddings) > 0 {
		return parsed.Embeddings, nil
	}

	if len(parsed.Data) > 0 {
		out := make([][]float32, len(parsed.Data))
		for i, d := range parsed.Data {
			out[i] = d.Embedding
		}
		return out, nil
	}

	return nil, errors.New("embed response contained neither 'embeddings' nor 'data' fields")
}

// MockEmbedClient produces deterministic mock vectors for testing.
type MockEmbedClient struct {
	Dim int
	Err error
}

func (m *MockEmbedClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	dim := m.Dim
	if dim <= 0 {
		dim = 384
	}
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = float32(len(text)%10+i) / float32(dim)
	}
	return vec, nil
}

func (m *MockEmbedClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := m.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
