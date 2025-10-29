.PHONY: help build run test clean deps lint docker-build docker-run

# 변수 정의
APP_NAME=media-server
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# 기본 타겟
help:
	@echo "Available targets:"
	@echo "  make deps         - 의존성 다운로드"
	@echo "  make build        - 애플리케이션 빌드"
	@echo "  make run          - 애플리케이션 실행"
	@echo "  make test         - 테스트 실행"
	@echo "  make test-cover   - 커버리지와 함께 테스트 실행"
	@echo "  make lint         - 린트 실행"
	@echo "  make clean        - 빌드 파일 정리"
	@echo "  make docker-build - Docker 이미지 빌드"
	@echo "  make docker-run   - Docker 컨테이너 실행"

# 의존성 다운로드
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify
	go mod tidy

# 빌드
build: deps
	@echo "Building $(APP_NAME)..."
	go build $(LDFLAGS) -o bin/$(APP_NAME) cmd/server/main.go

# 최적화 빌드
build-prod: deps
	@echo "Building $(APP_NAME) for production..."
	CGO_ENABLED=0 go build $(LDFLAGS) -trimpath -o bin/$(APP_NAME) cmd/server/main.go

# 실행
run: build
	@echo "Running $(APP_NAME)..."
	./bin/$(APP_NAME)

# 개발 모드 실행 (hot reload with air)
dev:
	@echo "Running in development mode with hot reload..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "air not installed. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Running without hot reload..."; \
		go run cmd/server/main.go; \
	fi

# 테스트
test:
	@echo "Running tests..."
	go test -v -race ./...

# 커버리지와 함께 테스트
test-cover:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 벤치마크
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# 린트
lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# 포맷팅
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# 정리
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf dist/
	rm -rf tmp/
	rm -f coverage.txt coverage.html
	go clean

# Docker 이미지 빌드
docker-build:
	@echo "Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) -f docker/Dockerfile .

# Docker 컨테이너 실행
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 8081:8081 -p 9090:9090 \
		-v $(PWD)/configs:/app/configs \
		--name $(APP_NAME) \
		$(APP_NAME):$(VERSION)

# Docker Compose 실행
docker-compose-up:
	@echo "Starting with Docker Compose..."
	docker-compose -f docker/docker-compose.yml up --build

# Docker Compose 종료
docker-compose-down:
	@echo "Stopping Docker Compose..."
	docker-compose -f docker/docker-compose.yml down

# 크로스 컴파일
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(APP_NAME)-linux-amd64 cmd/server/main.go
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(APP_NAME)-linux-arm64 cmd/server/main.go
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(APP_NAME)-windows-amd64.exe cmd/server/main.go
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(APP_NAME)-darwin-amd64 cmd/server/main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(APP_NAME)-darwin-arm64 cmd/server/main.go

# 버전 정보
version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
