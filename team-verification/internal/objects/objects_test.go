package objects_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/internal/objects"
)

var (
	_ objects.Store = (*objects.Minio)(nil)
	_ objects.Store = (*objects.Fake)(nil)
)

const (
	minioImage = "cgr.dev/chainguard/minio@sha256:cee9a8e35885e93f14b3cb32960e5017aff74e2adfb52ba87c65003b0163886b"
	bucket     = "kyc-documents"
	rootUser   = "minioadmin"
	rootPass   = "minioadmin"
	maxBytes   = 5 * 1024 * 1024
)

func storageCfg(endpoint string) config.Storage {
	return config.Storage{Endpoint: endpoint, Bucket: bucket, AccessKey: rootUser, SecretKey: rootPass, Region: "us-east-1"}
}

func startMinio(t *testing.T) *objects.Minio {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        minioImage,
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			Env:          map[string]string{"MINIO_ROOT_USER": rootUser, "MINIO_ROOT_PASSWORD": rootPass},
			WaitingFor: wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp").
				WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	if ctr != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	}
	if err != nil {
		if os.Getenv("GO_SDK_REQUIRE_DOCKER") == "" {
			t.Skipf("docker unavailable: %v", err)
		}
		t.Fatalf("start minio: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for _, args := range [][]string{
		{"mc", "alias", "set", "local", "http://localhost:9000", rootUser, rootPass},
		{"mc", "mb", "--ignore-existing", "local/" + bucket},
		{"mc", "anonymous", "set", "none", "local/" + bucket},
	} {
		for {
			code, out, err := ctr.Exec(ctx, args)
			if err == nil && code == 0 {
				break
			}
			if time.Now().After(deadline) {
				var b []byte
				if out != nil {
					b, _ = io.ReadAll(out)
				}
				t.Fatalf("%v: code=%d err=%v out=%s", args, code, err, b)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	endpoint, err := ctr.PortEndpoint(ctx, "9000/tcp", "")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	m, err := objects.NewMinio(storageCfg(endpoint))
	if err != nil {
		t.Fatalf("NewMinio: %v", err)
	}
	return m
}

func put(ctx context.Context, t *testing.T, rawURL, contentType string, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func get(t *testing.T, rawURL string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(rawURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestPresignPutSignsContentTypeAndLength(t *testing.T) {
	const sc = "scenario Upload that differs from the signature is refused by storage: signed headers"
	m, err := objects.NewMinio(storageCfg("localhost:9000"))
	if err != nil {
		t.Fatalf("NewMinio: %v", err)
	}
	raw, err := m.PresignPut(context.Background(), "kyc/u-1/abc", "image/jpeg", 1048576, 15*time.Minute)
	if err != nil {
		t.Fatalf("%s: PresignPut: %v", sc, err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(u.Path, "/"+bucket+"/kyc/u-1/abc") {
		t.Errorf("%s: path = %q", sc, u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Expires") != "900" {
		t.Errorf("%s: X-Amz-Expires = %q, want 900", sc, q.Get("X-Amz-Expires"))
	}
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	for _, h := range []string{"content-type", "content-length", "host"} {
		found := false
		for _, s := range signed {
			found = found || s == h
		}
		if !found {
			t.Errorf("%s: %s not in signed headers %v", sc, h, signed)
		}
	}
}

func TestPresignedURLsUseThePublicEndpoint(t *testing.T) {
	const sc = "scenario Seller receives an upload URL: presigned URLs are reachable from the browser"
	cfg := storageCfg("minio:9000")
	cfg.PublicEndpoint = "http://localhost:39000"
	m, err := objects.NewMinio(cfg)
	if err != nil {
		t.Fatalf("NewMinio: %v", err)
	}
	put, err := m.PresignPut(context.Background(), "kyc/u-1/abc", "image/jpeg", 10, 15*time.Minute)
	if err != nil {
		t.Fatalf("%s: PresignPut: %v", sc, err)
	}
	get, err := m.PresignGet(context.Background(), "kyc/u-1/abc", 5*time.Minute)
	if err != nil {
		t.Fatalf("%s: PresignGet: %v", sc, err)
	}
	for _, raw := range []string{put, get} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if u.Scheme != "http" || u.Host != "localhost:39000" {
			t.Errorf("%s: URL %q not on the public endpoint", sc, raw)
		}
	}
}

func TestPresignGetExpiresInFiveMinutes(t *testing.T) {
	const sc = "scenario Admin opens a document: presigned GET lasts 5 minutes"
	m, err := objects.NewMinio(storageCfg("localhost:9000"))
	if err != nil {
		t.Fatalf("NewMinio: %v", err)
	}
	raw, err := m.PresignGet(context.Background(), "kyc/u-1/abc", 5*time.Minute)
	if err != nil {
		t.Fatalf("%s: %v", sc, err)
	}
	u, _ := url.Parse(raw)
	if got := u.Query().Get("X-Amz-Expires"); got != "300" {
		t.Errorf("%s: X-Amz-Expires = %q, want 300", sc, got)
	}
}

func TestMinioStorageBehaviour(t *testing.T) {
	m := startMinio(t)
	ctx := context.Background()

	t.Run("scenario Seller receives an upload URL: exact PUT succeeds and is stat-able", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), 1048576)
		raw, err := m.PresignPut(ctx, "kyc/u-1/ok", "image/jpeg", int64(len(body)), 15*time.Minute)
		if err != nil {
			t.Fatalf("PresignPut: %v", err)
		}
		if code := put(ctx, t, raw, "image/jpeg", body); code < 200 || code > 299 {
			t.Fatalf("PUT status = %d, want 2xx", code)
		}
		info, err := m.Stat(ctx, "kyc/u-1/ok")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Size != 1048576 || info.ContentType != "image/jpeg" {
			t.Errorf("Stat = %+v", info)
		}
	})

	t.Run("scenario Upload that differs from the signature is refused by storage: content type", func(t *testing.T) {
		raw, err := m.PresignPut(ctx, "kyc/u-1/badtype", "image/jpeg", 10, 15*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if code := put(ctx, t, raw, "image/png", bytes.Repeat([]byte("a"), 10)); code != http.StatusForbidden {
			t.Errorf("PUT status = %d, want 403", code)
		}
		if _, err := m.Stat(ctx, "kyc/u-1/badtype"); !errors.Is(err, objects.ErrNotFound) {
			t.Errorf("Stat err = %v, want ErrNotFound", err)
		}
	})

	t.Run("scenario Upload that differs from the signature is refused by storage: length", func(t *testing.T) {
		raw, err := m.PresignPut(ctx, "kyc/u-1/badlen", "image/jpeg", 10, 15*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if code := put(ctx, t, raw, "image/jpeg", bytes.Repeat([]byte("a"), 11)); code != http.StatusForbidden {
			t.Errorf("PUT status = %d, want 403", code)
		}
		if _, err := m.Stat(ctx, "kyc/u-1/badlen"); !errors.Is(err, objects.ErrNotFound) {
			t.Errorf("Stat err = %v, want ErrNotFound", err)
		}
	})

	t.Run("scenario Maximum size is accepted", func(t *testing.T) {
		raw, err := m.PresignPut(ctx, "kyc/u-1/max", "application/pdf", maxBytes, 15*time.Minute)
		if err != nil {
			t.Fatalf("PresignPut: %v", err)
		}
		if code := put(ctx, t, raw, "application/pdf", bytes.Repeat([]byte("p"), maxBytes)); code < 200 || code > 299 {
			t.Fatalf("PUT status = %d, want 2xx", code)
		}
		info, err := m.Stat(ctx, "kyc/u-1/max")
		if err != nil || info.Size != maxBytes {
			t.Fatalf("Stat = %+v, %v", info, err)
		}
	})

	t.Run("scenario Bucket is not publicly readable: unsigned GET is refused, signed GET works", func(t *testing.T) {
		base, _ := url.Parse(mustPresignGet(t, m, "kyc/u-1/ok"))
		unsigned := *base
		unsigned.RawQuery = ""
		if code, _ := get(t, unsigned.String()); code != http.StatusForbidden {
			t.Errorf("unsigned GET status = %d, want 403", code)
		}
		code, body := get(t, base.String())
		if code != http.StatusOK || len(body) != 1048576 {
			t.Errorf("signed GET status = %d len = %d", code, len(body))
		}
	})

	t.Run("scenario Stat of a missing key reports ErrNotFound", func(t *testing.T) {
		if _, err := m.Stat(ctx, "kyc/u-1/missing"); !errors.Is(err, objects.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("scenario Delete removes one object and tolerates a missing one", func(t *testing.T) {
		if err := m.Delete(ctx, "kyc/u-1/badtype"); err != nil {
			t.Errorf("delete missing: %v", err)
		}
		if err := m.Delete(ctx, "kyc/u-1/ok"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := m.Stat(ctx, "kyc/u-1/ok"); !errors.Is(err, objects.ErrNotFound) {
			t.Errorf("Stat after delete err = %v", err)
		}
	})

	t.Run("scenario UserErased removes the user's KYC data: delete by prefix leaves other users", func(t *testing.T) {
		for _, k := range []string{"kyc/u-2/a", "kyc/u-2/b", "kyc/u-22/a"} {
			raw, err := m.PresignPut(ctx, k, "image/png", 3, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if code := put(ctx, t, raw, "image/png", []byte("abc")); code != http.StatusOK {
				t.Fatalf("PUT %s = %d", k, code)
			}
		}
		if err := m.DeletePrefix(ctx, "kyc/u-2/"); err != nil {
			t.Fatalf("DeletePrefix: %v", err)
		}
		for _, k := range []string{"kyc/u-2/a", "kyc/u-2/b"} {
			if _, err := m.Stat(ctx, k); !errors.Is(err, objects.ErrNotFound) {
				t.Errorf("%s still present: %v", k, err)
			}
		}
		if _, err := m.Stat(ctx, "kyc/u-22/a"); err != nil {
			t.Errorf("other user's object removed: %v", err)
		}
	})
}

func mustPresignGet(t *testing.T, m *objects.Minio, key string) string {
	t.Helper()
	raw, err := m.PresignGet(context.Background(), key, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFakeStore(t *testing.T) {
	const sc = "scenario Oversize or mismatching object is rejected and removed: fake store"
	ctx := context.Background()
	f := objects.NewFake()

	if _, err := f.Stat(ctx, "kyc/u/x"); !errors.Is(err, objects.ErrNotFound) {
		t.Fatalf("%s: Stat missing err = %v", sc, err)
	}
	f.Put("kyc/u/x", "image/png", 42)
	f.Put("kyc/u/y", "image/png", 7)
	f.Put("kyc/v/z", "image/png", 7)
	info, err := f.Stat(ctx, "kyc/u/x")
	if err != nil || info.Size != 42 || info.ContentType != "image/png" {
		t.Fatalf("%s: Stat = %+v, %v", sc, info, err)
	}
	if err := f.Delete(ctx, "kyc/u/x"); err != nil || f.Has("kyc/u/x") {
		t.Fatalf("%s: Delete err = %v has = %v", sc, err, f.Has("kyc/u/x"))
	}
	if err := f.Delete(ctx, "kyc/u/x"); err != nil {
		t.Fatalf("%s: deleting a missing key must not fail: %v", sc, err)
	}
	if err := f.DeletePrefix(ctx, "kyc/u/"); err != nil {
		t.Fatal(err)
	}
	if f.Has("kyc/u/y") || !f.Has("kyc/v/z") {
		t.Errorf("%s: DeletePrefix removed the wrong keys", sc)
	}
	raw, err := f.PresignPut(ctx, "kyc/u/q", "image/png", 3, time.Minute)
	if err != nil || raw == "" {
		t.Errorf("PresignPut = %q, %v", raw, err)
	}
	raw, err = f.PresignGet(ctx, "kyc/v/z", time.Minute)
	if err != nil || raw == "" {
		t.Errorf("PresignGet = %q, %v", raw, err)
	}
}

func TestFakeFailureInjection(t *testing.T) {
	const sc = "scenario Storage outage sends the event to retry then DLQ: fake outage"
	ctx := context.Background()
	f := objects.NewFake()
	f.Put("kyc/u/x", "image/png", 1)
	boom := errors.New("storage down")
	f.SetError(boom)
	if err := f.DeletePrefix(ctx, "kyc/u/"); !errors.Is(err, boom) {
		t.Errorf("%s: DeletePrefix err = %v", sc, err)
	}
	if err := f.Delete(ctx, "kyc/u/x"); !errors.Is(err, boom) {
		t.Errorf("%s: Delete err = %v", sc, err)
	}
	if !f.Has("kyc/u/x") {
		t.Errorf("%s: object removed despite the outage", sc)
	}
	f.SetError(nil)
	if err := f.DeletePrefix(ctx, "kyc/u/"); err != nil || f.Has("kyc/u/x") {
		t.Errorf("%s: recovery failed: %v", sc, err)
	}
}
