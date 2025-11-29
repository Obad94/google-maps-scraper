# Multi-Tenancy Integration Guide

## Overview
This guide explains how to integrate the multi-tenancy system into the existing Google Maps Scraper application.

## Files Created

### Database Migration
- ✅ `postgres/migrations/001_multi_tenancy.sql` - Complete database schema

### Models (web/ package)
- ✅ `web/organization.go` - Organization model
- ✅ `web/user.go` - User model
- ✅ `web/organization_member.go` - Member model with RBAC
- ✅ `web/session.go` - Session model
- ✅ `web/invitation.go` - Invitation model
- ✅ `web/audit_log.go` - Audit log model

### Repositories (postgres/ package)
- ✅ `postgres/organization_repository.go` - Organization CRUD
- ✅ `postgres/user_repository.go` - User CRUD
- ✅ `postgres/organization_member_repository.go` - Member management
- ✅ `postgres/session_repository.go` - Session management
- ✅ `postgres/invitation_repository.go` - Invitation management
- ✅ `postgres/audit_log_repository.go` - Audit logging

### Services (web/ package)
- ✅ `web/auth_service.go` - Authentication and session management
- ✅ `web/organization_service.go` - Organization operations
- ✅ `web/member_service.go` - Member and invitation operations

## Integration Steps

### Step 1: Run Database Migration

```bash
# Connect to your PostgreSQL database
psql -U your_user -d your_database

# Run the migration
\i postgres/migrations/001_multi_tenancy.sql

# Verify tables were created
\dt
```

### Step 2: Update Existing Models

#### Update `web/job.go`

Add organization fields to the `Job` struct:

```go
type Job struct {
    ID             string
    OrganizationID string    // ADD THIS
    CreatedBy      string    // ADD THIS
    Name           string
    Date           time.Time
    UpdatedAt      time.Time
    Status         string
    Data           JobData
}
```

Update validation to require organization:

```go
func (j *Job) Validate() error {
    if j.ID == "" {
        return errors.New("missing id")
    }

    if j.OrganizationID == "" {  // ADD THIS
        return errors.New("missing organization_id")
    }

    // ... rest of validation
}
```

#### Update `web/apikey.go`

Add organization fields to the `APIKey` struct:

```go
type APIKey struct {
    ID             string
    OrganizationID string    // ADD THIS
    CreatedBy      string    // ADD THIS
    Name           string
    Key            string
    KeyHash        string
    Status         string
    CreatedAt      time.Time
    UpdatedAt      time.Time
    LastUsedAt     *time.Time
    ExpiresAt      *time.Time
}
```

#### Update `web/job.go` SelectParams

Add organization filter:

```go
type SelectParams struct {
    OrganizationID string  // ADD THIS
    Status         string
    Limit          int
}
```

### Step 3: Update Repositories

#### Update `web/sqlite/sqlite.go` (or create new PostgreSQL job repository)

Add organization filtering to all queries:

```go
func (r *repo) Select(ctx context.Context, params web.SelectParams) ([]web.Job, error) {
    q := `
        SELECT id, organization_id, created_by, name, status, data, created_at, updated_at
        FROM jobs
        WHERE deleted_at IS NULL
    `

    args := []interface{}{}
    argCount := 1

    // ADD ORGANIZATION FILTER
    if params.OrganizationID != "" {
        q += fmt.Sprintf(" AND organization_id = $%d", argCount)
        args = append(args, params.OrganizationID)
        argCount++
    }

    // ... rest of query
}
```

Update `Create` method:

```go
func (r *repo) Create(ctx context.Context, job *web.Job) error {
    q := `
        INSERT INTO jobs (id, organization_id, created_by, name, status, data, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `

    _, err := r.db.ExecContext(ctx, q,
        job.ID,
        job.OrganizationID,  // ADD THIS
        job.CreatedBy,        // ADD THIS
        job.Name,
        job.Status,
        // ... rest
    )

    return err
}
```

### Step 4: Update Middleware

#### Update `web/middleware.go`

Replace API key middleware with session + organization middleware:

```go
package web

import (
    "context"
    "net/http"
    "strings"
)

type contextKey string

const (
    contextKeyUser         contextKey = "user"
    contextKeyOrganization contextKey = "organization"
    contextKeyMember       contextKey = "member"
)

// AuthMiddleware validates session and injects user into context
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract token from header or cookie
        token := extractSessionToken(r)
        if token == "" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        // Validate session
        user, session, err := s.authSvc.ValidateSession(r.Context(), token)
        if err != nil {
            http.Error(w, "Invalid session", http.StatusUnauthorized)
            return
        }

        // Inject user into context
        ctx := context.WithValue(r.Context(), contextKeyUser, user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// OrganizationMiddleware extracts organization from URL and validates membership
func (s *Server) OrganizationMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user := getUserFromContext(r.Context())
        if user == nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        // Extract organization ID from URL path
        orgID := extractOrgIDFromPath(r.URL.Path)
        if orgID == "" {
            http.Error(w, "Organization ID required", http.StatusBadRequest)
            return
        }

        // Verify membership
        member, err := s.memberRepo.GetByOrganizationAndUser(r.Context(), orgID, user.ID)
        if err != nil {
            http.Error(w, "Access denied", http.StatusForbidden)
            return
        }

        // Get organization
        org, err := s.orgRepo.Get(r.Context(), orgID)
        if err != nil {
            http.Error(w, "Organization not found", http.StatusNotFound)
            return
        }

        // Inject into context
        ctx := context.WithValue(r.Context(), contextKeyOrganization, &org)
        ctx = context.WithValue(ctx, contextKeyMember, &member)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Helper functions
func extractSessionToken(r *http.Request) string {
    // Try Authorization header first
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }

    // Try cookie
    cookie, err := r.Cookie("session_token")
    if err == nil {
        return cookie.Value
    }

    return ""
}

func getUserFromContext(ctx context.Context) *User {
    user, _ := ctx.Value(contextKeyUser).(*User)
    return user
}

func getOrganizationFromContext(ctx context.Context) *Organization {
    org, _ := ctx.Value(contextKeyOrganization).(*Organization)
    return org
}

func getMemberFromContext(ctx context.Context) *OrganizationMember {
    member, _ := ctx.Value(contextKeyMember).(*OrganizationMember)
    return member
}

func extractOrgIDFromPath(path string) string {
    // Extract from /api/v1/organizations/{orgId}/...
    parts := strings.Split(path, "/")
    for i, part := range parts {
        if part == "organizations" && i+1 < len(parts) {
            return parts[i+1]
        }
    }
    return ""
}
```

### Step 5: Create API Handlers

Create `web/auth_handlers.go`:

```go
package web

import (
    "encoding/json"
    "net/http"
)

// Register handler
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email     string `json:"email"`
        Password  string `json:"password"`
        FirstName string `json:"first_name"`
        LastName  string `json:"last_name"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }

    user, err := s.authSvc.Register(r.Context(), req.Email, req.Password, req.FirstName, req.LastName)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    renderJSON(w, http.StatusCreated, user)
}

// Login handler
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }

    ipAddress := r.RemoteAddr
    userAgent := r.Header.Get("User-Agent")

    user, session, token, err := s.authSvc.Login(r.Context(), req.Email, req.Password, ipAddress, userAgent)
    if err != nil {
        http.Error(w, err.Error(), http.StatusUnauthorized)
        return
    }

    // Set cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    token,
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteStrictMode,
        Expires:  session.ExpiresAt,
    })

    renderJSON(w, http.StatusOK, map[string]interface{}{
        "user":          user,
        "session_token": token,
        "expires_at":    session.ExpiresAt,
    })
}

// Logout handler
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r.Context())
    if user == nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Get session from context (would need to be added to middleware)
    // For now, logout all sessions
    if err := s.authSvc.LogoutAll(r.Context(), user.ID); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Clear cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    "",
        Path:     "/",
        HttpOnly: true,
        MaxAge:   -1,
    })

    w.WriteHeader(http.StatusNoContent)
}

// Get current user
func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
    user := getUserFromContext(r.Context())
    if user == nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    renderJSON(w, http.StatusOK, user)
}
```

### Step 6: Update Routing in `web/web.go`

Add routes for authentication and organizations:

```go
func (s *Server) setupRoutes() {
    // Public routes
    mux.HandleFunc("/api/v1/auth/register", s.handleRegister)
    mux.HandleFunc("/api/v1/auth/login", s.handleLogin)

    // Protected routes
    authMux := http.NewServeMux()

    // User routes
    authMux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
    authMux.HandleFunc("/api/v1/auth/me", s.handleGetMe)

    // Organization routes
    authMux.HandleFunc("/api/v1/organizations", s.handleOrganizations)
    authMux.HandleFunc("/api/v1/organizations/", s.handleOrganizationDetail)

    // Wrap with auth middleware
    mux.Handle("/api/v1/", s.AuthMiddleware(authMux))

    // ... existing routes
}
```

### Step 7: Update UI Templates

Create login page `web/static/templates/login.html`:

```html
<!DOCTYPE html>
<html>
<head>
    <title>Login - Google Maps Scraper</title>
</head>
<body>
    <div class="container">
        <h1>Login</h1>
        <form id="loginForm">
            <input type="email" name="email" placeholder="Email" required>
            <input type="password" name="password" placeholder="Password" required>
            <button type="submit">Login</button>
        </form>
        <p>Don't have an account? <a href="/register">Register</a></p>
    </div>

    <script>
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const formData = new FormData(e.target);

            const response = await fetch('/api/v1/auth/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    email: formData.get('email'),
                    password: formData.get('password')
                })
            });

            if (response.ok) {
                window.location.href = '/';
            } else {
                alert('Login failed');
            }
        });
    </script>
</body>
</html>
```

### Step 8: Initialize Services in Main

Update `web/web.go` Server initialization:

```go
type Server struct {
    // Existing
    repo      JobRepository
    apiKeySvc *APIKeyService

    // NEW: Add these
    authSvc   *AuthService
    orgSvc    *OrganizationService
    memberSvc *MemberService

    // Repositories
    userRepo       UserRepository
    orgRepo        OrganizationRepository
    memberRepo     OrganizationMemberRepository
    sessionRepo    UserSessionRepository
    invitationRepo OrganizationInvitationRepository
    auditRepo      AuditLogRepository
}

func NewServer(db *sql.DB) *Server {
    // Initialize repositories
    userRepo := postgres.NewUserRepository(db)
    orgRepo := postgres.NewOrganizationRepository(db)
    memberRepo := postgres.NewOrganizationMemberRepository(db)
    sessionRepo := postgres.NewUserSessionRepository(db)
    invitationRepo := postgres.NewOrganizationInvitationRepository(db)
    auditRepo := postgres.NewAuditLogRepository(db)

    // Initialize services
    authSvc := NewAuthService(userRepo, sessionRepo, auditRepo)
    orgSvc := NewOrganizationService(orgRepo, memberRepo, auditRepo)
    memberSvc := NewMemberService(memberRepo, userRepo, invitationRepo, auditRepo)

    return &Server{
        // Existing
        repo:      existingJobRepo,
        apiKeySvc: existingAPIKeySvc,

        // New
        authSvc:        authSvc,
        orgSvc:         orgSvc,
        memberSvc:      memberSvc,
        userRepo:       userRepo,
        orgRepo:        orgRepo,
        memberRepo:     memberRepo,
        sessionRepo:    sessionRepo,
        invitationRepo: invitationRepo,
        auditRepo:      auditRepo,
    }
}
```

## Testing the Implementation

### 1. Test User Registration

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "securepassword123",
    "first_name": "Test",
    "last_name": "User"
  }'
```

### 2. Test Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "securepassword123"
  }'
```

Save the `session_token` from the response.

### 3. Test Create Organization

```bash
curl -X POST http://localhost:8080/api/v1/organizations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SESSION_TOKEN" \
  -d '{
    "name": "My Organization",
    "description": "Test organization"
  }'
```

### 4. Test List Organizations

```bash
curl -X GET http://localhost:8080/api/v1/organizations \
  -H "Authorization: Bearer YOUR_SESSION_TOKEN"
```

## Troubleshooting

### Common Issues

1. **Migration fails**: Check PostgreSQL version and permissions
2. **Sessions not working**: Verify token extraction in middleware
3. **Organization context missing**: Check URL routing and extraction
4. **Permission denied**: Verify role assignment and permission checks

### Debug Tips

- Enable SQL logging to see queries
- Add logging to middleware for request flow
- Use audit logs to track operations
- Check context values in handlers

## Summary

You now have:
- ✅ Complete database schema with multi-tenancy
- ✅ All models, repositories, and services
- ✅ Authentication system with sessions
- ✅ Organization and member management
- ✅ Role-based access control
- ✅ Audit logging

Next: Implement the API handlers and UI components to complete the integration!
