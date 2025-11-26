#!/bin/bash
set -e

# 스크립트 위치 기준으로 프로젝트 루트 경로 설정
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 이미지 이름 및 태그
IMAGE_NAME="media-server"
IMAGE_TAG="${1:-latest}"

echo "Building Docker image: ${IMAGE_NAME}:${IMAGE_TAG}"
echo "Project root: ${PROJECT_ROOT}"

# 프로젝트 루트에서 빌드
cd "$PROJECT_ROOT"

# 도커 이미지 빌드
docker build \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    -f docker/Dockerfile \
    .

echo ""
echo "✅ Build completed successfully!"
echo "   Image: ${IMAGE_NAME}:${IMAGE_TAG}"
echo ""
echo "To run the container:"
echo "  docker run -d --name media-server -p 8107:8107 ${IMAGE_NAME}:${IMAGE_TAG}"
echo ""
echo "To deploy with docker-compose:"
echo "  cd ~/docker/aiot"
echo "  docker-compose up -d media-server"
