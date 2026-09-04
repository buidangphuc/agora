# syntax=docker/dockerfile:1

FROM golang:1.22 AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/server /server

ENV GRPC_PORT=50062
EXPOSE 50062

USER nonroot:nonroot
ENTRYPOINT ["/server"]
