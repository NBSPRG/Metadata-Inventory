.PHONY: all build test lint clean up down fmt vet tidy swagger test-e2e-docker install-deps

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

test-e2e-docker:
	docker compose down -v
	docker compose up -d --build
	go test -tags=e2e_docker -count=1 -v ./api -run TestE2E_DockerFullStack

# --- Docker ---
up:
	docker compose up -d --build

down:
	docker compose down -v

logs:
	docker compose logs -f

# --- Utilities ---
clean:
	rm -rf bin coverage.out coverage.html

install-deps:
	@echo "Installing Go and build-essential (requires sudo privileges)..."
	sudo snap install go --classic || sudo apt-get update && sudo apt-get install -y golang-go
	sudo apt-get update && sudo apt-get install -y build-essential

env:
	test -f .env || cp .env.example .env

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
	@echo "  swagger      - Generate Swagger docs"
	@echo "  install-deps - Install Go and build tools (Ubuntu/Debian)"

swagger:
	swag init -g api/main.go -o docs/swagger
