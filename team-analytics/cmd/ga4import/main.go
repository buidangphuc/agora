package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-analytics/internal/ga4import"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return ga4import.RunCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
}
