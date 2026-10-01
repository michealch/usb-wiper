# syntax=docker/dockerfile:1.27@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# ---- Build stage ----
FROM golang:1.26-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM debian:stable-slim@sha256:5bc3287b25407c965a30f38e32603dc253a3869e1b12a21ac09bfc27fd8b13ce
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
