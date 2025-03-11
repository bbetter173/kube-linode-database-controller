.PHONY: build test lint docker clean deploy

# Variables
IMG_NAME := linode-db-allowlist
IMG_TAG := latest
BINARY_NAME := nodewatcher

# Go variables
GO := go
GOBUILD := $(GO) build
GOTEST := $(GO) test
GOLINT := golangci-lint

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	mkdir -p bin
	CGO_ENABLED=0 $(GOBUILD) -o bin/$(BINARY_NAME) ./cmd/nodewatcher

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run

# Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t $(IMG_NAME):$(IMG_TAG) .

# Run locally
run-local:
	@echo "Running locally with kubeconfig..."
	$(GO) run ./cmd/nodewatcher --kubeconfig=${HOME}/.kube/config --config=config/config.yaml

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Deploy to Kubernetes
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/

# Help command
help:
	@echo "Available commands:"
	@echo "  make build            - Build the application"
	@echo "  make test             - Run tests"
	@echo "  make test-coverage    - Run tests with coverage"
	@echo "  make lint             - Run linter"
	@echo "  make docker           - Build Docker image"
	@echo "  make run-local        - Run locally with kubeconfig"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make deploy           - Deploy to Kubernetes"
	@echo "  make help             - Show this help message"

# Default target
all: lint test build 