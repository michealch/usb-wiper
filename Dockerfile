# syntax=docker/dockerfile:1.6

# ---- Build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/usb-wiper ./cmd/usb-wiper

# ---- Runtime stage ----
FROM alpine:3.19
RUN apk add --no-cache \
    smartmontools \
    dosfstools \
    parted \
    util-linux \
    e2fsprogs \
    wget \
    && rm -rf /var/cache/apk/*

COPY --from=builder /out/usb-wiper /usr/local/bin/usb-wiper

RUN adduser -D -u 1000 wiper

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/usb-wiper"]
