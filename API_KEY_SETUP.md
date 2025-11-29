# API Key Authentication Setup

This guide explains how to set up and use API key authentication for the Google Maps Scraper API.

## Overview

The Google Maps Scraper now supports API key authentication to secure your API endpoints. This ensures that only authorized users with valid API keys can make API calls.

## Features

- ✅ Secure API key generation with SHA-256 hashing
- ✅ Multiple authentication methods (Bearer token, X-API-Key header, query parameter)
- ✅ Web UI for managing API keys
- ✅ Support for API key expiration
- ✅ Track last usage of API keys
- ✅ Revoke API keys instantly
- ✅ Support for both SQLite (web mode) and PostgreSQL (database mode)

## Database Setup

### For PostgreSQL (Database Mode)

1. Run the migration to create the API keys table:

```bash
# Assuming you have a PostgreSQL migration tool set up
# The migration file is located at: scripts/migrations/0005_create_api_keys.up.sql
```

Or manually execute:

```sql
CREATE TABLE IF NOT EXISTS api_keys(
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_status ON api_keys(status);
```

### For SQLite (Web Mode)

The API keys table is automatically created when you start the web server. The schema is included in the SQLite initialization.

## Enabling API Key Authentication

### Web Runner Mode (SQLite)

To enable API key authentication in web mode, you need to initialize the API key service when creating the web server.

Example integration in `runner/webrunner/webrunner.go`:

```go
package webrunner

import (
    "github.com/gosom/google-maps-scraper/web"
    "github.com/gosom/google-maps-scraper/web/sqlite"
)

func New(cfg *runner.Config) (runner.Runner, error) {
    // ... existing code ...

    // Initialize database
    db, err := sqlite.InitDB(dbpath)
    if err != nil {
        return nil, err
    }

    // Create job repository
    repo := sqlite.NewWithDB(db)
    svc := web.NewService(repo, cfg.DataFolder)

    // Create API key repository and service
    apiKeyRepo := sqlite.NewAPIKeyRepository(db)
    apiKeySvc := web.NewAPIKeyService(apiKeyRepo)

    // Create server with API key support
    srv, err := web.NewWithAPIKeys(svc, apiKeySvc, cfg.Addr)
    if err != nil {
        return nil, err
    }

    // ... rest of the code ...
}
```

### Database Runner Mode (PostgreSQL)

Similar integration can be done for PostgreSQL by using the PostgreSQL API key repository:

```go
import (
    "github.com/gosom/google-maps-scraper/postgres"
)

// After opening PostgreSQL connection
apiKeyRepo := postgres.NewAPIKeyRepository(conn)
apiKeySvc := web.NewAPIKeyService(apiKeyRepo)
```

## Using the API Key Management UI

Once API key authentication is enabled:

1. Start your server
2. Navigate to `http://localhost:8080/apikeys` (or your configured address)
3. Create a new API key:
   - Enter a descriptive name (e.g., "Production Key", "Development Key")
   - Optionally set an expiration date
   - Click "Create API Key"
4. **Important**: Copy the generated API key immediately - it will only be shown once!
5. Save the API key securely (e.g., in your password manager or environment variables)

## Making Authenticated API Requests

Once you have an API key, you can use it in three ways:

### Method 1: Bearer Token (Recommended)

```bash
curl -H "Authorization: Bearer gms_YOUR_API_KEY_HERE" \
  http://localhost:8080/api/v1/jobs
```

### Method 2: X-API-Key Header

```bash
curl -H "X-API-Key: gms_YOUR_API_KEY_HERE" \
  http://localhost:8080/api/v1/jobs
```

### Method 3: Query Parameter

```bash
curl "http://localhost:8080/api/v1/jobs?api_key=gms_YOUR_API_KEY_HERE"
```

## API Key Management

### Create API Key

**Web UI**: Go to `/apikeys` and fill out the form

**API Endpoint**:
```bash
curl -X POST http://localhost:8080/api/v1/apikeys \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Key",
    "expires_at": "2025-12-31T23:59:59Z"
  }'
```

Response:
```json
{
  "id": "uuid",
  "name": "Production Key",
  "key": "gms_BASE64_ENCODED_KEY",
  "status": "active",
  "created_at": "2025-01-01T00:00:00Z",
  "message": "API key created successfully. Please save this key as it will not be shown again."
}
```

### List API Keys

**Web UI**: Navigate to `/apikeys`

**API Endpoint**:
```bash
curl -H "Authorization: Bearer gms_YOUR_API_KEY" \
  http://localhost:8080/api/v1/apikeys
```

### Revoke API Key

**Web UI**: Click the "Revoke" button next to the API key

**API Endpoint**:
```bash
curl -X POST \
  -H "Authorization: Bearer gms_YOUR_API_KEY" \
  http://localhost:8080/api/v1/apikeys/{id}/revoke
```

### Delete API Key

**Web UI**: Click the "Delete" button next to the API key

**API Endpoint**:
```bash
curl -X DELETE \
  -H "Authorization: Bearer gms_YOUR_API_KEY" \
  http://localhost:8080/api/v1/apikeys/{id}
```

## Security Best Practices

1. **Never commit API keys to version control**
   - Add API keys to `.gitignore`
   - Use environment variables for storing keys

2. **Use environment variables**
   ```bash
   export GMAPS_API_KEY="gms_YOUR_API_KEY_HERE"
   curl -H "Authorization: Bearer $GMAPS_API_KEY" http://localhost:8080/api/v1/jobs
   ```

3. **Rotate API keys regularly**
   - Create new keys periodically
   - Revoke old keys after migration

4. **Set expiration dates**
   - Use short-lived keys for temporary access
   - Set appropriate expiration dates for production keys

5. **Monitor key usage**
   - Check the "Last Used" column in the API keys page
   - Revoke unused keys

6. **Use HTTPS in production**
   - Always use HTTPS to prevent key interception
   - Consider using a reverse proxy like Nginx or Caddy

## Troubleshooting

### "API key is required" error

- Make sure you're including the API key in your request
- Check that you're using the correct header format or query parameter

### "Invalid or expired API key" error

- Verify the API key is correct (no extra spaces or characters)
- Check if the key has been revoked in the web UI
- Check if the key has expired

### API keys not working after server restart

- The API keys are stored in the database and persist across restarts
- If using SQLite, ensure the database file is in the data folder
- Check database permissions

### Cannot access /apikeys page

- Ensure API key service is initialized in your runner
- Check server logs for initialization errors

## Example: Complete Workflow

### 1. Start the server with API key support enabled

```bash
./google-maps-scraper --run-mode web --data-folder ./data
```

### 2. Create an API key via the web UI

Navigate to `http://localhost:8080/apikeys` and create a key named "My App Key"

### 3. Save the generated key

```bash
export GMAPS_API_KEY="gms_abc123..."
```

### 4. Make an authenticated API request

```bash
# Create a scraping job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Authorization: Bearer $GMAPS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coffee shops in NYC",
    "keywords": ["coffee shop"],
    "lang": "en",
    "zoom": 15,
    "depth": 10,
    "max_time": "10m"
  }'
```

### 5. Check job status

```bash
curl -H "Authorization: Bearer $GMAPS_API_KEY" \
  http://localhost:8080/api/v1/jobs
```

## API Endpoints Protected by Authentication

When API key authentication is enabled, the following endpoints require a valid API key:

- `POST /api/v1/jobs` - Create scraping job
- `GET /api/v1/jobs` - List all jobs
- `GET /api/v1/jobs/{id}` - Get job details
- `DELETE /api/v1/jobs/{id}` - Delete job
- `GET /api/v1/jobs/{id}/download` - Download job results (CSV)
- `GET /api/v1/jobs/{id}/results` - Get job results (JSON)
- `POST /api/v1/jobs/{id}/retry` - Retry failed job
- `POST /api/v1/jobs/import` - Import data from CSV/JSON

## Disabling API Key Authentication

To run the server without API key authentication (not recommended for production):

Simply use the standard `web.New()` function instead of `web.NewWithAPIKeys()`, and the API endpoints will be accessible without authentication.

## Support

For issues or questions:
- GitHub Issues: https://github.com/gosom/google-maps-scraper/issues
- Documentation: https://github.com/gosom/google-maps-scraper
