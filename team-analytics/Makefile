SERVICE := team-analytics
GRPC_PORT := 50059
BINARIES := consumer ga4-relay ga4import
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: consumer ga4-relay
consumer:
	go run ./cmd/consumer

ga4-relay:
	go run ./cmd/ga4-relay
