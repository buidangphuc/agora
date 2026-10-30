SERVICE := team-search
GRPC_PORT := 50052
BINARIES := server indexer
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: indexer
indexer:
	go run ./cmd/indexer
