package handlers

import (
	"database/sql"
	"fmt"
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

func NewTFSAHandler(db *sql.DB) *TFSAHandler {
	return &TFSAHandler{
		Service: services.NewTFSAService(db),
	}
}

// Dashboard shows the TFSA contribution room summary and transaction history
func (h *TFSAHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

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

	// Basic HTML response (Replace with html/template in UI step)
	fmt.Fprintf(w, "<h1>TFSA Dashboard (%d)</h1>", summary.Year)
	fmt.Fprintf(w, "<p>Available Room: $%.2f</p>", summary.RemainingRoom)
	fmt.Fprintf(w, "<p>Total Deposited This Year: $%.2f</p>", summary.TotalDeposited)
	fmt.Fprintf(w, "<p>Total Withdrawn This Year: $%.2f</p>", summary.TotalWithdrawn)

	fmt.Fprintf(w, "<h2>Recent Transactions</h2><ul>")
	for _, tx := range txs {
		fmt.Fprintf(w, "<li>[%s] %s - $%.2f (%s)</li>", tx.Date, tx.Type, tx.Amount, tx.Note)
	}
	fmt.Fprintf(w, "</ul>")
}

// AddTransaction handles adding a deposit or withdrawal
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

// DeleteTransaction handles transaction deletion
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
