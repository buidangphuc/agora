package names

import (
	"context"
	"log/slog"
)

type Resolver struct{}

func New(domainAddr, identityAddr string, logger *slog.Logger) (*Resolver, error) {
	return &Resolver{}, nil
}

func (r *Resolver) ResolveSenderName(ctx context.Context, senderID, sellerID string) string {
	return ""
}

func (r *Resolver) Close() {}
