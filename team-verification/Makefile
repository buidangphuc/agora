.PHONY: proto build test tidy run

# Regenerate Go stubs from the vendored proto into ./generated.
proto:
	buf generate

build:
	go build ./...

test:
	go test ./...

tidy:
	go mod tidy

run:
	go run ./cmd/server
