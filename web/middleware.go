package web

import (
	"net/http"
	"strings"
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

			// Validate API key
			_, err := apiKeyService.Validate(ctx, apiKey)
			if err != nil {
				renderJSON(w, http.StatusUnauthorized, apiError{
					Code:    http.StatusUnauthorized,
					Message: "Invalid or expired API key",
				})
				return
			}

			// API key is valid, proceed to next handler
			next.ServeHTTP(w, r)
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
