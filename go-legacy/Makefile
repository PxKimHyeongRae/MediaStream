.PHONY: help build run test clean deps lint docker-build docker-run docker-up docker-down docker-logs docker-deploy

# 변수 정의
APP_NAME=media-server
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
DOCKER_IMAGE=rtsp-webrtc-server
DOCKER_TAG=$(VERSION)

# 기본 타겟
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  make deps           - 의존성 다운로드"
	@echo "  make build          - 애플리케이션 빌드"
	@echo "  make build-prod     - 프로덕션 최적화 빌드"
	@echo "  make run            - 애플리케이션 실행"
	@echo "  make dev            - 개발 모드 실행 (hot reload)"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - 테스트 실행"
	@echo "  make test-cover     - 커버리지와 함께 테스트 실행"
	@echo "  make bench          - 벤치마크 실행"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint           - 린트 실행"
	@echo "  make fmt            - 코드 포맷팅"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build   - Docker 이미지 빌드"
	@echo "  make docker-run     - Docker 컨테이너 실행"
	@echo "  make docker-up      - Docker Compose로 실행"
	@echo "  make docker-down    - Docker Compose 종료"
	@echo "  make docker-logs    - Docker 로그 보기"
	@echo "  make docker-deploy  - 프로덕션 배포 (빌드+실행)"
	@echo "  make docker-clean   - Docker 리소스 정리"
	@echo ""
	@echo "Utilities:"
	@echo "  make clean          - 빌드 파일 정리"
	@echo "  make version        - 버전 정보 출력"

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
	cd docker && docker-compose build

# Docker 컨테이너 실행 (단일)
docker-run:
	@echo "Running Docker container..."
	docker run -d \
		-p 8080:8080 -p 8081:8081 -p 9090:9090 \
		-v $(PWD)/configs:/app/configs:ro \
		-v $(APP_NAME)-logs:/app/logs \
		--name $(APP_NAME) \
		--restart unless-stopped \
		$(DOCKER_IMAGE):latest

# Docker Compose 실행 (개발)
docker-up:
	@echo "Starting with Docker Compose (development)..."
	cd docker && docker-compose up -d
	@echo "Waiting for service to be ready..."
	@sleep 5
	@echo "Application is running at:"
	@echo "  Dashboard: http://localhost:8080/static/dashboard.html"
	@echo "  Viewer:    http://localhost:8080/static/viewer.html"
	@echo "  Health:    http://localhost:8080/api/v1/health"

# Docker Compose 종료
docker-down:
	@echo "Stopping Docker Compose..."
	cd docker && docker-compose down

# Docker 로그 보기
docker-logs:
	@echo "Showing Docker logs..."
	cd docker && docker-compose logs -f media-server

# Docker Compose 재시작
docker-restart:
	@echo "Restarting Docker Compose..."
	cd docker && docker-compose restart

# Docker 상태 확인
docker-ps:
	@echo "Docker container status:"
	cd docker && docker-compose ps

# 프로덕션 배포 (모니터링 포함)
docker-deploy-prod:
	@echo "Deploying to production with monitoring..."
	cd docker && docker-compose --profile monitoring up -d --build
	@echo "Deployment complete!"
	@echo "  Application: http://localhost:8080"
	@echo "  Prometheus:  http://localhost:9091"
	@echo "  Grafana:     http://localhost:3000 (admin/admin)"

# 빠른 배포 (빌드+실행)
docker-deploy:
	@echo "Quick deployment (build and run)..."
	cd docker && docker-compose up -d --build
	@sleep 5
	@echo "Checking health..."
	@curl -sf http://localhost:8080/api/v1/health || echo "Health check failed!"
	@echo "Deployment complete!"

# Docker 리소스 정리
docker-clean:
	@echo "Cleaning Docker resources..."
	cd docker && docker-compose down -v
	docker rmi $(DOCKER_IMAGE):latest 2>/dev/null || true
	@echo "Docker cleanup complete"

# Docker 전체 재빌드
docker-rebuild:
	@echo "Rebuilding from scratch..."
	$(MAKE) docker-clean
	cd docker && docker-compose build --no-cache
	$(MAKE) docker-up

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
