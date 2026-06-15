# syntax=docker/dockerfile:1.24@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

# ---- Build stage ----
FROM golang:1.26-alpine@sha256:7a3e50096189ad57c9f9f865e7e4aa8585ed1585248513dc5cda498e2f41812c AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM debian:stable-slim@sha256:34363c20bd149e41365fc77b086da067ed13ab2dff4cd0612788e12e6d52c44c
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
