# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
ARG BIN=server
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/${BIN} ./cmd/${BIN}
FROM gcr.io/distroless/static:nonroot
ARG BIN=server
WORKDIR /
COPY --from=build /out/${BIN} /server
ENV GRPC_PORT=50054
EXPOSE 50054
USER nonroot:nonroot
ENTRYPOINT ["/server"]
