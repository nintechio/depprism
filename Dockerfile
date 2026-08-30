FROM golang:1.27.0-alpine3.23@sha256:3747dcba41c8b0db3211fda4db61638b980e17ac5bb3c94460a975a9cfe19395 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY cmd/depprism ./cmd/depprism
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=action" -o /out/depprism ./cmd/depprism

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates git \
    && addgroup -S depprism \
    && adduser -S -D -u 10001 -G depprism depprism
COPY --from=build /out/depprism /usr/local/bin/depprism

ENV HOME=/home/depprism
USER depprism
ENTRYPOINT ["/usr/local/bin/depprism"]
