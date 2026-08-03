package handlers

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"

	"tfsa-tracker/auth"
	"tfsa-tracker/config"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

//go:embed templates/login.html templates/register.html
var authTemplateFS embed.FS

type AuthHandler struct {
	DB           *sql.DB
	SessionMgr   *auth.SessionManager
	EmailService *services.EmailService
	Config       *config.Config
	loginTmpl    *template.Template
	registerTmpl *template.Template
}

func NewAuthHandler(db *sql.DB, sm *auth.SessionManager, cfg *config.Config) *AuthHandler {
	loginTmpl, err := template.ParseFS(authTemplateFS, "templates/login.html")
	if err != nil {
		panic("Failed to parse login template: " + err.Error())
	}

	registerTmpl, err := template.ParseFS(authTemplateFS, "templates/register.html")
	if err != nil {
		panic("Failed to parse register template: " + err.Error())
	}

	return &AuthHandler{
		DB:           db,
		SessionMgr:   sm,
		EmailService: services.NewEmailService(db, cfg),
		Config:       cfg,
		loginTmpl:    loginTmpl,
		registerTmpl: registerTmpl,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.loginTmpl.Execute(w, nil)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := models.AuthenticateUser(h.DB, email, password)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		h.loginTmpl.Execute(w, map[string]string{
			"Error": "Invalid email/password or account pending approval.",
		})
		return
	}

	h.SessionMgr.CreateSession(w, user)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.registerTmpl.Execute(w, nil)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := models.CreateUser(h.DB, email, password)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		h.registerTmpl.Execute(w, map[string]string{
			"Error": "Registration failed: " + err.Error(),
		})
		return
	}

	token, err := h.EmailService.GenerateActivationToken(user.ID)
	if err == nil {
		_ = h.EmailService.SendActivationEmail(user.Email, token)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.registerTmpl.Execute(w, map[string]string{
		"Success": "Registration successful! An administrator must approve your account before you can sign in.",
	})
}

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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.loginTmpl.Execute(w, map[string]string{
		"Success": "Account activated successfully! You may now sign in.",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.SessionMgr.DestroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
