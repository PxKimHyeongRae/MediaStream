#!/bin/sh
set -e

# 로그 디렉토리가 존재하는지 확인하고 권한 설정
if [ -d "/app/logs" ]; then
    echo "Checking log directory permissions..."
    # 디렉토리가 쓰기 가능한지 확인
    if [ ! -w "/app/logs" ]; then
        echo "Warning: /app/logs is not writable by current user"
        echo "Creating logs directory with proper permissions..."
        mkdir -p /app/logs 2>/dev/null || true
    fi
else
    echo "Creating logs directory..."
    mkdir -p /app/logs
fi

# 로그 디렉토리 확인
echo "Log directory status:"
ls -ld /app/logs || true

# 애플리케이션 실행
exec "$@"
