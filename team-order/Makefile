SERVICE := team-order
GRPC_PORT := 50055
BINARIES := server resync-commits
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: tidy
tidy:
	go mod tidy
