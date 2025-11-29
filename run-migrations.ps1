# PowerShell script to run PostgreSQL migrations for multi-tenancy

Write-Host "======================================" -ForegroundColor Cyan
Write-Host "Google Maps Scraper - Database Setup" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""

# Load environment variables from .env file
if (Test-Path ".env") {
    Get-Content .env | ForEach-Object {
        if ($_ -match '^([^#][^=]+)=(.+)$') {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

# Database connection parameters with defaults
if ([string]::IsNullOrEmpty($env:POSTGRES_HOST)) { $POSTGRES_HOST = "localhost" } else { $POSTGRES_HOST = $env:POSTGRES_HOST }
if ([string]::IsNullOrEmpty($env:POSTGRES_PORT)) { $POSTGRES_PORT = "5432" } else { $POSTGRES_PORT = $env:POSTGRES_PORT }
if ([string]::IsNullOrEmpty($env:POSTGRES_USER)) { $POSTGRES_USER = "postgres" } else { $POSTGRES_USER = $env:POSTGRES_USER }
if ([string]::IsNullOrEmpty($env:POSTGRES_PASSWORD)) { $POSTGRES_PASSWORD = "postgres" } else { $POSTGRES_PASSWORD = $env:POSTGRES_PASSWORD }
if ([string]::IsNullOrEmpty($env:POSTGRES_DB)) { $POSTGRES_DB = "google_maps_scraper" } else { $POSTGRES_DB = $env:POSTGRES_DB }

Write-Host "Database Configuration:" -ForegroundColor Yellow
Write-Host "  Host: $POSTGRES_HOST" -ForegroundColor White
Write-Host "  Port: $POSTGRES_PORT" -ForegroundColor White
Write-Host "  Database: $POSTGRES_DB" -ForegroundColor White
Write-Host "  User: $POSTGRES_USER" -ForegroundColor White
Write-Host ""

# Set PGPASSWORD environment variable for psql
$env:PGPASSWORD = $POSTGRES_PASSWORD

# Check if PostgreSQL is accessible
Write-Host "Checking PostgreSQL connection..." -ForegroundColor Yellow
$psqlPath = "C:\Program Files\PostgreSQL\17\bin\psql.exe"

$testConnection = & $psqlPath -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d postgres -c "SELECT 1;" 2>&1

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Cannot connect to PostgreSQL!" -ForegroundColor Red
    Write-Host "Please ensure PostgreSQL is running and credentials are correct." -ForegroundColor Red
    Write-Host "Connection string: host=$POSTGRES_HOST port=$POSTGRES_PORT user=$POSTGRES_USER" -ForegroundColor Yellow
    exit 1
}

Write-Host "PostgreSQL connection successful" -ForegroundColor Green
Write-Host ""

# Check if database exists, create if not
Write-Host "Checking if database exists..." -ForegroundColor Yellow
$dbExists = & $psqlPath -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$POSTGRES_DB';"

if ([string]::IsNullOrWhiteSpace($dbExists)) {
    Write-Host "Creating database '$POSTGRES_DB'..." -ForegroundColor Yellow
    & $psqlPath -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d postgres -c "CREATE DATABASE $POSTGRES_DB;"

    if ($LASTEXITCODE -eq 0) {
        Write-Host "Database created successfully" -ForegroundColor Green
    } else {
        Write-Host "ERROR: Failed to create database!" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "Database already exists" -ForegroundColor Green
}

Write-Host ""

# Run migrations
Write-Host "Running multi-tenancy migrations..." -ForegroundColor Yellow
Write-Host ""

$migrationFile = "postgres\migrations\001_multi_tenancy.sql"

if (-not (Test-Path $migrationFile)) {
    Write-Host "ERROR: Migration file not found: $migrationFile" -ForegroundColor Red
    exit 1
}

Write-Host "Executing migration: 001_multi_tenancy.sql" -ForegroundColor Cyan

& $psqlPath -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -f $migrationFile

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Migration completed successfully!" -ForegroundColor Green
} else {
    Write-Host ""
    Write-Host "ERROR: Migration failed!" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "Verifying tables..." -ForegroundColor Yellow

& $psqlPath -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -c "\dt"

Write-Host ""
Write-Host "======================================" -ForegroundColor Cyan
Write-Host "Database setup complete!" -ForegroundColor Green
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Multi-tenancy tables created:" -ForegroundColor White
Write-Host "  - organizations" -ForegroundColor Cyan
Write-Host "  - users" -ForegroundColor Cyan
Write-Host "  - organization_members" -ForegroundColor Cyan
Write-Host "  - user_sessions" -ForegroundColor Cyan
Write-Host "  - organization_invitations" -ForegroundColor Cyan
Write-Host "  - audit_logs" -ForegroundColor Cyan
Write-Host ""
Write-Host "You can now run the application in web mode." -ForegroundColor White
Write-Host ""
