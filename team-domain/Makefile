SERVICE := team-domain
GRPC_PORT := 50051
BINARIES := server
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: dev stop
dev:
	docker compose -f docker-compose.local.yaml up -d postgres-listing

stop:
	docker compose -f docker-compose.local.yaml down
