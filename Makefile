.PHONY: all build build-backend build-frontend docker docker-build docker-up docker-down clean test run-backend run-frontend run

# Variables
BACKEND_BINARY=issue-migrator-backend
DOCKER_IMAGE_BACKEND=issue-migrator-backend
DOCKER_IMAGE_FRONTEND=issue-migrator-frontend

# Default target
all: build

# Build everything
build: build-backend build-frontend
	@echo "✅ Build completed successfully!"

# Build backend
build-backend:
	@echo "🔨 Building Go backend..."
	@cd backend && go build -o $(BACKEND_BINARY) .
	@echo "✅ Backend built: backend/$(BACKEND_BINARY)"

# Build frontend
build-frontend:
	@echo "🔨 Building React frontend..."
	@cd frontend && npm install && npm run build
	@echo "✅ Frontend built: frontend/dist/"

# Run backend in development mode
run-backend:
	@echo "🚀 Starting backend server..."
	@cd backend && go run main.go

# Run frontend in development mode
run-frontend:
	@echo "🚀 Starting frontend development server..."
	@cd frontend && npm run dev

# Run both backend and frontend
run:
	@echo "🚀 Starting both backend and frontend..."
	@make -j2 run-backend run-frontend

# Docker commands
docker: docker-build docker-up

docker-build:
	@echo "🐳 Building Docker images..."
	@docker-compose build
	@echo "✅ Docker images built successfully!"

docker-up:
	@echo "🚀 Starting Docker containers..."
	@docker-compose up -d
	@echo "✅ Application is running!"
	@echo "📋 Frontend: http://localhost:3000"
	@echo "📋 Backend: http://localhost:8080"

docker-down:
	@echo "🛑 Stopping Docker containers..."
	@docker-compose down
	@echo "✅ Containers stopped"

docker-logs:
	@docker-compose logs -f

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f backend/$(BACKEND_BINARY)
	@rm -rf frontend/dist
	@rm -rf frontend/node_modules
	@echo "✅ Clean completed"

# Test commands
test:
	@echo "🧪 Running tests..."
	@cd backend && go test ./...
	@cd frontend && npm test -- --run
	@echo "✅ All tests passed!"

test-backend:
	@echo "🧪 Running backend tests..."
	@cd backend && go test -v ./...

test-frontend:
	@echo "🧪 Running frontend tests..."
	@cd frontend && npm test

# Lint commands
lint:
	@echo "🔍 Running linters..."
	@cd backend && go fmt ./...
	@cd backend && go vet ./...
	@echo "✅ Linting completed"

# Help command
help:
	@echo "📚 Available commands:"
	@echo "  make build          - Build both backend and frontend"
	@echo "  make build-backend  - Build Go backend only"
	@echo "  make build-frontend - Build React frontend only"
	@echo "  make run           - Run both in development mode"
	@echo "  make run-backend   - Run backend in development mode"
	@echo "  make run-frontend  - Run frontend in development mode"
	@echo "  make docker        - Build and run with Docker"
	@echo "  make docker-build  - Build Docker images"
	@echo "  make docker-up     - Start Docker containers"
	@echo "  make docker-down   - Stop Docker containers"
	@echo "  make docker-logs   - View Docker logs"
	@echo "  make test          - Run all tests"
	@echo "  make lint          - Run linters"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make help          - Show this help message"