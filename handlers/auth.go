package handlers

import (
	"database/sql"
	"fmt"
	"net/http"

	"tfsa-tracker/auth"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

type AuthHandler struct {
	DB           *sql.DB
	Session      *auth.SessionManager
	EmailService *services.EmailService
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Write([]byte(`<h1>Register</h1><form method="POST"><input type="email" name="email" placeholder="Email"/><input type="password" name="password"/><button type="submit">Register</button></form>`))
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := models.CreateUser(h.DB, email, password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// First user auto-admin login
	if user.Status == models.StatusApproved {
		h.Session.CreateSession(w, user)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	w.Write([]byte("Registration submitted! Your account is pending admin approval."))
}

func (h *AuthHandler) Activate(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing activation token", http.StatusBadRequest)
		return
	}

	user, err := h.EmailService.VerifyActivationToken(token)
	if err != nil {
		http.Error(w, fmt.Sprintf("Activation failed: %v", err), http.StatusBadRequest)
		return
	}

	w.Write([]byte(fmt.Sprintf("<h1>Account Activated!</h1><p>Welcome %s. You can now <a href='/login'>login</a>.</p>", user.Email)))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Write([]byte(`<h1>Login</h1><form method="POST"><input type="email" name="email" placeholder="Email"/><input type="password" name="password"/><button type="submit">Login</button></form>`))
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := models.AuthenticateUser(h.DB, email, password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	h.Session.CreateSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.Session.DestroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
