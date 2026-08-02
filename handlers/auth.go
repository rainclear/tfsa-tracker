package handlers

import (
	"database/sql"
	"fmt"
	"net/http"

	"tfsa-tracker/auth"
	"tfsa-tracker/config"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

type AuthHandler struct {
	DB           *sql.DB
	SessionMgr   *auth.SessionManager
	EmailService *services.EmailService
	Config       *config.Config
}

func NewAuthHandler(db *sql.DB, sm *auth.SessionManager, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		DB:           db,
		SessionMgr:   sm,
		EmailService: services.NewEmailService(db, cfg),
		Config:       cfg,
	}
}

// Login renders the login page or processes credentials
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<h2>Login - TFSA Tracker</h2>
			<form method="POST" action="/login">
				<label>Email: <input type="email" name="email" required></label><br><br>
				<label>Password: <input type="password" name="password" required></label><br><br>
				<button type="submit">Sign In</button>
			</form>
			<p>Need an account? <a href="/register">Register here</a></p>
		`)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := models.AuthenticateUser(h.DB, email, password)
	if err != nil {
		http.Error(w, "Invalid credentials or account not yet approved", http.StatusUnauthorized)
		return
	}

	h.SessionMgr.CreateSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Register processes new user registration requests
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<h2>Register - TFSA Tracker</h2>
			<form method="POST" action="/register">
				<label>Email: <input type="email" name="email" required></label><br><br>
				<label>Password: <input type="password" name="password" required></label><br><br>
				<button type="submit">Register Account</button>
			</form>
			<p>Already have an account? <a href="/login">Login</a></p>
		`)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	// Call models.CreateUser with 3 arguments (db, email, password)
	user, err := models.CreateUser(h.DB, email, password)
	if err != nil {
		http.Error(w, "Error creating account: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Generate activation token
	token, err := h.EmailService.GenerateActivationToken(user.ID)
	if err == nil {
		_ = h.EmailService.SendActivationEmail(user.Email, token)
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<h3>Registration Successful!</h3>
		<p>Your registration request has been submitted. An administrator must approve your account before you can log in.</p>
		<p><a href="/login">Return to Login</a></p>
	`)
}

// Activate verifies the token sent to the user's email
func (h *AuthHandler) Activate(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing activation token", http.StatusBadRequest)
		return
	}

	_, err := h.EmailService.VerifyActivationToken(token)
	if err != nil {
		http.Error(w, "Invalid or expired token: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<h3>Account Activated!</h3>
		<p>Your account has been activated successfully. You may now log in.</p>
		<p><a href="/login">Log In Now</a></p>
	`)
}

// Logout destroys the current user session
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.SessionMgr.DestroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
