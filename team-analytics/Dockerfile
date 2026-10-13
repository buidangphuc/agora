# syntax=docker/dockerfile:1
# Go service image (ADR-0016 template), adapted for team-analytics.
# Build:  docker build -t team-analytics:dev .
# The build context is the service dir; third_party/go-sdk must be synced first
# and generated/ produced by `make proto` (buf is not in the image).
#
# Differences from the template:
#  - three binaries (consumer, ga4-relay, ga4import) share one image; the
#    deploy picks one by command/entrypoint (default: /consumer);
#  - the DuckDB adapter uses CGO, so the build is CGO_ENABLED=1 and the runtime
#    is the glibc distroless/cc base, not distroless/static;
#  - an empty /data owned by nonroot: a fresh named volume mounted there
#    inherits the ownership, so DuckDB (and the Parquet export) can write to it.

FROM golang:1.26-bookworm AS build
WORKDIR /src

# go.mod replaces the SDK with ./third_party/go-sdk, so it must be present
# before `go mod download`.
COPY go.mod go.sum ./
COPY third_party/ third_party/
RUN go mod download

COPY . .

ENV CGO_ENABLED=1 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/consumer ./cmd/consumer \
 && go build -trimpath -ldflags="-s -w" -o /out/ga4import ./cmd/ga4import \
 && go build -trimpath -ldflags="-s -w" -o /out/ga4-relay ./cmd/ga4-relay \
 && mkdir -p /out/data

FROM gcr.io/distroless/cc-debian12:nonroot
WORKDIR /
COPY --from=build /out/consumer /consumer
COPY --from=build /out/ga4import /ga4import
COPY --from=build /out/ga4-relay /ga4-relay
COPY --from=build --chown=nonroot:nonroot /out/data /data

ARG GRPC_PORT=50059
ENV GRPC_PORT=${GRPC_PORT}
EXPOSE ${GRPC_PORT}

USER nonroot:nonroot
ENTRYPOINT ["/consumer"]
