# Makefile for Go Log Management API

.PHONY: help run build test swagger clean docker docker-build docker-run lint fmt vet deps install

# Variables
APP_NAME=log-api
BINARY_NAME=log-api
DOCKER_IMAGE=tcc-log-api
DOCKER_TAG=latest
GO_FILES=$(shell find . -name '*.go' -type f -not -path "./vendor/*")
MAIN_PATH=./cmd/api

# Help command
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Install dependencies
deps: ## Download dependencies
	@echo "📦 Downloading dependencies..."
	go mod download
	go mod verify

install: deps ## Install dependencies and tools
	@echo "🔧 Installing development tools..."
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/go-delve/delve/cmd/dlv@latest
	@echo "✅ Installation complete!"

# Build commands
build: ## Build the application
	@echo "🔨 Building $(BINARY_NAME)..."
	go build -ldflags="-s -w" -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "✅ Build complete: bin/$(BINARY_NAME)"

build-linux: ## Build for Linux
	@echo "🔨 Building for Linux..."
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(BINARY_NAME)-linux $(MAIN_PATH)

build-windows: ## Build for Windows
	@echo "🔨 Building for Windows..."
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(BINARY_NAME).exe $(MAIN_PATH)

# Run commands
run: ## Run the application
	@echo "🚀 Running $(APP_NAME)..."
	go run $(MAIN_PATH)/main.go

run-watch: ## Run with auto-reload (requires air)
	@echo "🔄 Running with hot reload..."
	air

dev: swagger run ## Generate swagger and run

# Swagger
swagger: ## Generate Swagger documentation
	@echo "📝 Generating Swagger docs..."
	swag init -g $(MAIN_PATH)/main.go -o ./docs --parseDependency --parseInternal
	@echo "✅ Swagger docs generated in ./docs"

swagger-fmt: ## Format swagger comments
	@echo "📝 Formatting Swagger comments..."
	swag fmt

# Testing
test: ## Run tests
	@echo "🧪 Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "✅ Tests complete"

test-coverage: test ## Run tests with coverage report
	@echo "📊 Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

test-integration: ## Run integration tests
	@echo "🧪 Running integration tests..."
	go test -v -tags=integration ./tests/...

test-benchmark: ## Run benchmark tests
	@echo "⚡ Running benchmarks..."
	go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./...

# Code quality
lint: ## Run linter
	@echo "🔍 Running linter..."
	golangci-lint run --timeout=5m

fmt: ## Format code
	@echo "🎨 Formatting code..."
	gofmt -s -w $(GO_FILES)
	goimports -w $(GO_FILES)

vet: ## Run go vet
	@echo "🔍 Running go vet..."
	go vet ./...

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

# Docker
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "✅ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: ## Run Docker container
	@echo "🐳 Running Docker container..."
	docker run -d \
		--name $(APP_NAME) \
		-p 5001:5001 \
		-p 9090:9090 \
		-v $(PWD)/config.yaml:/app/config.yaml \
		-v /var/log/tcc-wal:/var/log/tcc-wal \
		--restart unless-stopped \
		$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✅ Container running on http://localhost:5001"

docker-stop: ## Stop Docker container
	@echo "🛑 Stopping Docker container..."
	docker stop $(APP_NAME) || true
	docker rm $(APP_NAME) || true

docker-logs: ## Show Docker logs
	docker logs -f $(APP_NAME)

docker-compose-up: ## Start with docker-compose
	@echo "🐳 Starting services with docker-compose..."
	docker-compose up -d
	@echo "✅ Services started"

docker-compose-down: ## Stop docker-compose services
	@echo "🛑 Stopping docker-compose services..."
	docker-compose down

# Profiling
profile-cpu: ## Profile CPU usage
	@echo "🔬 Profiling CPU..."
	go tool pprof -http=:8080 cpu.prof

profile-mem: ## Profile memory usage
	@echo "🔬 Profiling memory..."
	go tool pprof -http=:8080 mem.prof

# Utilities
clean: ## Clean build artifacts
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	rm -rf docs/docs.go docs/swagger.json docs/swagger.yaml
	rm -f coverage.out coverage.html
	rm -f cpu.prof mem.prof
	go clean -cache -testcache -modcache
	@echo "✅ Clean complete"

logs: ## Show application logs (if running in Docker)
	docker logs -f $(APP_NAME)

health: ## Check API health
	@echo "🏥 Checking API health..."
	@curl -s http://localhost:5001/health | jq . || echo "API not responding"

deps-update: ## Update dependencies
	@echo "⬆️  Updating dependencies..."
	go get -u ./...
	go mod tidy

deps-vendor: ## Vendor dependencies
	@echo "📦 Vendoring dependencies..."
	go mod vendor

# Database
db-migrate: ## Run database migrations (placeholder)
	@echo "🗄️  Running migrations..."
	@echo "⚠️  Implement migration logic here"

# Load testing
load-test: ## Run load test with vegeta
	@echo "⚡ Running load test..."
	@echo "POST http://localhost:5001/logs" | vegeta attack -duration=30s -rate=1000 -body=test_payload.json | vegeta report

wrk-test: ## Run load test with wrk
	@echo "⚡ Running wrk load test..."
	wrk -t4 -c100 -d30s --latency http://localhost:5001/health

# Development
dev-setup: install ## Complete development setup
	@echo "🚀 Development setup complete!"
	@echo "Next steps:"
	@echo "  1. Copy .env.example to .env"
	@echo "  2. Update config.yaml with your settings"
	@echo "  3. Run 'make swagger' to generate API docs"
	@echo "  4. Run 'make run' to start the server"

version: ## Show Go version
	@go version

info: ## Show project info
	@echo "Project: $(APP_NAME)"
	@echo "Binary: $(BINARY_NAME)"
	@echo "Docker: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	@echo "Go Files: $(shell echo $(GO_FILES) | wc -w)"
	@echo "Go Version: $(shell go version)"
