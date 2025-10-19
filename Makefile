.PHONY: help build test lint fmt clean docker-build up-deps-services up-app up-all down-app down-volume down logs setup release release-snapshot go-build go-test go-test-coverage go-lint go-fmt go-fmt-check docker-up-deps-services docker-up-app docker-up-all docker-status docker-down-app docker-down docker-down-volume docker-deps-logs docker-app-logs

# Variables
BINARY_NAME=vault-sync
DOCKER_IMAGE=vault-sync
DOCKER_TAG=latest
MAIN_PATH=./main.go

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

SERVICES ?= postgres vault1 vault2

help:
	@echo "$(BLUE)Vault Sync - Makefile Commands$(NC)"
	@echo ""
	@echo "$(YELLOW)Development:$(NC)"
	@echo "  make go-build              - Build the binary"
	@echo "  make go-test               - Run all tests"
	@echo "  make go-test-coverage      - Run all tests with coverage"
	@echo "  make go-lint               - Run golangci-lint"
	@echo "  make go-fmt                - Format code with gofmt"
	@echo "  make go-fmt-check          - Check if code is properly formatted"
	@echo ""
	@echo "$(YELLOW)Docker:$(NC)"
	@echo "  make docker-build              - Build Docker image"
	@echo "  make docker-up-deps-services   - Start dependency services (default: postgres vault1 vault2)"
	@echo "                                   Usage: make docker-up-deps-services SERVICES=\"postgres\""
	@echo "  make docker-up-app             - Start app service only"
	@echo "  make docker-up-all             - Start all services"
	@echo "  make docker-status             - Check status of services"
	@echo "  make docker-down-app           - Stop app service"
	@echo "  make docker-down               - Stop all services"
	@echo "  make docker-down-volume        - Stop all services and remove volumes"
	@echo "  make docker-deps-logs          - View dependency services logs"
	@echo "  make docker-app-logs           - View app logs with jq formatting"
	@echo ""
	@echo "$(YELLOW)Setup:$(NC)"
	@echo "  make setup                     - Full local setup (init + seed)"
	@echo "  make clean                     - Clean build artifacts"
	@echo ""
	@echo "$(YELLOW)Release:$(NC)"
	@echo "  make release                   - Create a new release with GoReleaser"
	@echo "  make release-snapshot          - Build snapshot release (no publish)"
	@echo ""
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make docker-up-deps-services SERVICES=\"postgres\""
	@echo "  make docker-build DOCKER_TAG=v1.0.0"
	@echo ""
	@echo "$(YELLOW)Variables:$(NC)"
	@echo "  SERVICES          - Services to start (default: postgres vault1 vault2)"
	@echo "  DOCKER_TAG        - Docker image tag (default: latest)"
	@echo "  DOCKER_IMAGE      - Docker image name (default: vault-sync)"

## build: Build the binary
go-build:
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)✓ Build complete: bin/$(BINARY_NAME)$(NC)"

## test: Run all tests
## test: Run tests silently  
go-test:
	@echo "$(BLUE)Running all tests...$(NC)"
	@TEST_SILENT=1 go test ./...
	@echo "$(GREEN)✓ Tests completed$(NC)"

## test-verbose: Run tests with logs
go-test-verbose:
	@echo "$(BLUE)Running all tests (verbose)...$(NC)"
	@go test -v ./...
		@echo "$(GREEN)✓ Tests completed$(NC)"

go-test-coverage:
	@echo "$(BLUE)Running all tests...$(NC)"
	@go test -v -race -timeout 5m -cover ./...


## lint: Run golangci-lint
go-lint:
	@echo "$(BLUE)Running golangci-lint (production code only)...$(NC)"
	@golangci-lint run --config .golangci.yml --timeout 5m
	@echo "$(GREEN)✓ Linting complete$(NC)"

## fmt: Format code with gofmt
go-fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	@gofmt -s -w .
	@echo "$(GREEN)✓ Code formatted$(NC)"

## fmt-check: Check if code is properly formatted
go-fmt-check:
	@echo "$(BLUE)Checking code formatting...$(NC)"
	@OUTPUT=$$(gofmt -l .); \
	if [ -n "$$OUTPUT" ]; then \
		echo "$(RED)Go files are not formatted. Please run 'make go-fmt':$(NC)"; \
		echo "$$OUTPUT"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Code is properly formatted$(NC)"

## docker-build: Build Docker image
docker-build:
	@echo "$(BLUE)Building Docker image...$(NC)"
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) . 
	@echo "$(GREEN)✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(NC)"

### Useful commands for local development with Docker Compose
docker-up-deps-services:
	@echo "$(BLUE)Starting services...$(NC)"
	@docker compose up -d $(SERVICES)
	@echo "$(GREEN)✓ Services started$(NC)"

docker-up-app:
	@echo "$(BLUE)Starting app service...$(NC)"
	@docker compose up -d app
	@echo "$(GREEN)✓ App service started$(NC)"

docker-up-all:
	@echo "$(BLUE)Starting all services...$(NC)"
	@docker compose up -d
	@echo "$(GREEN)✓ Services started$(NC)"

## Check status of services
docker-status:
	@echo "$(BLUE)Checking status of services...$(NC)"
	@docker compose ps
	@echo "$(GREEN)✓ Status check complete$(NC)"

## down: Stop all services
docker-down-app:
	@echo "$(BLUE)Stopping all services...$(NC)"
	@docker compose down app
	@echo "$(GREEN)✓ App stopped$(NC)"

docker-down:
	@echo "$(BLUE)Stopping all services...$(NC)"
	@docker compose down
	@echo "$(GREEN)✓ Services stopped$(NC)"

docker-down-volume:
	@echo "$(BLUE)Stopping all services, and removing volumes...$(NC)"
	@docker compose down -v
	@echo "$(GREEN)✓ Services stopped and Volume Deleted$(NC)"

docker-deps-logs:
	@docker compose logs --no-log-prefix -f $(SERVICES)

docker-app-logs:
	@which jq > /dev/null || (echo "$(RED)Error: jq not installed$(NC)" && exit 1)
	@docker compose logs --no-log-prefix -f app | jq -C '.'

# Add this to the setup target in Makefile
setup:
	@echo "$(BLUE)Full Local Setup$(NC)"
	@echo "$(YELLOW)1. Starting infrastructure...$(NC)"
	$(MAKE) docker-up-deps-services
	@echo "$(YELLOW)2. Waiting for services...$(NC)"
	@sleep 10
	@echo "$(YELLOW)3. Initializing Vault cluster...$(NC)"
	@chmod +x docker/*.sh && ./docker/init.sh
	@echo "$(YELLOW)4. Generating Config File...$(NC)"
	@sh ./docker/config-generator.sh
	@echo "$(YELLOW)5. Seeding Vault with random secrets...$(NC)"
	@sh ./docker/seed-vault-secrets.sh
	@echo "$(GREEN)✓ Setup complete! Use 'make docker-deps-logs' or `make docker-app-logs` to view logs$(NC)"


export GITHUB_REPOSITORY_OWNER ?= binsabbar
export DOCKER_REGISTRY ?= ghcr.io

## release: Create a new release with GoReleaser
release:
	@echo "$(BLUE)Creating release with GoReleaser...$(NC)"
	@which goreleaser > /dev/null || (echo "$(RED)Error: goreleaser not installed. Run: brew install goreleaser$(NC)" && exit 1)
	@goreleaser release --clean
	@echo "$(GREEN)✓ Release complete$(NC)"

## release-snapshot: Build snapshot release (no publish)
release-snapshot:
	@echo "$(BLUE)Building snapshot release...$(NC)"
	@which goreleaser > /dev/null || (echo "$(RED)Error: goreleaser not installed. Run: brew install goreleaser$(NC)" && exit 1)
	@goreleaser release --snapshot --clean
	@echo "$(GREEN)✓ Snapshot built in dist/$(NC)"

## clean: Clean build artifacts
clean:
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf bin/
	@rm -rf dist/
	@go clean -cache
	@echo "$(GREEN)✓ Clean complete$(NC)"

# Default target
.DEFAULT_GOAL := help