# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
# go.mod replaces the SDK with ./third_party/go-sdk, so it must be present
# before `go mod download` (ADR-0016).
COPY go.mod go.sum ./
COPY third_party/ third_party/
RUN go mod download
COPY . .
ARG BIN=gateway
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/${BIN}
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/gateway /gateway
ENV HTTP_PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gateway"]
