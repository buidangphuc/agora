# syntax=docker/dockerfile:1
#
# team-analytics is a Kafka-consumer WORKER (no serving surface but a health
# port). The DuckDB adapter uses CGO, so this image is glibc-based (not the
# static distroless the pure-Go services use).
#
# Build flow (ADR-0001): generated proto is produced by `make proto` (buf) BEFORE
# `docker build`, since buf is not on the host and generated/ is gitignored.
# `go mod tidy` in-image resolves the (BigQuery-heavy) dependency graph + go.sum
# on first build.
FROM golang:1.23 AS build
WORKDIR /src
COPY . .
ENV CGO_ENABLED=1 GOOS=linux
RUN go mod tidy
RUN go build -trimpath -o /out/consumer ./cmd/consumer

FROM gcr.io/distroless/cc-debian12:nonroot
WORKDIR /
COPY --from=build /out/consumer /consumer
ENV GRPC_PORT=50059
EXPOSE 50059
USER nonroot:nonroot
ENTRYPOINT ["/consumer"]
