package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

type AdminHandler struct {
	DB           *sql.DB
	EmailService *services.EmailService
}

func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	users, err := models.ListAllUsers(h.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "<h1>Admin Dashboard</h1><ul>")
	for _, u := range users {
		fmt.Fprintf(w, "<li>ID: %d | Email: %s | Role: %s | Status: %s</li>", u.ID, u.Email, u.Role, u.Status)
	}
	fmt.Fprintf(w, "</ul>")
}

// ApproveUser generates activation token and sends email
func (h *AdminHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	targetID, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)

	targetUser, err := models.GetUserByID(h.DB, targetID)
	if err != nil {
		http.Error(w, "User not found", http.StatusBadRequest)
		return
	}

	// Set status to PENDING_ACTIVATION
	if err := models.UpdateUserStatus(h.DB, targetID, models.StatusPendingActivation); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Generate Token and dispatch email
	token, err := h.EmailService.GenerateActivationToken(targetID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	if err := h.EmailService.SendActivationEmail(targetUser.Email, token); err != nil {
		http.Error(w, "Failed to send activation email", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}
