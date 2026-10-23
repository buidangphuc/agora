SERVICE := team-audit
GRPC_PORT := 50066
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: tidy
tidy:
	go mod tidy
