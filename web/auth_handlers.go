package web

import (
	"encoding/json"
	"net/http"
)

// handleRegister creates a new user account
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if auth service is configured
	if s.authSvc == nil {
		renderJSON(w, http.StatusServiceUnavailable, apiError{Code: http.StatusServiceUnavailable, Message: "Authentication service not configured"})
		return
	}

	var req struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	user, err := s.authSvc.Register(r.Context(), req.Email, req.Password, req.FirstName, req.LastName)
	if err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	// Auto-login after registration
	ipAddress := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")

	_, session, token, err := s.authSvc.Login(r.Context(), req.Email, req.Password, ipAddress, userAgent)
	if err != nil {
		renderJSON(w, http.StatusOK, user)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

	renderJSON(w, http.StatusCreated, map[string]interface{}{
		"user":          user,
		"session_token": token,
		"expires_at":    session.ExpiresAt,
	})
}

// handleLogin authenticates a user
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if auth service is configured
	if s.authSvc == nil {
		renderJSON(w, http.StatusServiceUnavailable, apiError{Code: http.StatusServiceUnavailable, Message: "Authentication service not configured"})
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	ipAddress := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")

	user, session, token, err := s.authSvc.Login(r.Context(), req.Email, req.Password, ipAddress, userAgent)
	if err != nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Invalid email or password"})
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

	renderJSON(w, http.StatusOK, map[string]interface{}{
		"user":          user,
		"session_token": token,
		"expires_at":    session.ExpiresAt,
	})
}

// handleLogout invalidates the current session
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	// Logout all sessions
	if err := s.authSvc.LogoutAll(r.Context(), user.ID); err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{Code: http.StatusBadRequest, Message: err.Error()})
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

// handleGetMe returns the current user
func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	// Get user's organizations
	orgs, err := s.orgSvc.GetUserOrganizations(r.Context(), user.ID)
	if err != nil {
		orgs = []Organization{}
	}

	renderJSON(w, http.StatusOK, map[string]interface{}{
		"user":          user,
		"organizations": orgs,
	})
}

// handleChangePassword changes user's password
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	if err := s.authSvc.ChangePassword(r.Context(), user.ID, req.OldPassword, req.NewPassword); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: err.Error()})
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

	renderJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Password changed successfully. Please login again.",
	})
}
