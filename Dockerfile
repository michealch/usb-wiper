# syntax=docker/dockerfile:1.26@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# ---- Build stage ----
FROM golang:1.26-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM debian:stable-slim@sha256:04634311a8d5fc442b6eb06d792293c4f3e2268652ca7634e00ce8ef5cc0a28a
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
