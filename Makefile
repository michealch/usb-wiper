# USB Wiper Makefile — every target runs inside a container.
# The host only needs Docker; no Go toolchain, no node, no runtime deps.
#
# Defaults:
#   GO_IMAGE   - toolchain image (Debian-based so -race works via cgo)
#   BUILD_IMAGE- the production runtime image (built by `make docker-build`)
#   PORT       - host port for `run`/`dev`
#   DATA_DIR   - host directory mounted at /data
#   UNSAFE_ALLOW_ALL_USB - passed through to the container ("" | "1")
#   ALLOW_HARDWARE_SECURE_ERASE - passed through ("" | "1")

.PHONY: help build run test test-verbose test-norace fmt vet lint tidy \
        docker-build dev dev-detached prod stop logs shell debug clean

APP        := usb-wiper
IMAGE      := usb-wiper
TAG        := latest
GO_IMAGE   := golang:1.26
BUILD_IMAGE := $(IMAGE):$(TAG)
PORT       ?= 8181
DATA_DIR   ?= $(CURDIR)/data
PWD        := $(shell pwd)
# Mount the local Go build cache so repeated runs stay fast, and /dev+/sys
# (read-only) so `run` and `debug` see real devices.
DOCKER_GO  := docker run --rm \
	-v "$(PWD)":/src -w /src \
	-v "$(HOME)/.cache/go-docker":/root/.cache/go-build \
	$(GO_IMAGE)

help:
	@echo "Targets:"
	@echo "  build         - Build Linux binary into bin/ (inside container)"
	@echo "  run           - Run the app inside a container (mounts /dev and /sys; use UNSAFE_ALLOW_ALL_USB=1 when needed)"
	@echo "  debug         - Run inside a container with dlv (debugger) attached; connect your IDE to localhost:2345"
	@echo "  test          - Run tests with race detector (inside container)"
	@echo "  test-verbose  - Run tests with verbose output"
	@echo "  test-norace   - Run tests without race detector"
	@echo "  fmt           - gofmt all Go files (inside container)"
	@echo "  vet           - go vet ./... (inside container)"
	@echo "  lint          - vet + golangci-lint (installed into a temp container)"
	@echo "  tidy          - go mod tidy (inside container)"
	@echo "  docker-build  - Build production Docker image"
	@echo "  dev           - Start dev compose (hot reload)"
	@echo "  dev-detached  - Start dev compose in background"
	@echo "  prod          - Start prod compose"
	@echo "  stop          - Stop all compose services"
	@echo "  logs          - Tail container logs"
	@echo "  shell         - Shell into a running dev container"
	@echo "  clean         - Remove build artifacts"
	@echo
	@echo "Container targets run with $(GO_IMAGE); override with GO_IMAGE=... PORT=... DATA_DIR=..."

build:
	mkdir -p bin
	$(DOCKER_GO) sh -c 'CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(APP) ./cmd/$(APP) && chown $(shell id -u):$(shell id -g) bin/$(APP)'

# Run the appliance in a throwaway container with host devices visible.
# Bind the host data dir so state persists across runs.
run:
	mkdir -p "$(DATA_DIR)"
	docker run --rm -t --name $(APP)-run \
		-p $(PORT):8181 \
		-v /dev:/dev \
		-v /sys:/sys:ro \
		-v "$(DATA_DIR)":/data \
		-e PORT=8181 \
		-e UNSAFE_ALLOW_ALL_USB="$(UNSAFE_ALLOW_ALL_USB)" \
		-e ALLOW_HARDWARE_SECURE_ERASE="$(ALLOW_HARDWARE_SECURE_ERASE)" \
		$(BUILD_IMAGE)

# Debug: run the container with a dlv headless debugger on :2345 and the
# app paused at startup. Attach a Go DAP client (VS Code "Go: Attach to
# local process" or dlv client) to localhost:2345, then continue.
debug:
	mkdir -p "$(DATA_DIR)"
	docker build -t $(IMAGE)-debug:$(TAG) -f deploy/Dockerfile.debug --build-arg BASE_IMAGE=$(GO_IMAGE) .
	docker run --rm -t --name $(APP)-debug \
		-p 2345:2345 -p $(PORT):8181 \
		-v /dev:/dev \
		-v /sys:/sys:ro \
		-v "$(DATA_DIR)":/data \
		-v "$(PWD)":/src \
		-e PORT=8181 \
		-e UNSAFE_ALLOW_ALL_USB="$(UNSAFE_ALLOW_ALL_USB)" \
		-e ALLOW_HARDWARE_SECURE_ERASE="$(ALLOW_HARDWARE_SECURE_ERASE)" \
		$(IMAGE)-debug:$(TAG)

test:
	$(DOCKER_GO) go test ./... -race -count=1

test-norace:
	$(DOCKER_GO) go test ./... -count=1

test-verbose:
	$(DOCKER_GO) go test -v ./... -race -count=1

fmt:
	$(DOCKER_GO) gofmt -w ./cmd ./internal

vet:
	$(DOCKER_GO) go vet ./...

# lint runs golangci-lint inside a throwaway container; no host install.
lint: vet
	@docker run --rm -v "$(PWD)":/src -w /src golangci/golangci-lint:v2.10.1-alpine golangci-lint run

tidy:
	$(DOCKER_GO) go mod tidy

docker-build:
	docker build -t $(BUILD_IMAGE) .

dev:
	docker compose -f deploy/docker-compose.dev.yml up --build

dev-detached:
	docker compose -f deploy/docker-compose.dev.yml up --build -d

prod:
	docker compose -f deploy/docker-compose.prod.yml up --build -d

stop:
	docker compose -f deploy/docker-compose.dev.yml down 2>/dev/null || true
	docker compose -f deploy/docker-compose.prod.yml down 2>/dev/null || true

logs:
	docker compose -f deploy/docker-compose.dev.yml logs -f

shell:
	docker compose -f deploy/docker-compose.dev.yml exec usb-wiper-dev sh

clean:
	rm -rf bin/ tmp/
