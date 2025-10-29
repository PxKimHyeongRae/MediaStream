# Docker Deployment Guide

This guide explains how to deploy the RTSP to WebRTC Media Server using Docker Compose.

## 📋 Prerequisites

- Docker 20.10+ and Docker Compose 1.29+
- Network access to RTSP cameras
- At least 512MB RAM available
- Ports 8080, 8081, 9090 available

## 🚀 Quick Start

### 1. Clone and Setup

```bash
cd /path/to/cctv3

# Copy environment template
cp .env.example .env

# Edit with your camera credentials
nano .env
```

### 2. Configure Cameras

Edit `configs/config.yaml` with your RTSP camera URLs:

```yaml
paths:
  camera_01:
    source: rtsp://admin:password@192.168.1.100:554/stream
    sourceOnDemand: no
    rtspTransport: tcp
```

**Important**: URL-encode special characters in passwords:
- `!` → `%21`
- `@` → `%40`
- `#` → `%23`

### 3. Build and Run

```bash
# Navigate to docker directory
cd docker

# Build and start
docker-compose up -d

# Check logs
docker-compose logs -f media-server

# Check health
docker-compose ps
```

### 4. Access Web UI

- **Dashboard** (All Cameras): http://localhost:8080/static/dashboard.html
- **Single Viewer**: http://localhost:8080/static/viewer.html
- **API Health**: http://localhost:8080/api/v1/health
- **Prometheus Metrics**: http://localhost:9090/metrics

## 📦 Deployment Modes

### Development Mode (Default)

```bash
docker-compose up
```

- Real-time logs visible
- Auto-reload not supported (rebuild required)
- For testing and debugging

### Production Mode

```bash
docker-compose up -d
```

- Runs in background
- Auto-restart on failure
- Resource limits applied

### Production with Monitoring

```bash
docker-compose --profile monitoring up -d
```

Includes:
- **Prometheus**: http://localhost:9091
- **Grafana**: http://localhost:3000 (admin/admin)

## 🔧 Configuration

### Environment Variables

Create `.env` file from `.env.example`:

```env
# Server
HTTP_PORT=8080
WS_PORT=8081
METRICS_PORT=9090

# Cameras
CAMERA_01_SOURCE=rtsp://admin:password@192.168.1.100:554/stream
CAMERA_01_ON_DEMAND=no

# Performance
MAX_STREAMS=100
MAX_PEERS=1000
```

### Custom Config File

Use a custom config file:

```yaml
# docker-compose.override.yml
version: '3.8'

services:
  media-server:
    volumes:
      - ./config.production.yaml:/app/configs/config.yaml:ro
```

Then run:
```bash
docker-compose -f docker-compose.yml -f docker-compose.override.yml up -d
```

## 🔍 Monitoring

### Health Check

```bash
# Check container health
docker-compose ps

# Check application health
curl http://localhost:8080/api/v1/health
```

### Logs

```bash
# All logs
docker-compose logs -f

# Media server only
docker-compose logs -f media-server

# Last 100 lines
docker-compose logs --tail=100 media-server

# Since 10 minutes ago
docker-compose logs --since 10m media-server
```

### Resource Usage

```bash
# CPU and memory
docker stats rtsp-webrtc-server

# Detailed info
docker-compose top
```

## 🛠️ Management Commands

### Start/Stop

```bash
# Start
docker-compose up -d

# Stop
docker-compose stop

# Stop and remove containers
docker-compose down

# Stop and remove volumes (⚠️ deletes logs)
docker-compose down -v
```

### Restart

```bash
# Restart all
docker-compose restart

# Restart media server only
docker-compose restart media-server
```

### Update

```bash
# Pull latest code
git pull

# Rebuild image
docker-compose build --no-cache

# Recreate containers
docker-compose up -d --force-recreate
```

### Cleanup

```bash
# Remove stopped containers
docker-compose rm

# Remove unused images
docker image prune

# Full cleanup (⚠️ removes all unused Docker data)
docker system prune -a
```

## 📊 Performance Tuning

### Resource Limits

Edit `docker-compose.yml`:

```yaml
deploy:
  resources:
    limits:
      cpus: '4.0'      # Max 4 CPUs
      memory: 2G       # Max 2GB RAM
    reservations:
      cpus: '1.0'      # Min 1 CPU
      memory: 512M     # Min 512MB RAM
```

### Network Performance

For better network performance, use host network mode:

```yaml
# docker-compose.override.yml
version: '3.8'

services:
  media-server:
    network_mode: host
```

**Note**: Requires adjusting firewall rules.

## 🔐 Security Best Practices

### 1. Non-Root User

Container runs as non-root user `appuser` (already configured).

### 2. Read-Only Config

Config volume is mounted as read-only (`:ro`).

### 3. Secrets Management

Use Docker secrets for sensitive data:

```bash
echo "rtsp://user:pass@host:554/stream" | docker secret create camera_01_url -
```

```yaml
services:
  media-server:
    secrets:
      - camera_01_url
```

### 4. Network Isolation

Keep containers in isolated network:

```yaml
networks:
  media-network:
    driver: bridge
    internal: true  # No external access
```

### 5. HTTPS/WSS (Production)

Use reverse proxy (nginx, Traefik, Caddy):

```yaml
services:
  nginx:
    image: nginx:alpine
    ports:
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
```

## 🐛 Troubleshooting

### Container Won't Start

```bash
# Check logs
docker-compose logs media-server

# Check container status
docker-compose ps

# Inspect container
docker inspect rtsp-webrtc-server
```

### Camera Connection Failed

```bash
# Test RTSP URL with ffmpeg
docker run --rm -it jrottenberg/ffmpeg:4.4-alpine \
  -rtsp_transport tcp -i "rtsp://admin:password@192.168.1.100:554/stream" \
  -frames:v 1 -f null -

# Check if camera is reachable
docker run --rm -it nicolaka/netshoot \
  nc -zv 192.168.1.100 554
```

### WebRTC Connection Failed

1. Check browser console for errors
2. Verify STUN server connectivity
3. Check firewall rules for UDP ports
4. Try different network (NAT issues)

### High Memory Usage

```bash
# Check memory stats
docker stats rtsp-webrtc-server

# Reduce GC pressure (edit .env)
GC_PERCENT=50

# Limit max memory
docker-compose up -d --scale media-server=1 --memory 512m
```

### Permission Denied Errors

```bash
# Fix log directory permissions
sudo chown -R 1000:1000 ./logs

# Or run as root (not recommended)
docker-compose run --user root media-server
```

## 📁 Volume Management

### Persistent Data

```bash
# List volumes
docker volume ls

# Inspect logs volume
docker volume inspect docker_media-logs

# Backup logs
docker run --rm -v docker_media-logs:/data -v $(pwd):/backup \
  alpine tar czf /backup/logs-backup.tar.gz -C /data .

# Restore logs
docker run --rm -v docker_media-logs:/data -v $(pwd):/backup \
  alpine tar xzf /backup/logs-backup.tar.gz -C /data
```

### Cleanup Old Logs

```bash
# Enter container
docker-compose exec media-server sh

# Clean logs older than 7 days
find /app/logs -name "*.log" -mtime +7 -delete
```

## 🚢 Production Deployment Checklist

- [ ] Update `.env` with production values
- [ ] Configure RTSP camera URLs with URL-encoded passwords
- [ ] Set `PRODUCTION=true` in environment
- [ ] Configure resource limits
- [ ] Setup HTTPS/WSS reverse proxy
- [ ] Enable monitoring profile
- [ ] Configure log rotation
- [ ] Setup backup for logs volume
- [ ] Configure firewall rules
- [ ] Test health check endpoint
- [ ] Test camera connections
- [ ] Test WebRTC connections from external network
- [ ] Setup alerting (Prometheus + Alertmanager)
- [ ] Document access credentials
- [ ] Plan upgrade/rollback strategy

## 📚 Additional Resources

- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [Docker Best Practices](https://docs.docker.com/develop/dev-best-practices/)
- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)

## 🆘 Getting Help

If you encounter issues:

1. Check logs: `docker-compose logs -f media-server`
2. Check health: `docker-compose ps`
3. Check project [CLAUDE.md](../CLAUDE.md)
4. Check skill [.claude/skills/rtsp-webrtc-streaming.md](../.claude/skills/rtsp-webrtc-streaming.md)
5. Check [GitHub Issues](https://github.com/your-repo/issues)

---

**Last Updated**: 2025-10-29
**Docker Version**: 20.10+
**Docker Compose Version**: 1.29+
