.PHONY: help build run test test-verbose test-race test-norace fmt lint vet tidy \
        docker-build dev dev-detached prod stop logs shell clean

APP        := usb-wiper
IMAGE      := usb-wiper
TAG        := latest

help:
	@echo "Targets:"
	@echo "  build         - Build local binary"
	@echo "  run           - Run locally (requires sudo)"
	@echo "  test          - Run tests"
	@echo "  test-verbose  - Run tests with verbose output"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run linter"
	@echo "  tidy          - Run go mod tidy"
	@echo "  docker-build  - Build production image"
	@echo "  dev           - Start dev compose"
	@echo "  dev-detached  - Start dev compose in background"
	@echo "  prod          - Start prod compose"
	@echo "  stop          - Stop all compose services"
	@echo "  logs          - Tail container logs"
	@echo "  shell         - Shell into running container"
	@echo "  clean         - Remove build artifacts"

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(APP) ./cmd/$(APP)

run: build
	sudo ./bin/$(APP)

test:
	go test ./... -race -count=1

test-norace:
	go test ./... -count=1

test-verbose:
	go test -v ./... -race -count=1

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: vet
	@which golangci-lint > /dev/null && golangci-lint run || echo "golangci-lint not installed, skipping"

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE):$(TAG) .

dev:
	docker compose -f deploy/docker-compose.dev.yml --env-file deploy/dev.env up --build

dev-detached:
	docker compose -f deploy/docker-compose.dev.yml --env-file deploy/dev.env up --build -d

prod:
	docker compose -f deploy/docker-compose.prod.yml --env-file deploy/prod.env up --build -d

stop:
	docker compose -f deploy/docker-compose.dev.yml down 2>/dev/null || true
	docker compose -f deploy/docker-compose.prod.yml down 2>/dev/null || true

logs:
	docker compose -f deploy/docker-compose.dev.yml logs -f

shell:
	docker compose -f deploy/docker-compose.dev.yml exec usb-wiper-dev sh

clean:
	rm -rf bin/ tmp/
