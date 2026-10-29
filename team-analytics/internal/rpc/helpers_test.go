package rpc_test

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
)

func asPrincipal(id, ptype, scopes string) context.Context {
	var sc []string
	for _, s := range strings.Split(scopes, ",") {
		if s = strings.TrimSpace(s); s != "" {
			sc = append(sc, s)
		}
	}
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: auth.PrincipalType(ptype), Scopes: sc})
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func mustHandler(repo query.Repository, opts ...rpc.Option) *rpc.Handler {
	h, err := rpc.NewHandler(repo, discardLogger(), opts...)
	if err != nil {
		panic(err)
	}
	return h
}

func startServer(t *testing.T, cfg sdkconfig.GRPC, h *rpc.Handler) *grpc.ClientConn {
	t.Helper()
	logger := discardLogger()
	hl, err := healthx.New(time.Hour, logger)
	if err != nil {
		t.Fatalf("healthx.New: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: cfg, Logger: logger, Health: hl})
	if h != nil {
		rpc.Register(srv, h)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- grpcx.ServeListener(srv, lis, time.Second).Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
