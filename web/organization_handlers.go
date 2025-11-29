package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleOrganizations handles organization listing and creation
func (s *Server) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListOrganizations(w, r)
	case http.MethodPost:
		s.handleCreateOrganization(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListOrganizations lists all organizations for the current user
func (s *Server) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	orgs, err := s.orgSvc.GetUserOrganizations(r.Context(), user.ID)
	if err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, orgs)
}

// handleCreateOrganization creates a new organization
func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	org, err := s.orgSvc.Create(r.Context(), req.Name, req.Description, user.ID)
	if err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusCreated, org)
}

// handleOrganizationDetail handles getting, updating, and deleting a specific organization
func (s *Server) handleOrganizationDetail(w http.ResponseWriter, r *http.Request) {
	// Extract org ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Organization ID required", http.StatusBadRequest)
		return
	}

	orgID := parts[0]

	switch r.Method {
	case http.MethodGet:
		s.handleGetOrganization(w, r, orgID)
	case http.MethodPut:
		s.handleUpdateOrganization(w, r, orgID)
	case http.MethodDelete:
		s.handleDeleteOrganization(w, r, orgID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetOrganization gets an organization by ID
func (s *Server) handleGetOrganization(w http.ResponseWriter, r *http.Request, orgID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	org, err := s.orgSvc.Get(r.Context(), orgID, user.ID)
	if err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: "Access denied"})
		return
	}

	renderJSON(w, http.StatusOK, org)
}

// handleUpdateOrganization updates an organization
func (s *Server) handleUpdateOrganization(w http.ResponseWriter, r *http.Request, orgID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	org, err := s.orgSvc.Get(r.Context(), orgID, user.ID)
	if err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: "Access denied"})
		return
	}

	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Description != "" {
		org.Description = req.Description
	}
	if req.Status != "" {
		org.Status = req.Status
	}

	if err := s.orgSvc.Update(r.Context(), org, user.ID); err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, org)
}

// handleDeleteOrganization deletes an organization
func (s *Server) handleDeleteOrganization(w http.ResponseWriter, r *http.Request, orgID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	if err := s.orgSvc.Delete(r.Context(), orgID, user.ID); err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
