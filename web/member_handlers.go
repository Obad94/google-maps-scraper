package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleOrganizationMembers handles member management for an organization
func (s *Server) handleOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	// Extract org ID from path: /api/v1/organizations/{orgId}/members
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		http.Error(w, "Organization ID required", http.StatusBadRequest)
		return
	}

	orgID := parts[0]

	switch r.Method {
	case http.MethodGet:
		s.handleListMembers(w, r, orgID)
	case http.MethodPost:
		if len(parts) > 2 && parts[2] == "invite" {
			s.handleInviteMember(w, r, orgID)
		} else {
			http.Error(w, "Invalid endpoint", http.StatusNotFound)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListMembers lists all members of an organization
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request, orgID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	members, err := s.memberSvc.GetMembers(r.Context(), orgID, user.ID)
	if err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, members)
}

// handleInviteMember invites a new member to the organization
func (s *Server) handleInviteMember(w http.ResponseWriter, r *http.Request, orgID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	invitation, err := s.memberSvc.InviteMember(r.Context(), orgID, req.Email, req.Role, user.ID)
	if err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	// Return invitation with token (for email sending or display)
	renderJSON(w, http.StatusCreated, map[string]interface{}{
		"invitation": invitation,
		"invite_url": "/accept-invitation?token=" + invitation.TokenHash,
	})
}

// handleMemberDetail handles updating and removing a specific member
func (s *Server) handleMemberDetail(w http.ResponseWriter, r *http.Request) {
	// Extract from path: /api/v1/organizations/{orgId}/members/{userId}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] == "" || parts[2] == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	orgID := parts[0]
	memberUserID := parts[2]

	switch r.Method {
	case http.MethodDelete:
		s.handleRemoveMember(w, r, orgID, memberUserID)
	case http.MethodPatch:
		s.handleUpdateMemberRole(w, r, orgID, memberUserID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRemoveMember removes a member from the organization
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, orgID, memberUserID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	if err := s.memberSvc.RemoveMember(r.Context(), orgID, memberUserID, user.ID); err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateMemberRole updates a member's role
func (s *Server) handleUpdateMemberRole(w http.ResponseWriter, r *http.Request, orgID, memberUserID string) {
	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	var req struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		return
	}

	if err := s.memberSvc.UpdateMemberRole(r.Context(), orgID, memberUserID, req.Role, user.ID); err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Role updated successfully",
	})
}

// handleAcceptInvitation handles accepting an organization invitation
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: "Invitation token required"})
		return
	}

	if err := s.memberSvc.AcceptInvitation(r.Context(), token, user.ID); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Invitation accepted successfully",
	})
}

// handlePendingInvitations lists pending invitations for an organization
func (s *Server) handlePendingInvitations(w http.ResponseWriter, r *http.Request, orgID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusBadRequest, Message: "Unauthorized"})
		return
	}

	invitations, err := s.memberSvc.GetPendingInvitations(r.Context(), orgID, user.ID)
	if err != nil {
		renderJSON(w, http.StatusForbidden, apiError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	renderJSON(w, http.StatusOK, invitations)
}
