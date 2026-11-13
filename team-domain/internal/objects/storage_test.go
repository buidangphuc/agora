package objects_test

import (
	"context"
	"strings"
	"testing"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/objects"
)

func TestPublicBaseUsesConfiguredBaseWithoutTrailingSlash(t *testing.T) {
	got := objects.PublicBase(config.Storage{PublicBaseURL: "https://cdn.example/media/", Endpoint: "minio:9000", Bucket: "b"})
	if got != "https://cdn.example/media" {
		t.Fatalf("PublicBase = %q", got)
	}
}

func TestPublicBaseFallsBackToSchemeEndpointBucket(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Storage
		want string
	}{
		{"plain http", config.Storage{Endpoint: "minio:9000", Bucket: "b"}, "http://minio:9000/b"},
		{"ssl", config.Storage{Endpoint: "minio:9000", Bucket: "b", UseSSL: true}, "https://minio:9000/b"},
		{"endpoint with scheme and slash", config.Storage{Endpoint: "http://minio:9000/", Bucket: "b"}, "http://minio:9000/b"},
	}
	for _, c := range cases {
		if got := objects.PublicBase(c.cfg); got != c.want {
			t.Errorf("%s: PublicBase = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPublicBaseMatchesPresignedPublicURL(t *testing.T) {
	cfgs := []config.Storage{
		{Endpoint: "minio:9000", Bucket: "b", AccessKey: "k", SecretKey: "s", PublicBaseURL: "http://localhost:9000/b/"},
		{Endpoint: "minio:9000", Bucket: "b", AccessKey: "k", SecretKey: "s"},
		{Endpoint: "https://s3.example:443", Bucket: "b", AccessKey: "k", SecretKey: "s", UseSSL: true},
	}
	for _, cfg := range cfgs {
		_, _, pub, err := objects.NewS3Storage(cfg).GetPresignedUploadURL(context.Background(), "image/png", "a.png")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(pub, objects.PublicBase(cfg)+"/") {
			t.Errorf("public url %q not under PublicBase %q", pub, objects.PublicBase(cfg))
		}
	}
}
