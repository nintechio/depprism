FROM golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY cmd/depprism ./cmd/depprism
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=action" -o /out/depprism ./cmd/depprism

FROM alpine:3.23.5@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

RUN apk add --no-cache ca-certificates git \
    && addgroup -S depprism \
    && adduser -S -D -u 10001 -G depprism depprism
COPY --from=build /out/depprism /usr/local/bin/depprism

ENV HOME=/home/depprism
USER depprism
ENTRYPOINT ["/usr/local/bin/depprism"]
