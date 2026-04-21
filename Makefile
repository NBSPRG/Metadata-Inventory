.PHONY: all build test lint clean up down fmt vet tidy

# ============================================================
# HTTP Metadata Inventory — Makefile
# ============================================================

# --- Build ---
all: tidy vet build

build:
	go build -o bin/api.exe ./api
	go build -o bin/worker.exe ./worker

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/api ./api
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/worker ./worker

# --- Code Quality ---
fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# --- Testing ---
test:
	go test -race -count=1 ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-verbose:
	go test -race -v -count=1 ./...

# --- Docker ---
up:
	docker-compose up -d --build

down:
	docker-compose down -v

logs:
	docker-compose logs -f

# --- Utilities ---
clean:
	rm -rf bin/ coverage.out coverage.html

env:
	cp -n .env.example .env || true

help:
	@echo "Available targets:"
	@echo "  build        - Build API and Worker binaries"
	@echo "  test         - Run all tests with race detector"
	@echo "  test-cover   - Run tests with coverage report"
	@echo "  lint         - Run golangci-lint"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  tidy         - Run go mod tidy"
	@echo "  up           - Start all services via docker-compose"
	@echo "  down         - Stop and remove all containers"
	@echo "  logs         - Tail docker-compose logs"
	@echo "  clean        - Remove build artifacts"
	@echo "  env          - Create .env from .env.example"
