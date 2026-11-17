package objects

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("object not found")

type Info struct {
	Size        int64
	ContentType string
}

type Store interface {
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Stat(ctx context.Context, key string) (Info, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}
