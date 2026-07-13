# syntax=docker/dockerfile:1.25@sha256:0adf442eae370b6087e08edc7c50b552d80ddf261576f4ebd6421006b2461f12

# ---- Build stage ----
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM debian:stable-slim@sha256:ee12ffb55625b99d62837a72f037d9b2f18fd0c787a89c2b9a4f09666c48776c
RUN apt-get update && apt-get install -y --no-install-recommends \
    smartmontools \
    dosfstools \
    parted \
    util-linux \
    e2fsprogs \
    wget \
    hdparm \
    nvme-cli \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/usb-wiper /usr/local/bin/usb-wiper

RUN useradd --no-create-home --shell /usr/sbin/nologin --uid 1000 wiper

EXPOSE 8181
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8181/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/usb-wiper"]
