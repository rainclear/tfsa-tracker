package handlers

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"

	"tfsa-tracker/models"
)

//go:embed templates/admin.html
var adminTemplateFS embed.FS

type AdminHandler struct {
	DB            *sql.DB
	adminTemplate *template.Template
}

type AdminViewData struct {
	Users []models.User
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	tmpl, err := template.ParseFS(adminTemplateFS, "templates/admin.html")
	if err != nil {
		panic("Failed to parse embedded admin template: " + err.Error())
	}

	return &AdminHandler{
		DB:            db,
		adminTemplate: tmpl,
	}
}

// AdminPanel renders all user registration requests for admin review
func (h *AdminHandler) AdminPanel(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetAllUsers(h.DB)
	if err != nil {
		http.Error(w, "Failed to load users: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := AdminViewData{
		Users: users,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.adminTemplate.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ApproveUser approves a pending user account
func (h *AdminHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		userIDStr := r.FormValue("user_id")
		_, err := h.DB.Exec(`UPDATE users SET status = 'APPROVED' WHERE id = ?`, userIDStr)
		if err != nil {
			http.Error(w, "Failed to approve user: "+err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}
