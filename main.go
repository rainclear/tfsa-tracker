package main

import (
	"fmt"
	"log"
	"net/http"

	"tfsa-tracker/auth"
	"tfsa-tracker/config"
	"tfsa-tracker/handlers"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

func main() {
	cfg := config.LoadConfig()

	db, err := models.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer db.Close()

	sessionMgr := auth.NewSessionManager()
	emailService := services.NewEmailService(db, cfg)

	authHandler := &handlers.AuthHandler{
		DB:           db,
		Session:      sessionMgr,
		EmailService: emailService,
	}

	adminHandler := &handlers.AdminHandler{
		DB:           db,
		EmailService: emailService,
	}

	mux := http.NewServeMux()

	// Public Routes
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) })
	mux.HandleFunc("GET /register", authHandler.Register)
	mux.HandleFunc("POST /register", authHandler.Register)
	mux.HandleFunc("GET /activate", authHandler.Activate)
	mux.HandleFunc("GET /login", authHandler.Login)
	mux.HandleFunc("POST /login", authHandler.Login)
	mux.HandleFunc("POST /logout", authHandler.Logout)

	// User Routes
	mux.HandleFunc("GET /dashboard", sessionMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.GetUserID(r.Context())
		user, _ := models.GetUserByID(db, userID)
		fmt.Fprintf(w, "Welcome %s!", user.Email)
	}))

	// Admin Routes
	mux.HandleFunc("GET /admin/dashboard", sessionMgr.RequireAdmin(adminHandler.Dashboard))
	mux.HandleFunc("POST /admin/approve-user", sessionMgr.RequireAdmin(adminHandler.ApproveUser))

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("🚀 TFSA Tracker running on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
