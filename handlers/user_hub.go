package handlers

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"tfsa-tracker/auth"
	"tfsa-tracker/models"
	"tfsa-tracker/services"
	"tfsa-tracker/utils"
)

//go:embed templates/dashboard.html templates/accounts.html templates/yearly_checking.html
var templateFS embed.FS

type TFSAHandler struct {
	Service        *services.TFSAService
	dashboard      *template.Template
	accounts       *template.Template
	yearlyChecking *template.Template
}

type DashboardViewData struct {
	User         *models.User
	Accounts     []models.Account
	Summary      *models.TFSASummary
	Transactions []models.Transaction
	UserRole     models.UserRole
	SuccessMsg   string
	ErrMsg       string
}

type AccountsViewData struct {
	User     *models.User
	Accounts []models.Account
	UserRole models.UserRole
}

type YearlyCheckingViewData struct {
	User     *models.User
	Rows     []models.YearlyCheckingRow
	UserRole models.UserRole
}

func NewTFSAHandler(db *sql.DB) *TFSAHandler {
	dashTmpl, err := template.ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		panic("Failed to parse embedded dashboard template: " + err.Error())
	}

	acctTmpl, err := template.ParseFS(templateFS, "templates/accounts.html")
	if err != nil {
		panic("Failed to parse embedded accounts template: " + err.Error())
	}

	yearlyTmpl, err := template.ParseFS(templateFS, "templates/yearly_checking.html")
	if err != nil {
		panic("Failed to parse embedded yearly checking template: " + err.Error())
	}

	return &TFSAHandler{
		Service:        services.NewTFSAService(db),
		dashboard:      dashTmpl,
		accounts:       acctTmpl,
		yearlyChecking: yearlyTmpl,
	}
}

func (h *TFSAHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	role, _ := r.Context().Value(auth.RoleContextKey).(models.UserRole)

	user, err := models.GetUserByID(h.Service.DB, userID)
	if err != nil {
		http.Error(w, "Failed to load user profile", http.StatusInternalServerError)
		return
	}

	accounts, err := models.GetUserAccounts(h.Service.DB, userID)
	if err != nil {
		http.Error(w, "Failed to load user accounts: "+err.Error(), http.StatusInternalServerError)
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

	data := DashboardViewData{
		User:         user,
		Accounts:     accounts,
		Summary:      summary,
		Transactions: txs,
		UserRole:     role,
		SuccessMsg:   r.URL.Query().Get("success"),
		ErrMsg:       r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.dashboard.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *TFSAHandler) YearlyChecking(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	role, _ := r.Context().Value(auth.RoleContextKey).(models.UserRole)

	user, err := models.GetUserByID(h.Service.DB, userID)
	if err != nil {
		http.Error(w, "Failed to load user profile", http.StatusInternalServerError)
		return
	}

	rows, err := h.Service.CalculateYearlyCheckingHistory(userID)
	if err != nil {
		http.Error(w, "Failed to calculate yearly history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := YearlyCheckingViewData{
		User:     user,
		Rows:     rows,
		UserRole: role,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.yearlyChecking.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *TFSAHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		file, _, err := r.FormFile("csv_file")
		if err != nil {
			http.Redirect(w, r, "/dashboard?error="+err.Error(), http.StatusSeeOther)
			return
		}
		defer file.Close()

		imported, err := h.Service.ImportTransactionsCSV(userID, file)
		if err != nil {
			http.Redirect(w, r, "/dashboard?error="+err.Error(), http.StatusSeeOther)
			return
		}

		msg := fmt.Sprintf("CSV Import Complete! %d transactions imported.", imported)
		http.Redirect(w, r, "/dashboard?success="+msg, http.StatusSeeOther)
	}
}

func (h *TFSAHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := fmt.Sprintf("TFSA_Transactions_Export_%s.csv", time.Now().In(h.Service.Loc).Format("2006-01-02"))

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	if err := h.Service.ExportTransactionsCSV(userID, w); err != nil {
		http.Error(w, "Failed to export CSV: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *TFSAHandler) AccountsPage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	role, _ := r.Context().Value(auth.RoleContextKey).(models.UserRole)
	user, err := models.GetUserByID(h.Service.DB, userID)
	if err != nil {
		http.Error(w, "Failed to load user profile", http.StatusInternalServerError)
		return
	}

	accounts, err := models.GetUserAccounts(h.Service.DB, userID)
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := AccountsViewData{
		User:     user,
		Accounts: accounts,
		UserRole: role,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.accounts.Execute(w, data)
}

func (h *TFSAHandler) SaveAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		accountID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		acct := models.Account{
			ID:             accountID,
			UserID:         userID,
			AccountName:    r.FormValue("account_name"),
			AccountNameCRA: r.FormValue("account_name_cra"),
			AccountType:    r.FormValue("account_type"),
			Institution:    r.FormValue("institution"),
			AccountNumber:  r.FormValue("account_number"),
			OpeningDate:    r.FormValue("opening_date"),
			CloseDate:      r.FormValue("close_date"),
			Notes:          r.FormValue("notes"),
		}

		if acct.AccountType == "" {
			acct.AccountType = "TFSA"
		}

		var err error
		if acct.ID == 0 {
			err = models.AddAccount(h.Service.DB, &acct)
		} else {
			err = models.UpdateAccount(h.Service.DB, &acct)
		}

		if err != nil {
			http.Error(w, "Failed to save account: "+err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
	}
}

func (h *TFSAHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		accountID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if err := models.DeleteAccount(h.Service.DB, userID, accountID); err != nil {
			http.Error(w, "Failed to delete account: "+err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
	}
}

func (h *TFSAHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		startYear, _ := strconv.Atoi(r.FormValue("start_year"))
		phone := r.FormValue("phone_number")

		if err := models.UpdateUserProfile(h.Service.DB, userID, startYear, phone); err != nil {
			http.Error(w, "Failed to update profile: "+err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func (h *TFSAHandler) AddTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodPost {
		accountID, err := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
		if err != nil || accountID <= 0 {
			http.Error(w, "Please select a valid account", http.StatusBadRequest)
			return
		}

		txType := models.TransactionType(r.FormValue("type"))
		amountStr := r.FormValue("amount")

		amountCents, err := utils.DollarsToCents(amountStr)
		if err != nil {
			http.Error(w, "Invalid amount format: "+err.Error(), http.StatusBadRequest)
			return
		}

		dateStr := r.FormValue("date")
		note := r.FormValue("note")

		if dateStr == "" {
			dateStr = time.Now().In(h.Service.Loc).Format("2006-01-02")
		}

		err = h.Service.AddTransaction(userID, accountID, txType, amountCents, dateStr, note)
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

	if r.Method == http.MethodPost {
		txID, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		err := h.Service.DeleteTransaction(userID, txID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}
