# syntax=docker/dockerfile:1.28@sha256:bb22d9815c728170f72750f4e5b0d672e06176142e1d602c7e66c050100b7e5b

# ---- Build stage ----
FROM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM debian:stable-slim@sha256:a717a33469731894e77e912ea7d714e27c2dc38ee6554fd1a3102e92999cfc66
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
