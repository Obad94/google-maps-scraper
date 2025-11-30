# 🚀 Docker Quick Start

Get Google Maps Scraper running with Docker in 3 minutes.

---

## ⚡ Quick Commands

### Development Mode

```bash
# 1. Configure environment
cp .env.docker .env
# Edit .env and add your GOOGLE_MAPS_API_KEY

# 2. Start everything
docker-compose up -d

# 3. View logs
docker-compose logs -f

# 4. Access
# → Web UI: http://localhost:8080
# → Register: http://localhost:8080/register
```

**Using Make:**
```bash
make docker-up       # Start services
make docker-logs     # View logs
make docker-down     # Stop services
```

---

## 📦 What's Included

```
docker-compose up -d starts:
├── PostgreSQL 15.2 (port 5432)
├── Database Migrations (auto-runs)
└── Google Maps Scraper (port 8080)
```

**Volumes:**
- `postgres_data`: Database storage (persists after restart)
- `app_data`: Application data & results

---

## 🔧 Common Operations

```bash
# View running services
docker-compose ps

# Restart services
docker-compose restart

# Rebuild after code changes
docker-compose up -d --build

# Stop everything
docker-compose down

# Stop and delete data (⚠️ careful!)
docker-compose down -v

# View database
docker-compose exec db psql -U postgres -d google_maps_scraper

# Backup database
docker-compose exec db pg_dump -U postgres google_maps_scraper > backup.sql
```

---

## 🏭 Production Deployment

```bash
# 1. Create production environment
cp .env.docker .env.prod

# 2. Set required variables in .env.prod
GOOGLE_MAPS_API_KEY=your-key
POSTGRES_PASSWORD=strong-password
SESSION_SECRET=random-32-chars

# 3. Deploy
docker-compose -f docker-compose.prod.yaml --env-file .env.prod up -d
```

**Using Make:**
```bash
make docker-prod-up     # Start production
make docker-prod-logs   # View logs
make docker-prod-down   # Stop production
```

---

## 🐛 Troubleshooting

| Problem | Solution |
|---------|----------|
| Port 8080 in use | Change in docker-compose.yaml: `"8081:8080"` |
| Can't connect to DB | Wait 30s for migrations: `docker-compose logs migrate` |
| Out of memory | Docker → Settings → Resources → 4GB RAM |
| Changes not applied | Rebuild: `docker-compose up -d --build` |

**Check logs:**
```bash
docker-compose logs app     # Application logs
docker-compose logs db      # Database logs
docker-compose logs migrate # Migration logs
```

---

## 📊 Make Commands Reference

```bash
make docker-build          # Build Docker image
make docker-up             # Start dev services
make docker-down           # Stop services
make docker-logs           # View logs
make docker-restart        # Restart services
make docker-rebuild        # Rebuild & restart
make docker-ps             # Show containers
make docker-shell-app      # Shell into app
make docker-shell-db       # PostgreSQL shell
make docker-backup-db      # Backup database
make docker-stats          # Resource usage
make docker-clean          # Remove volumes ⚠️
make docker-prod-up        # Start production
```

---

## 📚 Full Documentation

For complete documentation, see [DOCKER_DEPLOYMENT.md](DOCKER_DEPLOYMENT.md)

---

## ✅ Verify Installation

```bash
# All services should show "Up" and "healthy"
docker-compose ps

# Should show:
# gmaps-postgres   Up (healthy)
# gmaps-migrate    Exit 0
# gmaps-scraper    Up (healthy)
```

**Test the application:**
```bash
curl http://localhost:8080/health
# or
wget -O- http://localhost:8080/health
```

---

## 🎯 Next Steps

1. Register your first user: http://localhost:8080/register
2. Create an organization
3. Start your first scraping job
4. View results: http://localhost:8080

---

**Need help?** See [DOCKER_DEPLOYMENT.md](DOCKER_DEPLOYMENT.md) or open an issue.
