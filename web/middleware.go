package web

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	contextKeyUser           contextKey = "user"
	contextKeyMember         contextKey = "member"
	contextKeyOrganizationID contextKey = "organization_id"
)

// APIKeyAuthMiddleware checks for a valid API key in the request
// It looks for the API key in the following order:
// 1. Authorization header (Bearer token)
// 2. X-API-Key header
// 3. api_key query parameter
func APIKeyAuthMiddleware(apiKeyService *APIKeyService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract API key from request
			apiKey := extractAPIKey(r)

			if apiKey == "" {
				renderJSON(w, http.StatusUnauthorized, apiError{
					Code:    http.StatusUnauthorized,
					Message: "API key is required. Provide it via Authorization header (Bearer token), X-API-Key header, or api_key query parameter.",
				})
				return
			}

			// Validate API key and get updated context with organization
			_, newCtx, err := apiKeyService.Validate(ctx, apiKey)
			if err != nil {
				renderJSON(w, http.StatusUnauthorized, apiError{
					Code:    http.StatusUnauthorized,
					Message: "Invalid or expired API key",
				})
				return
			}

			// API key is valid, proceed to next handler with updated context
			next.ServeHTTP(w, r.WithContext(newCtx))
		})
	}
}

// extractAPIKey extracts the API key from the request
// Priority: Authorization header > X-API-Key header > api_key query parameter
func extractAPIKey(r *http.Request) string {
	// 1. Check Authorization header (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Check X-API-Key header
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != "" {
		return strings.TrimSpace(apiKeyHeader)
	}

	// 3. Check api_key query parameter
	apiKeyQuery := r.URL.Query().Get("api_key")
	if apiKeyQuery != "" {
		return strings.TrimSpace(apiKeyQuery)
	}

	return ""
}

// SessionAuthMiddleware validates session and injects user + organization into context
func (s *Server) SessionAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from header or cookie
		token := extractSessionToken(r)
		if token == "" {
			renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusUnauthorized, Message: "Authentication required"})
			return
		}

		// Validate session
		user, _, err := s.authSvc.ValidateSession(r.Context(), token)
		if err != nil {
			// Clear invalid cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
			renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusUnauthorized, Message: "Invalid or expired session"})
			return
		}

		// Inject user into context
		ctx := context.WithValue(r.Context(), contextKeyUser, user)

		// Get user's first organization and inject into context
		if s.memberRepo != nil {
			orgs, err := s.memberRepo.GetUserOrganizations(ctx, user.ID)
			if err == nil && len(orgs) > 0 {
				// Use first organization as default
				ctx = context.WithValue(ctx, contextKeyOrganizationID, orgs[0].ID)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractSessionToken extracts the session token from the request
// Priority: Authorization header > Cookie
func extractSessionToken(r *http.Request) string {
	// Check Authorization header (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return strings.TrimSpace(parts[1])
		}
	}

	// Check cookie
	cookie, err := r.Cookie("session_token")
	if err == nil {
		return cookie.Value
	}

	return ""
}

// APIOrSessionAuthMiddleware supports both API key and session authentication
// It tries API key first (for programmatic access), then falls back to session (for web UI)
func (s *Server) APIOrSessionAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// First, try API key authentication (if apiKeySvc is available)
		if s.apiKeySvc != nil {
			apiKey := extractAPIKey(r)
			if apiKey != "" {
				// Validate API key and get updated context with organization
				_, newCtx, err := s.apiKeySvc.Validate(ctx, apiKey)
				if err == nil {
					// API key is valid, proceed with updated context (includes org)
					next.ServeHTTP(w, r.WithContext(newCtx))
					return
				}
				// API key provided but invalid - return error immediately
				renderJSON(w, http.StatusUnauthorized, apiError{
					Code:    http.StatusUnauthorized,
					Message: "Invalid or expired API key: " + err.Error(),
				})
				return
			}
		}

		// No API key provided, try session authentication
		token := extractSessionToken(r)
		if token == "" {
			renderJSON(w, http.StatusUnauthorized, apiError{
				Code:    http.StatusUnauthorized,
				Message: "Authentication required. Provide API key via Authorization header (Bearer token) or X-API-Key header, or log in via web UI.",
			})
			return
		}

		// Validate session
		user, _, err := s.authSvc.ValidateSession(ctx, token)
		if err != nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
			renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusUnauthorized, Message: "Invalid or expired session"})
			return
		}

		// Inject user into context
		ctx = context.WithValue(ctx, contextKeyUser, user)

		// Get user's first organization and inject into context
		if s.memberRepo != nil {
			orgs, err := s.memberRepo.GetUserOrganizations(ctx, user.ID)
			if err == nil && len(orgs) > 0 {
				// Use first organization as default
				ctx = context.WithValue(ctx, contextKeyOrganizationID, orgs[0].ID)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getUserFromContext retrieves the user from the request context
func getUserFromContext(ctx context.Context) *User {
	user, ok := ctx.Value(contextKeyUser).(*User)
	if !ok {
		return nil
	}
	return user
}

// getMemberFromContext retrieves the member from the request context
func getMemberFromContext(ctx context.Context) *OrganizationMember {
	member, ok := ctx.Value(contextKeyMember).(*OrganizationMember)
	if !ok {
		return nil
	}
	return member
}

// getOrganizationIDFromContext retrieves the organization ID from the request context
func getOrganizationIDFromContext(ctx context.Context) string {
	orgID, ok := ctx.Value(contextKeyOrganizationID).(string)
	if !ok {
		return ""
	}
	return orgID
}

// WebAuthMiddleware protects web pages by redirecting to login if not authenticated
func (s *Server) WebAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth service is not configured, allow access (backward compatibility)
		if s.authSvc == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from cookie
		token := extractSessionToken(r)
		if token == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session
		user, _, err := s.authSvc.ValidateSession(r.Context(), token)
		if err != nil {
			// Clear invalid cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Inject user into context
		ctx := context.WithValue(r.Context(), contextKeyUser, user)

		// Get user's first organization and inject into context
		if s.memberRepo != nil {
			orgs, err := s.memberRepo.GetUserOrganizations(ctx, user.ID)
			if err == nil && len(orgs) > 0 {
				// Use first organization as default
				ctx = context.WithValue(ctx, contextKeyOrganizationID, orgs[0].ID)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
