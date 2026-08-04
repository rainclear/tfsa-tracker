package handlers

import (
	"database/sql"
	"embed"
	"html/template"
	"math"
	"net/http"
	"strconv"

	"tfsa-tracker/models"
)

//go:embed templates/admin.html
var adminFS embed.FS

type AdminHandler struct {
	DB    *sql.DB
	admin *template.Template
}

type AdminViewData struct {
	Users        []models.User
	AnnualLimits []models.AnnualLimit
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	tmpl, err := template.ParseFS(adminFS, "templates/admin.html")
	if err != nil {
		panic("Failed to parse embedded admin template: " + err.Error())
	}

	return &AdminHandler{
		DB:    db,
		admin: tmpl,
	}
}

func (h *AdminHandler) AdminPanel(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetAllUsers(h.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := h.DB.Query(`SELECT year, amount FROM tfsa_annual_limits ORDER BY year DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var limits []models.AnnualLimit
	for rows.Next() {
		var l models.AnnualLimit
		if err := rows.Scan(&l.Year, &l.Amount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		limits = append(limits, l)
	}

	data := AdminViewData{
		Users:        users,
		AnnualLimits: limits,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.admin.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *AdminHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		userID, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		_, err := h.DB.Exec(`UPDATE users SET status = 'APPROVED' WHERE id = ?`, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}

func (h *AdminHandler) UpdateAnnualLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		year, _ := strconv.Atoi(r.FormValue("year"))
		amountFloat, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
		amountCents := int64(math.Round(amountFloat * 100)) // Convert $7000.00 to 700000 cents

		if year >= 2009 && amountCents >= 0 {
			_, err := h.DB.Exec(`
				INSERT INTO tfsa_annual_limits (year, amount) VALUES (?, ?)
				ON CONFLICT(year) DO UPDATE SET amount = excluded.amount
			`, year, amountCents)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}
