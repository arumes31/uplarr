# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test ./...

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /uplarr ./cmd/uplarr

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk --no-cache upgrade \
    && apk --no-cache add ca-certificates tzdata \
    && mkdir -p /app /config /data \
    && chown -R 10001:10001 /app /config /data

WORKDIR /app

COPY --from=builder --chown=10001:10001 /uplarr /app/uplarr

# Inside the container the app must listen on all interfaces for the published
# port to work; the binary itself defaults to 127.0.0.1.
ENV BIND_ADDR=0.0.0.0

EXPOSE 8080

USER 10001:10001

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -T 3 -O /dev/null http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/app/uplarr"]
