# 🐳 Docker Deployment Guide

Complete guide for deploying Google Maps Scraper with Docker.

---

## 📋 Table of Contents

1. [Quick Start](#quick-start)
2. [Development Setup](#development-setup)
3. [Production Deployment](#production-deployment)
4. [Configuration](#configuration)
5. [Maintenance](#maintenance)
6. [Troubleshooting](#troubleshooting)
7. [Advanced Usage](#advanced-usage)

---

## 🚀 Quick Start

Get up and running in 3 minutes:

### Prerequisites

- Docker 20.10+ ([Install Docker](https://docs.docker.com/get-docker/))
- Docker Compose 2.0+ ([Install Compose](https://docs.docker.com/compose/install/))
- 4GB+ RAM available
- 10GB+ disk space

### 1. Clone & Configure

```bash
cd google-maps-scraper

# Copy environment template
cp .env.docker .env

# Edit .env and add your API key
nano .env  # or use your preferred editor
```

**Required**: Set `GOOGLE_MAPS_API_KEY` in `.env`

### 2. Start the Stack

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f
```

### 3. Access the Application

- **Web UI**: http://localhost:8080
- **Register**: http://localhost:8080/register
- **Login**: http://localhost:8080/login
- **API Docs**: http://localhost:8080/api/swagger

---

## 🛠️ Development Setup

### Using docker-compose.yaml (Development)

The default `docker-compose.yaml` is configured for local development:

```yaml
Services:
├── db          PostgreSQL 15.2-alpine
├── migrate     Database migrations (auto-runs)
└── app         Google Maps Scraper
```

**Start services:**
```bash
docker-compose up -d
```

**View logs:**
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f app
docker-compose logs -f db
```

**Stop services:**
```bash
docker-compose down

# Stop and remove volumes (⚠️ deletes all data)
docker-compose down -v
```

**Rebuild after code changes:**
```bash
docker-compose up -d --build
```

---

## 🏭 Production Deployment

### Using docker-compose.prod.yaml (Production)

The production compose file includes:
- Enhanced security settings
- Resource limits and reservations
- Optimized logging
- Optional Nginx reverse proxy
- Health checks and restart policies

### 1. Create Production Environment File

```bash
cp .env.docker .env.prod
```

Edit `.env.prod` and set:

```bash
# Required
GOOGLE_MAPS_API_KEY=your-api-key-here
POSTGRES_PASSWORD=strong-password-here
SESSION_SECRET=random-32-char-string-here

# Optional
POSTGRES_USER=gmaps_user
POSTGRES_DB=google_maps_scraper
MAX_CONCURRENT_JOBS=10
LOG_LEVEL=info
```

**Generate SESSION_SECRET:**
```bash
# Linux/Mac
openssl rand -base64 32

# Windows PowerShell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Maximum 256 }))
```

### 2. Deploy

```bash
# Start with production config
docker-compose -f docker-compose.prod.yaml --env-file .env.prod up -d

# View logs
docker-compose -f docker-compose.prod.yaml logs -f
```

### 3. Enable Nginx Reverse Proxy (Optional)

First, create nginx configuration:

```bash
mkdir -p config
```

Create `config/nginx.conf`:

```nginx
events {
    worker_connections 1024;
}

http {
    upstream app {
        server app:8080;
    }

    server {
        listen 80;
        server_name yourdomain.com;

        # Redirect to HTTPS
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name yourdomain.com;

        ssl_certificate /etc/nginx/ssl/cert.pem;
        ssl_certificate_key /etc/nginx/ssl/key.pem;

        location / {
            proxy_pass http://app;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
```

Start with Nginx:
```bash
docker-compose -f docker-compose.prod.yaml --profile with-nginx up -d
```

---

## ⚙️ Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GOOGLE_MAPS_API_KEY` | ✅ Yes | - | Google Maps API key |
| `POSTGRES_PASSWORD` | ✅ Production | `postgres` | Database password |
| `SESSION_SECRET` | ✅ Production | - | Session encryption key |
| `POSTGRES_USER` | No | `postgres` | Database user |
| `POSTGRES_DB` | No | `google_maps_scraper` | Database name |
| `PROXY` | No | - | Proxy URL |
| `MAX_CONCURRENT_JOBS` | No | `5` | Max parallel scraping jobs |
| `JOB_TIMEOUT` | No | `60` | Job timeout (minutes) |
| `LOG_LEVEL` | No | `info` | Logging level |

### Volume Mounts

**Default Volumes:**
- `postgres_data`: PostgreSQL database files
- `app_data`: Application data (results, SQLite fallback)

**Custom Data Folder:**
```yaml
# In docker-compose.yaml, replace:
volumes:
  - app_data:/data
# With:
volumes:
  - ./gmapsdata:/data
```

---

## 🔧 Maintenance

### Backup Database

```bash
# Create backup
docker-compose exec db pg_dump -U postgres google_maps_scraper > backup.sql

# With compression
docker-compose exec db pg_dump -U postgres google_maps_scraper | gzip > backup.sql.gz
```

### Restore Database

```bash
# From SQL file
cat backup.sql | docker-compose exec -T db psql -U postgres google_maps_scraper

# From compressed file
gunzip -c backup.sql.gz | docker-compose exec -T db psql -U postgres google_maps_scraper
```

### View Database

```bash
# Connect to PostgreSQL
docker-compose exec db psql -U postgres -d google_maps_scraper

# List tables
\dt

# Query data
SELECT * FROM organizations;
SELECT * FROM jobs LIMIT 10;

# Exit
\q
```

### Update Application

```bash
# Pull latest code
git pull

# Rebuild and restart
docker-compose up -d --build

# View updated services
docker-compose ps
```

### Database Migrations

Migrations run automatically on startup. To run manually:

```bash
# Run migrations
docker-compose run --rm migrate \
  -path /migrations \
  -database "postgres://postgres:postgres@db:5432/google_maps_scraper?sslmode=disable" \
  up

# Rollback last migration
docker-compose run --rm migrate \
  -path /migrations \
  -database "postgres://postgres:postgres@db:5432/google_maps_scraper?sslmode=disable" \
  down 1
```

### Clean Up

```bash
# Stop all services
docker-compose down

# Remove all containers and volumes (⚠️ DELETES DATA)
docker-compose down -v

# Remove unused images
docker image prune -a

# Remove build cache
docker builder prune
```

---

## 🐛 Troubleshooting

### Application Won't Start

**Check logs:**
```bash
docker-compose logs app
```

**Common issues:**
1. **Missing API Key**: Add `GOOGLE_MAPS_API_KEY` to `.env`
2. **Database not ready**: Wait for migrations to complete
3. **Port conflict**: Another service using port 8080

**Solution:**
```bash
# Stop all services
docker-compose down

# Check what's using port 8080
netstat -ano | findstr :8080  # Windows
lsof -i :8080                 # Linux/Mac

# Restart with fresh state
docker-compose up -d
```

### Database Connection Failed

**Check database health:**
```bash
docker-compose ps
docker-compose logs db
```

**Test connection:**
```bash
docker-compose exec db pg_isready -U postgres
```

**Recreate database:**
```bash
docker-compose down
docker volume rm google-maps-scraper_postgres_data
docker-compose up -d
```

### Migrations Failed

**View migration logs:**
```bash
docker-compose logs migrate
```

**Manual migration:**
```bash
# Enter database
docker-compose exec db psql -U postgres -d google_maps_scraper

# Check schema_migrations table
SELECT * FROM schema_migrations;

# Exit and re-run migrations
docker-compose restart migrate
```

### Out of Memory

**Increase Docker memory:**
- Docker Desktop → Settings → Resources → Memory → 4GB+

**Check container resources:**
```bash
docker stats
```

**Reduce concurrent jobs:**
```bash
# In .env
MAX_CONCURRENT_JOBS=2
```

### Slow Performance

**Optimize PostgreSQL:**

Create `config/postgresql.conf`:
```
shared_buffers = 256MB
effective_cache_size = 1GB
maintenance_work_mem = 64MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100
random_page_cost = 1.1
work_mem = 10MB
min_wal_size = 1GB
max_wal_size = 4GB
```

Mount in docker-compose:
```yaml
db:
  volumes:
    - ./config/postgresql.conf:/etc/postgresql/postgresql.conf
  command: postgres -c config_file=/etc/postgresql/postgresql.conf
```

---

## 🎓 Advanced Usage

### Scaling

```bash
# Run multiple app instances
docker-compose up -d --scale app=3

# With load balancer
docker-compose -f docker-compose.prod.yaml --profile with-nginx up -d
```

### Custom Network

```bash
# Create external network
docker network create gmaps-prod-network

# Update docker-compose.yaml
networks:
  gmaps-network:
    external: true
    name: gmaps-prod-network
```

### Health Monitoring

```bash
# Check health status
docker-compose ps

# Watch health checks
watch -n 5 docker-compose ps

# Export metrics (if monitoring tools are configured)
docker stats --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
```

### Remote Deployment

```bash
# Build locally
docker-compose build

# Save image
docker save google-maps-scraper:latest | gzip > gmaps.tar.gz

# Copy to server
scp gmaps.tar.gz user@server:/path/

# Load on server
gunzip -c gmaps.tar.gz | docker load

# Start services on server
docker-compose -f docker-compose.prod.yaml up -d
```

### Docker Swarm

```bash
# Initialize swarm
docker swarm init

# Deploy stack
docker stack deploy -c docker-compose.prod.yaml gmaps

# Check services
docker service ls

# Scale service
docker service scale gmaps_app=5

# View logs
docker service logs -f gmaps_app
```

---

## 📊 Architecture

```
┌─────────────────────────────────────────────┐
│              Docker Host                     │
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │         gmaps-network (bridge)       │   │
│  │                                      │   │
│  │  ┌──────────┐  ┌──────────┐         │   │
│  │  │    db    │  │ migrate  │         │   │
│  │  │ postgres │◄─┤ runs on  │         │   │
│  │  │  :5432   │  │  startup │         │   │
│  │  └────▲─────┘  └──────────┘         │   │
│  │       │                              │   │
│  │       │                              │   │
│  │  ┌────┴─────┐                        │   │
│  │  │   app    │                        │   │
│  │  │ scraper  │                        │   │
│  │  │  :8080   │◄─── External Access    │   │
│  │  └──────────┘                        │   │
│  │                                      │   │
│  └──────────────────────────────────────┘   │
│                                              │
│  Volumes:                                    │
│  ├─ postgres_data (database persistence)    │
│  └─ app_data (application data)             │
│                                              │
└─────────────────────────────────────────────┘
```

---

## 🔐 Security Checklist

- [ ] Set strong `POSTGRES_PASSWORD`
- [ ] Set random `SESSION_SECRET` (32+ chars)
- [ ] Use `.env.prod` for production (not `.env`)
- [ ] Don't commit `.env` files to git
- [ ] Bind to `127.0.0.1` instead of `0.0.0.0` in production
- [ ] Use HTTPS/TLS (via nginx or reverse proxy)
- [ ] Enable firewall on host
- [ ] Regular database backups
- [ ] Keep Docker images updated
- [ ] Review logs regularly
- [ ] Implement rate limiting
- [ ] Use secrets management in production

---

## 📚 Additional Resources

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Reference](https://docs.docker.com/compose/compose-file/)
- [PostgreSQL Docker Hub](https://hub.docker.com/_/postgres)
- [Google Maps Scraper README](README.md)
- [Multi-Tenancy Implementation](MULTI_TENANCY_IMPLEMENTATION.md)

---

## 🆘 Getting Help

If you encounter issues:

1. Check logs: `docker-compose logs -f`
2. Review this guide's [Troubleshooting](#troubleshooting) section
3. Open an issue: https://github.com/gosom/google-maps-scraper/issues
4. Join Discord: [Community Server](https://discord.gg/fpaAVhNCCu)

---

**Happy Scraping!** 🎉
