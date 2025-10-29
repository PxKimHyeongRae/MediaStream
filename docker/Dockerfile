# Multi-stage build for minimal image size

# Build stage
FROM golang:1.21-alpine AS builder

# 빌드에 필요한 패키지 설치
RUN apk add --no-cache git make

# 작업 디렉토리 설정
WORKDIR /build

# 의존성 파일 복사 및 다운로드
COPY go.mod go.sum ./
RUN go mod download

# 소스 코드 복사
COPY . .

# 빌드
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/bin/media-server \
    cmd/server/main.go

# Runtime stage
FROM alpine:latest

# 런타임에 필요한 패키지 설치
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    ffmpeg

# 비특권 사용자 생성
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# 작업 디렉토리 설정
WORKDIR /app

# 빌드 스테이지에서 바이너리 복사
COPY --from=builder /build/bin/media-server /app/media-server

# 설정 파일 디렉토리 생성
RUN mkdir -p /app/configs /app/logs && \
    chown -R appuser:appgroup /app

# 설정 파일 및 웹 리소스 복사
COPY configs/config.yaml /app/configs/
COPY web/static /app/web/static

# 비특권 사용자로 전환
USER appuser

# 포트 노출
EXPOSE 8080 8081 9090

# 헬스체크
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 엔트리포인트
ENTRYPOINT ["/app/media-server"]

# 기본 설정 파일 경로
CMD ["-config", "/app/configs/config.yaml"]
