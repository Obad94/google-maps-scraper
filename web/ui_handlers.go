package web

import (
	"net/http"
)

// showLoginPage renders the login page
func (s *Server) showLoginPage(w http.ResponseWriter, r *http.Request) {
	tmpl, ok := s.tmpl["static/templates/login.html"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, nil)
}

// showRegisterPage renders the registration page
func (s *Server) showRegisterPage(w http.ResponseWriter, r *http.Request) {
	tmpl, ok := s.tmpl["static/templates/register.html"]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, nil)
}

// showOrganizationsPage renders the organizations list page
func (s *Server) showOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	// For now, redirect to login if not authenticated
	// In a full implementation, this would check auth and render the page
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// showOrganizationDetailPage renders a specific organization page
func (s *Server) showOrganizationDetailPage(w http.ResponseWriter, r *http.Request) {
	// For now, redirect to login if not authenticated
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
