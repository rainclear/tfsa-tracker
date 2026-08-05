package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"tfsa-tracker/auth"
	"tfsa-tracker/config"
	"tfsa-tracker/handlers"
	"tfsa-tracker/models"

	_ "modernc.org/sqlite"
)

var staticFS embed.FS

func main() {
	cfg := config.LoadConfig()

	// Ensure DB directory exists if mounting a volume
	if err := os.MkdirAll("/mnt/db", 0755); err != nil && !os.IsExist(err) {
		log.Printf("Warning: Could not create /mnt/db directory: %v", err)
	}

	db, err := models.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	sessionMgr := auth.NewSessionManager()
	authHandler := handlers.NewAuthHandler(db, sessionMgr, cfg)
	tfsaHandler := handlers.NewTFSAHandler(db)
	adminHandler := handlers.NewAdminHandler(db)

	mux := http.NewServeMux()

	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/static/", fileServer)

	// 1. Root route handler (Redirects / to /dashboard)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	// 2. Auth Routes
	mux.HandleFunc("/login", authHandler.Login)
	mux.HandleFunc("/register", authHandler.Register)
	mux.HandleFunc("/logout", authHandler.Logout)
	mux.HandleFunc("/activate", authHandler.Activate)

	// 3. Protected Dashboard & Account Routes
	mux.HandleFunc("/dashboard", sessionMgr.RequireAuth(tfsaHandler.Dashboard))
	mux.HandleFunc("/accounts", sessionMgr.RequireAuth(tfsaHandler.AccountsPage))
	mux.HandleFunc("/accounts/save", sessionMgr.RequireAuth(tfsaHandler.SaveAccount))
	mux.HandleFunc("/accounts/delete", sessionMgr.RequireAuth(tfsaHandler.DeleteAccount))
	mux.HandleFunc("/user/profile/update", sessionMgr.RequireAuth(tfsaHandler.UpdateProfile))
	mux.HandleFunc("/transaction/add", sessionMgr.RequireAuth(tfsaHandler.AddTransaction))
	mux.HandleFunc("/transaction/delete", sessionMgr.RequireAuth(tfsaHandler.DeleteTransaction))

	// 4. Protected Admin Routes
	mux.HandleFunc("/admin", sessionMgr.RequireAdmin(adminHandler.AdminPanel))
	mux.HandleFunc("/admin/approve", sessionMgr.RequireAdmin(adminHandler.ApproveUser))
	mux.HandleFunc("/admin/limit/update", sessionMgr.RequireAdmin(adminHandler.UpdateAnnualLimit))
	mux.HandleFunc("/admin/limit/delete", sessionMgr.RequireAdmin(adminHandler.DeleteAnnualLimit))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server listening on port %s...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
