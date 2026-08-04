package handlers

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"strconv"

	"tfsa-tracker/models"
)

//go:embed templates/admin.html
var adminTemplateFS embed.FS

type AnnualLimit struct {
	Year   int
	Amount float64
}

type AdminHandler struct {
	DB            *sql.DB
	adminTemplate *template.Template
}

type AdminViewData struct {
	Users        []models.User
	AnnualLimits []AnnualLimit
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	tmpl, err := template.ParseFS(adminTemplateFS, "templates/admin.html")
	if err != nil {
		panic("Failed to parse embedded admin template: " + err.Error())
	}

	return &AdminHandler{
		DB:            db,
		adminTemplate: tmpl,
	}
}

// AdminPanel renders all user registration requests & annual limits
func (h *AdminHandler) AdminPanel(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetAllUsers(h.DB)
	if err != nil {
		http.Error(w, "Failed to load users: "+err.Error(), http.StatusInternalServerError)
		return
	}

	limits, err := h.getAnnualLimits()
	if err != nil {
		http.Error(w, "Failed to load annual limits: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := AdminViewData{
		Users:        users,
		AnnualLimits: limits,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.adminTemplate.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ApproveUser approves a pending user account
func (h *AdminHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		userIDStr := r.FormValue("user_id")
		_, err := h.DB.Exec(`UPDATE users SET status = 'APPROVED' WHERE id = ?`, userIDStr)
		if err != nil {
			http.Error(w, "Failed to approve user: "+err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}

// UpdateAnnualLimit adds or updates a TFSA limit for a given year
func (h *AdminHandler) UpdateAnnualLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		year, _ := strconv.Atoi(r.FormValue("year"))
		amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)

		if year > 2000 && amount >= 0 {
			query := `
			INSERT INTO tfsa_annual_limits (year, amount) 
			VALUES (?, ?) 
			ON CONFLICT(year) DO UPDATE SET amount = excluded.amount`

			_, err := h.DB.Exec(query, year, amount)
			if err != nil {
				http.Error(w, "Failed to update limit: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}

func (h *AdminHandler) getAnnualLimits() ([]AnnualLimit, error) {
	rows, err := h.DB.Query("SELECT year, amount FROM tfsa_annual_limits ORDER BY year DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var limits []AnnualLimit
	for rows.Next() {
		var l AnnualLimit
		if err := rows.Scan(&l.Year, &l.Amount); err != nil {
			return nil, err
		}
		limits = append(limits, l)
	}
	return limits, nil
}
