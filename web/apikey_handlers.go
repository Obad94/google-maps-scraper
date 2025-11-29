package web

import (
	"encoding/json"
	"net/http"
	"time"
)

type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"` // Only returned on creation
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Message   string     `json:"message"`
}

type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type listAPIKeysResponse struct {
	APIKeys []apiKeyResponse `json:"api_keys"`
	Count   int              `json:"count"`
}

// apiCreateAPIKey handles POST /api/v1/apikeys
func (s *Server) apiCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createAPIKeyRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderJSON(w, http.StatusBadRequest, apiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request body",
		})
		return
	}

	if req.Name == "" {
		renderJSON(w, http.StatusUnprocessableEntity, apiError{
			Code:    http.StatusUnprocessableEntity,
			Message: "Name is required",
		})
		return
	}

	// Create API key
	key, apiKey, err := s.apiKeySvc.Create(r.Context(), req.Name, req.ExpiresAt)
	if err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to create API key: " + err.Error(),
		})
		return
	}

	response := createAPIKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       key, // Full API key - only shown once!
		Status:    apiKey.Status,
		CreatedAt: apiKey.CreatedAt,
		ExpiresAt: apiKey.ExpiresAt,
		Message:   "API key created successfully. Please save this key as it will not be shown again.",
	}

	renderJSON(w, http.StatusCreated, response)
}

// apiListAPIKeys handles GET /api/v1/apikeys
func (s *Server) apiListAPIKeys(w http.ResponseWriter, r *http.Request) {
	apiKeys, err := s.apiKeySvc.List(r.Context())
	if err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to list API keys: " + err.Error(),
		})
		return
	}

	response := listAPIKeysResponse{
		APIKeys: make([]apiKeyResponse, len(apiKeys)),
		Count:   len(apiKeys),
	}

	for i, key := range apiKeys {
		response.APIKeys[i] = apiKeyResponse{
			ID:         key.ID,
			Name:       key.Name,
			Status:     key.Status,
			CreatedAt:  key.CreatedAt,
			UpdatedAt:  key.UpdatedAt,
			LastUsedAt: key.LastUsedAt,
			ExpiresAt:  key.ExpiresAt,
		}
	}

	renderJSON(w, http.StatusOK, response)
}

// apiGetAPIKey handles GET /api/v1/apikeys/{id}
func (s *Server) apiGetAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		renderJSON(w, http.StatusBadRequest, apiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid or missing API key ID",
		})
		return
	}

	apiKey, err := s.apiKeySvc.Get(r.Context(), id.String())
	if err != nil {
		renderJSON(w, http.StatusNotFound, apiError{
			Code:    http.StatusNotFound,
			Message: "API key not found",
		})
		return
	}

	response := apiKeyResponse{
		ID:         apiKey.ID,
		Name:       apiKey.Name,
		Status:     apiKey.Status,
		CreatedAt:  apiKey.CreatedAt,
		UpdatedAt:  apiKey.UpdatedAt,
		LastUsedAt: apiKey.LastUsedAt,
		ExpiresAt:  apiKey.ExpiresAt,
	}

	renderJSON(w, http.StatusOK, response)
}

// apiRevokeAPIKey handles POST /api/v1/apikeys/{id}/revoke
func (s *Server) apiRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		renderJSON(w, http.StatusBadRequest, apiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid or missing API key ID",
		})
		return
	}

	if err := s.apiKeySvc.Revoke(r.Context(), id.String()); err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to revoke API key: " + err.Error(),
		})
		return
	}

	renderJSON(w, http.StatusOK, map[string]string{
		"message": "API key revoked successfully",
	})
}

// apiDeleteAPIKey handles DELETE /api/v1/apikeys/{id}
func (s *Server) apiDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := getIDFromRequest(r)
	if !ok {
		renderJSON(w, http.StatusBadRequest, apiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid or missing API key ID",
		})
		return
	}

	if err := s.apiKeySvc.Delete(r.Context(), id.String()); err != nil {
		renderJSON(w, http.StatusInternalServerError, apiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to delete API key: " + err.Error(),
		})
		return
	}

	renderJSON(w, http.StatusOK, map[string]string{
		"message": "API key deleted successfully",
	})
}

// Web UI handlers for API key management

// apiKeysPage handles GET /apikeys - Shows the API keys management page
func (s *Server) apiKeysPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, ok := s.tmpl["static/templates/apikeys.html"]
	if !ok {
		http.Error(w, "missing template", http.StatusInternalServerError)
		return
	}

	// Get all API keys
	apiKeys, err := s.apiKeySvc.List(r.Context())
	if err != nil {
		http.Error(w, "Failed to load API keys", http.StatusInternalServerError)
		return
	}

	data := struct {
		APIKeys []APIKey
	}{
		APIKeys: apiKeys,
	}

	_ = tmpl.Execute(w, data)
}

// createAPIKeyWeb handles POST /apikeys/create - Creates an API key via web form
func (s *Server) createAPIKeyWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method not allowed"))
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to parse form: " + err.Error()))
		return
	}

	name := r.Form.Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Name is required"))
		return
	}

	var expiresAt *time.Time
	expiresAtStr := r.Form.Get("expires_at")
	if expiresAtStr != "" {
		parsed, err := time.Parse("2006-01-02", expiresAtStr)
		if err == nil {
			expiresAt = &parsed
		}
	}

	// Create API key
	key, apiKey, err := s.apiKeySvc.Create(r.Context(), name, expiresAt)
	if err != nil {
		http.Error(w, "Failed to create API key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return HTML with the generated key
	tmpl, ok := s.tmpl["static/templates/apikey_created.html"]
	if !ok {
		// Fallback to plain text response
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`
			<div class="api-key-created">
				<h3>API Key Created Successfully</h3>
				<p><strong>Name:</strong> ` + apiKey.Name + `</p>
				<p><strong>Key:</strong> <code>` + key + `</code></p>
				<p class="warning">⚠️ Please save this key now. It will not be shown again.</p>
				<button onclick="location.reload()">OK</button>
			</div>
		`))
		return
	}

	data := struct {
		ID   string
		Name string
		Key  string
	}{
		ID:   apiKey.ID,
		Name: apiKey.Name,
		Key:  key,
	}

	_ = tmpl.Execute(w, data)
}

// revokeAPIKeyWeb handles POST /apikeys/{id}/revoke - Revokes an API key via web form
func (s *Server) revokeAPIKeyWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r = requestWithID(r)
	id, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid API key ID", http.StatusBadRequest)
		return
	}

	if err := s.apiKeySvc.Revoke(r.Context(), id.String()); err != nil {
		http.Error(w, "Failed to revoke API key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to API keys page
	http.Redirect(w, r, "/apikeys", http.StatusSeeOther)
}

// deleteAPIKeyWeb handles POST /apikeys/{id}/delete - Deletes an API key via web form
func (s *Server) deleteAPIKeyWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r = requestWithID(r)
	id, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "Invalid API key ID", http.StatusBadRequest)
		return
	}

	if err := s.apiKeySvc.Delete(r.Context(), id.String()); err != nil {
		http.Error(w, "Failed to delete API key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to API keys page
	http.Redirect(w, r, "/apikeys", http.StatusSeeOther)
}
