SERVICE := team-payment
GRPC_PORT := 50056
BINARIES := server
LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml
include ../platform-core/make/go-service.mk

.PHONY: tidy
tidy:
	go mod tidy
