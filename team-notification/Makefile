SERVICE := team-notification
GRPC_PORT := 50058
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: tidy
tidy:
	go mod tidy
