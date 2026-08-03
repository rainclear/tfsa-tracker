package handlers

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"tfsa-tracker/auth"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

type TFSAHandler struct {
	Service *services.TFSAService
}

type DashboardViewData struct {
	Summary      *models.TFSASummary
	Transactions []models.Transaction
	UserRole     models.UserRole
}

func NewTFSAHandler(db *sql.DB) *TFSAHandler {
	return &TFSAHandler{
		Service: services.NewTFSAService(db),
	}
}

func (h *TFSAHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	role, _ := r.Context().Value(auth.RoleContextKey).(models.UserRole)

	summary, err := h.Service.CalculateCurrentSummary(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	txs, err := h.Service.GetUserTransactions(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		http.Error(w, "Error loading template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := DashboardViewData{
		Summary:      summary,
		Transactions: txs,
		UserRole:     role,
	}

	tmpl.Execute(w, data)
}

func (h *TFSAHandler) AddTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		txType := models.TransactionType(r.FormValue("type"))
		amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
		dateStr := r.FormValue("date")
		note := r.FormValue("note")

		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		}

		err := h.Service.AddTransaction(userID, txType, amount, dateStr, note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func (h *TFSAHandler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	txID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	err := h.Service.DeleteTransaction(userID, txID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
