FROM golang:1.27.3-alpine3.23@sha256:8a5046497cda39860b832ffdbe8dd66018a0f05bb01352f533a36e7e2ced66a0 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY cmd/depprism ./cmd/depprism
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=action" -o /out/depprism ./cmd/depprism

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates git su-exec \
    && addgroup -S depprism \
    && adduser -S -D -u 10001 -G depprism depprism
COPY --from=build /out/depprism /usr/local/bin/depprism
COPY entrypoint.sh /usr/local/bin/depprism-entrypoint

ENV HOME=/home/depprism
ENTRYPOINT ["/usr/local/bin/depprism-entrypoint"]
