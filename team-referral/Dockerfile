# syntax=docker/dockerfile:1
# Template for Go services (ADR-0016). Copy to the service root.
# Build:  docker build --build-arg BIN=server --build-arg GRPC_PORT=50062 -t <svc>:dev .
# The build context is the service dir; third_party/go-sdk must be synced first.

FROM golang:1.26 AS build
WORKDIR /src

# go.mod replaces the SDK with ./third_party/go-sdk, so it must be present
# before `go mod download`.
COPY go.mod go.sum ./
COPY third_party/ third_party/
RUN go mod download

COPY . .

ARG BIN=server
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${BIN}

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/app /app

ARG GRPC_PORT=50062
ENV GRPC_PORT=${GRPC_PORT}
EXPOSE ${GRPC_PORT}

USER nonroot:nonroot
ENTRYPOINT ["/app"]
