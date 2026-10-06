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
# An empty /data owned by nonroot: a fresh named volume mounted there inherits
# the ownership, so DuckDB (and the Parquet export) can write to it.
RUN mkdir -p /out/data

FROM gcr.io/distroless/cc-debian12:nonroot
WORKDIR /
COPY --from=build /out/consumer /consumer
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV GRPC_PORT=50059
EXPOSE 50059
USER nonroot:nonroot
ENTRYPOINT ["/consumer"]
