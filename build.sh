#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Issue Migrator Build Script${NC}"
echo "================================"

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go 1.21+${NC}"
    exit 1
fi

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo -e "${RED}❌ Node.js is not installed. Please install Node.js 22+${NC}"
    exit 1
fi

# Check if npm is installed
if ! command -v npm &> /dev/null; then
    echo -e "${RED}❌ npm is not installed${NC}"
    exit 1
fi

echo -e "${GREEN}✅ All prerequisites met${NC}"
echo ""

# Build backend
echo -e "${YELLOW}Building backend...${NC}"
cd backend

# Download dependencies
echo "Downloading Go dependencies..."
go mod download

# Build binary
echo "Building Go binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o issue-migrator-linux-amd64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o issue-migrator-darwin-amd64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o issue-migrator-darwin-arm64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o issue-migrator-windows-amd64.exe .

echo -e "${GREEN}✅ Backend built successfully${NC}"
cd ..
echo ""

# Build frontend
echo -e "${YELLOW}Building frontend...${NC}"
cd frontend

# Install dependencies
echo "Installing npm dependencies..."
npm ci

# Build production bundle
echo "Building production bundle..."
npm run build

echo -e "${GREEN}✅ Frontend built successfully${NC}"
cd ..
echo ""

# Create distribution directory
echo -e "${YELLOW}Creating distribution package...${NC}"
rm -rf dist
mkdir -p dist/backend/binaries
mkdir -p dist/frontend

# Copy backend files
cp -r backend/handlers dist/backend/
cp -r backend/models dist/backend/
cp -r backend/utils dist/backend/
cp backend/*.go dist/backend/
cp backend/go.mod dist/backend/
cp backend/go.sum dist/backend/
cp backend/.env.example dist/backend/
cp backend/Dockerfile dist/backend/
mv backend/issue-migrator-* dist/backend/binaries/

# Copy frontend files
cp -r frontend/dist/* dist/frontend/
cp frontend/package*.json dist/frontend/
cp frontend/Dockerfile dist/frontend/
cp frontend/nginx.conf dist/frontend/

# Copy root files
cp docker-compose.yml dist/
cp README.md dist/
cp .gitignore dist/

echo -e "${GREEN}✅ Distribution package created in 'dist' directory${NC}"
echo ""

# Create tar archive
echo -e "${YELLOW}Creating archive...${NC}"
tar -czf issue-migrator-dist.tar.gz dist/
echo -e "${GREEN}✅ Archive created: issue-migrator-dist.tar.gz${NC}"

echo ""
echo -e "${GREEN}🎉 Build completed successfully!${NC}"
echo ""
echo "Next steps:"
echo "1. To run locally: make run"
echo "2. To run with Docker: docker-compose up"
echo "3. Distribution package: dist/"
echo "4. Archive: issue-migrator-dist.tar.gz"