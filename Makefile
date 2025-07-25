.PHONY: build run test test-integration clean docker-up docker-down swagger deps

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=jatis
BINARY_UNIX=$(BINARY_NAME)_unix

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...

# Run the application
run:
	$(GOCMD) run main.go

# Test the application
test:
	$(GOTEST) -v ./...

# Run integration tests
test-integration:
	$(GOTEST) -v ./tests/...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

# Cross compilation for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Generate Swagger documentation
swagger:
	swag init -g main.go

# Start Docker services (PostgreSQL and RabbitMQ)
docker-up:
	docker-compose up -d

# Stop Docker services
docker-down:
	docker-compose down

# Start development environment
dev: docker-up deps swagger
	sleep 5  # Wait for services to start
	$(GOCMD) run main.go

# Run all tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code
lint:
	golangci-lint run

# Install development tools
install-tools:
	$(GOGET) github.com/swaggo/swag/cmd/swag@latest
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Database migration
migrate-up:
	$(GOCMD) run cmd/migrate/main.go up

migrate-down:
	$(GOCMD) run cmd/migrate/main.go down

# Help
help:
	@echo "Available commands:"
	@echo "  build           - Build the application"
	@echo "  run             - Run the application"
	@echo "  test            - Run unit tests"
	@echo "  test-integration - Run integration tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  clean           - Clean build artifacts"
	@echo "  deps            - Download and tidy dependencies"
	@echo "  swagger         - Generate Swagger documentation"
	@echo "  docker-up       - Start Docker services"
	@echo "  docker-down     - Stop Docker services"
	@echo "  dev             - Start development environment"
	@echo "  fmt             - Format code"
	@echo "  lint            - Lint code"
	@echo "  install-tools   - Install development tools"
	@echo "  help            - Show this help message"