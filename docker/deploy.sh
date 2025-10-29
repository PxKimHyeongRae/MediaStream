#!/bin/bash
# RTSP to WebRTC Media Server - Deployment Script

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
print_header() {
    echo -e "${BLUE}================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

check_prerequisites() {
    print_header "Checking Prerequisites"

    # Check Docker
    if ! command -v docker &> /dev/null; then
        print_error "Docker not found. Please install Docker first."
        exit 1
    fi
    print_success "Docker found: $(docker --version)"

    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose not found. Please install Docker Compose first."
        exit 1
    fi
    print_success "Docker Compose found: $(docker-compose --version)"

    # Check if running in docker directory
    if [ ! -f "docker-compose.yml" ]; then
        print_error "Please run this script from the docker directory"
        exit 1
    fi
}

check_config() {
    print_header "Checking Configuration"

    # Check if .env exists
    if [ ! -f "../.env" ]; then
        print_warning ".env file not found"
        echo "Creating .env from .env.example..."
        cp ../.env.example ../.env
        print_success "Created .env file. Please edit it with your configuration."
        print_warning "Press Enter to continue after editing .env, or Ctrl+C to exit"
        read
    else
        print_success ".env file exists"
    fi

    # Check if config.yaml exists
    if [ ! -f "../configs/config.yaml" ]; then
        print_error "configs/config.yaml not found"
        exit 1
    fi
    print_success "config.yaml exists"
}

build_image() {
    print_header "Building Docker Image"

    echo "Building image with tag rtsp-webrtc-server:latest..."
    docker-compose build --no-cache

    print_success "Image built successfully"
}

deploy() {
    print_header "Deploying Application"

    # Check if already running
    if [ "$(docker-compose ps -q media-server)" ]; then
        print_warning "Application is already running"
        echo "Do you want to recreate containers? (y/n)"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            echo "Stopping existing containers..."
            docker-compose down
        else
            print_warning "Skipping deployment"
            return
        fi
    fi

    # Start containers
    echo "Starting containers..."
    docker-compose up -d

    print_success "Application deployed successfully"
}

check_health() {
    print_header "Checking Application Health"

    echo "Waiting for application to start..."
    sleep 5

    # Check container status
    if [ ! "$(docker-compose ps -q media-server)" ]; then
        print_error "Container is not running"
        docker-compose logs --tail=20 media-server
        exit 1
    fi

    # Check health endpoint
    local max_attempts=30
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
            print_success "Application is healthy"
            return 0
        fi

        echo -n "."
        sleep 1
        ((attempt++))
    done

    print_error "Health check failed after $max_attempts attempts"
    print_warning "Showing recent logs:"
    docker-compose logs --tail=50 media-server
    exit 1
}

show_info() {
    print_header "Deployment Information"

    echo -e "${GREEN}Application successfully deployed!${NC}\n"

    echo "Access URLs:"
    echo "  Dashboard:  http://localhost:8080/static/dashboard.html"
    echo "  Viewer:     http://localhost:8080/static/viewer.html"
    echo "  API Health: http://localhost:8080/api/v1/health"
    echo "  Metrics:    http://localhost:9090/metrics"
    echo ""

    echo "Management Commands:"
    echo "  View logs:      docker-compose logs -f media-server"
    echo "  Stop:           docker-compose stop"
    echo "  Restart:        docker-compose restart"
    echo "  Remove:         docker-compose down"
    echo "  Check status:   docker-compose ps"
    echo ""

    echo "Container Status:"
    docker-compose ps
}

main() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════╗"
    echo "║  RTSP to WebRTC Media Server Deployment ║"
    echo "╚══════════════════════════════════════════╝"
    echo -e "${NC}"

    check_prerequisites
    check_config

    # Parse arguments
    case "${1:-deploy}" in
        build)
            build_image
            ;;
        deploy)
            deploy
            check_health
            show_info
            ;;
        rebuild)
            build_image
            deploy
            check_health
            show_info
            ;;
        stop)
            print_header "Stopping Application"
            docker-compose stop
            print_success "Application stopped"
            ;;
        down)
            print_header "Removing Containers"
            docker-compose down
            print_success "Containers removed"
            ;;
        logs)
            docker-compose logs -f media-server
            ;;
        health)
            check_health
            ;;
        *)
            echo "Usage: $0 {build|deploy|rebuild|stop|down|logs|health}"
            echo ""
            echo "Commands:"
            echo "  build    - Build Docker image"
            echo "  deploy   - Deploy application (default)"
            echo "  rebuild  - Rebuild and deploy"
            echo "  stop     - Stop containers"
            echo "  down     - Remove containers"
            echo "  logs     - View logs"
            echo "  health   - Check application health"
            exit 1
            ;;
    esac
}

main "$@"
