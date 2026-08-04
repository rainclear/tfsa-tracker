package handlers

import (
	"bytes"
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"tfsa-tracker/auth"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
)

//go:embed templates/dashboard.html
var templateFS embed.FS

type TFSAHandler struct {
	Service   *services.TFSAService
	dashboard *template.Template
}

type DashboardViewData struct {
	Summary      *models.TFSASummary
	Transactions []models.Transaction
	UserRole     models.UserRole
}

func NewTFSAHandler(db *sql.DB) *TFSAHandler {
	// Parse with explicit template base name to ensure define blocks are recognized properly
	tmpl, err := template.New("dashboard.html").ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		panic("Failed to parse embedded dashboard template: " + err.Error())
	}

	return &TFSAHandler{
		Service:   services.NewTFSAService(db),
		dashboard: tmpl,
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

	data := DashboardViewData{
		Summary:      summary,
		Transactions: txs,
		UserRole:     role,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.dashboard.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
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

		if r.Header.Get("HX-Request") == "true" {
			h.renderUpdatedComponents(w, userID)
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

	if r.Header.Get("HX-Request") == "true" {
		h.renderUpdatedComponents(w, userID)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *TFSAHandler) renderUpdatedComponents(w http.ResponseWriter, userID int64) {
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

	var buf bytes.Buffer

	// 1. Render transaction rows (Target: #transaction-list)
	if err := h.dashboard.ExecuteTemplate(&buf, "transaction-rows", txs); err != nil {
		http.Error(w, "Error rendering rows: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Render summary cards via OOB (Target: #summary-cards)
	if err := h.dashboard.ExecuteTemplate(&buf, "summary-cards", summary); err != nil {
		http.Error(w, "Error rendering summary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
