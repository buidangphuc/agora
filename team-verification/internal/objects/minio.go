package objects

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/buidangphuc/team-verification/internal/config"
)

type Minio struct {
	cfg    config.Storage
	host   string
	scheme string
	http   *http.Client
	now    func() time.Time
}

func NewMinio(cfg config.Storage) (*Minio, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("storage endpoint and bucket are required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	host := strings.TrimRight(cfg.Endpoint, "/")
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("storage endpoint: %w", err)
		}
		host = u.Host
		scheme = u.Scheme
	}
	return &Minio{cfg: cfg, host: host, scheme: scheme, http: &http.Client{Timeout: 30 * time.Second}, now: time.Now}, nil
}

func (m *Minio) PresignPut(_ context.Context, key, contentType string, size int64, ttl time.Duration) (string, error) {
	return m.presign(http.MethodPut, "/"+m.cfg.Bucket+"/"+escapeKey(key), nil, map[string]string{
		"content-length": strconv.FormatInt(size, 10),
		"content-type":   contentType,
	}, ttl), nil
}

func (m *Minio) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodGet, "/"+m.cfg.Bucket+"/"+escapeKey(key), nil, nil, ttl), nil
}

func (m *Minio) Stat(ctx context.Context, key string) (Info, error) {
	resp, err := m.do(ctx, http.MethodHead, "/"+m.cfg.Bucket+"/"+escapeKey(key), nil)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Info{}, ErrNotFound
	case resp.StatusCode/100 != 2:
		return Info{}, fmt.Errorf("stat %s: status %d", key, resp.StatusCode)
	}
	return Info{Size: resp.ContentLength, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (m *Minio) Delete(ctx context.Context, key string) error {
	resp, err := m.do(ctx, http.MethodDelete, "/"+m.cfg.Bucket+"/"+escapeKey(key), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete %s: status %d", key, resp.StatusCode)
	}
	return nil
}

type listResult struct {
	Keys      []string `xml:"Contents>Key"`
	Truncated bool     `xml:"IsTruncated"`
	NextToken string   `xml:"NextContinuationToken"`
}

func (m *Minio) DeletePrefix(ctx context.Context, prefix string) error {
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := m.do(ctx, http.MethodGet, "/"+m.cfg.Bucket, q)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("list %s: status %d", prefix, resp.StatusCode)
		}
		var res listResult
		if err := xml.Unmarshal(body, &res); err != nil {
			return fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, k := range res.Keys {
			if err := m.Delete(ctx, k); err != nil {
				return err
			}
		}
		if !res.Truncated || res.NextToken == "" {
			return nil
		}
		token = res.NextToken
	}
}

func (m *Minio) do(ctx context.Context, method, path string, q url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, m.presign(method, path, q, nil, time.Minute), nil)
	if err != nil {
		return nil, err
	}
	return m.http.Do(req)
}

func (m *Minio) presign(method, path string, extra url.Values, headers map[string]string, ttl time.Duration) string {
	now := m.now().UTC()
	date := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	names := []string{"host"}
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	signed := strings.Join(names, ";")

	query := url.Values{}
	for k, v := range extra {
		query[k] = v
	}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", fmt.Sprintf("%s/%s/%s/s3/aws4_request", m.cfg.AccessKey, date, m.cfg.Region))
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	query.Set("X-Amz-SignedHeaders", signed)
	canonicalQuery := strings.ReplaceAll(query.Encode(), "+", "%20")

	var ch strings.Builder
	for _, n := range names {
		v := m.host
		if n != "host" {
			v = headers[n]
		}
		ch.WriteString(n + ":" + strings.TrimSpace(v) + "\n")
	}
	canonical := strings.Join([]string{method, path, canonicalQuery, ch.String(), signed, "UNSIGNED-PAYLOAD"}, "\n")
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", date, m.cfg.Region)
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))

	k := hmacSHA256([]byte("AWS4"+m.cfg.SecretKey), []byte(date))
	k = hmacSHA256(k, []byte(m.cfg.Region))
	k = hmacSHA256(k, []byte("s3"))
	k = hmacSHA256(k, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(k, []byte(toSign)))

	return fmt.Sprintf("%s://%s%s?%s&X-Amz-Signature=%s", m.scheme, m.host, path, canonicalQuery, sig)
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
