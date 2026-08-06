package models

import (
	"database/sql"
	"fmt"
	"time"
)

type EmailToken struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}

type Account struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	AccountName    string    `json:"account_name"`
	AccountNameCRA string    `json:"account_name_cra"`
	AccountType    string    `json:"account_type"`
	Institution    string    `json:"institution"`
	AccountNumber  string    `json:"account_number"`
	OpeningDate    string    `json:"opening_date"`
	CloseDate      string    `json:"close_date"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
}

type TransactionType string

const (
	Deposit    TransactionType = "DEPOSIT"
	Withdrawal TransactionType = "WITHDRAWAL"
)

type Transaction struct {
	ID          int64           `json:"id"`
	UserID      int64           `json:"user_id"`
	AccountID   int64           `json:"account_id"`
	AccountName string          `json:"account_name"`
	Type        TransactionType `json:"type"`
	Amount      int64           `json:"amount"` // Stored in CENTS
	Date        string          `json:"date"`
	Note        string          `json:"note"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (t Transaction) AmountInDollars() float64 {
	return float64(t.Amount) / 100.0
}

func (t Transaction) FormattedDate() string {
	if len(t.Date) >= 10 {
		return t.Date[:10]
	}
	return t.Date
}

type TFSASummary struct {
	Year                 int   `json:"year"`
	UnusedRoomFromPrior  int64 `json:"unused_room_from_prior"`
	PriorYearWithdrawals int64 `json:"prior_year_withdrawals"`
	NewRoomThisYear      int64 `json:"new_room_this_year"`
	TotalRoomAvailable   int64 `json:"total_room_available"`
	TotalDeposited       int64 `json:"total_deposited"`
	TotalWithdrawn       int64 `json:"total_withdrawn"`
	RemainingRoom        int64 `json:"remaining_room"`
}

func (s TFSASummary) RemainingRoomDollars() float64   { return float64(s.RemainingRoom) / 100.0 }
func (s TFSASummary) TotalDepositedDollars() float64  { return float64(s.TotalDeposited) / 100.0 }
func (s TFSASummary) TotalWithdrawnDollars() float64  { return float64(s.TotalWithdrawn) / 100.0 }
func (s TFSASummary) NewRoomThisYearDollars() float64 { return float64(s.NewRoomThisYear) / 100.0 }

// YearlyCheckingRow represents a row in the dynamic Yearly Checking view
type YearlyCheckingRow struct {
	Year           int   `json:"year"`
	NewRoom        int64 `json:"new_room"`         // Cents
	TotalStartRoom int64 `json:"total_start_room"` // Cents
	Deposit        int64 `json:"deposit"`          // Cents
	Withdrawal     int64 `json:"withdrawal"`       // Cents
	RemainingRoom  int64 `json:"remaining_room"`   // Cents
	IsOverLimit    bool  `json:"is_over_limit"`
}

func (r YearlyCheckingRow) NewRoomDollars() float64        { return float64(r.NewRoom) / 100.0 }
func (r YearlyCheckingRow) TotalStartRoomDollars() float64 { return float64(r.TotalStartRoom) / 100.0 }
func (r YearlyCheckingRow) DepositDollars() float64        { return float64(r.Deposit) / 100.0 }
func (r YearlyCheckingRow) WithdrawalDollars() float64     { return float64(r.Withdrawal) / 100.0 }
func (r YearlyCheckingRow) RemainingRoomDollars() float64  { return float64(r.RemainingRoom) / 100.0 }

type AnnualLimit struct {
	Year   int   `json:"year"`
	Amount int64 `json:"amount"`
}

func (a AnnualLimit) AmountInDollars() float64 {
	return float64(a.Amount) / 100.0
}

// Account Database Methods

func GetUserAccounts(db *sql.DB, userID int64) ([]Account, error) {
	rows, err := db.Query(`
		SELECT id, user_id, account_name, account_name_cra, account_type, institution, account_number, COALESCE(opening_date, ''), COALESCE(close_date, ''), COALESCE(notes, ''), created_at 
		FROM accounts 
		WHERE user_id = ? 
		ORDER BY account_name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.UserID, &a.AccountName, &a.AccountNameCRA, &a.AccountType, &a.Institution, &a.AccountNumber, &a.OpeningDate, &a.CloseDate, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

func AddAccount(db *sql.DB, a *Account) error {
	_, err := db.Exec(`
		INSERT INTO accounts (user_id, account_name, account_name_cra, account_type, institution, account_number, opening_date, close_date, notes)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
	`, a.UserID, a.AccountName, a.AccountNameCRA, a.AccountType, a.Institution, a.AccountNumber, a.OpeningDate, a.CloseDate, a.Notes)
	return err
}

func UpdateAccount(db *sql.DB, a *Account) error {
	res, err := db.Exec(`
		UPDATE accounts 
		SET account_name = ?, account_name_cra = ?, account_type = ?, institution = ?, account_number = ?, opening_date = NULLIF(?, ''), close_date = NULLIF(?, ''), notes = ?
		WHERE id = ? AND user_id = ?
	`, a.AccountName, a.AccountNameCRA, a.AccountType, a.Institution, a.AccountNumber, a.OpeningDate, a.CloseDate, a.Notes, a.ID, a.UserID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("account not found or unauthorized")
	}
	return nil
}

func DeleteAccount(db *sql.DB, userID, accountID int64) error {
	res, err := db.Exec(`DELETE FROM accounts WHERE id = ? AND user_id = ?`, accountID, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("account not found or unauthorized")
	}
	return nil
}

type CRASummaryDetail struct {
	Date          string
	FormattedDate string
	Contribution  int64
	Withdrawal    int64
	TransCount    int
}

func (d CRASummaryDetail) ContributionDollars() float64 {
	return float64(d.Contribution) / 100.0
}

func (d CRASummaryDetail) WithdrawalDollars() float64 {
	return float64(d.Withdrawal) / 100.0
}

type CRASummaryRow struct {
	AccountNameCRA string
	AccountName    string
	Contribution   int64
	Withdrawal     int64
	TransCount     int
	Details        []CRASummaryDetail
}

func (r CRASummaryRow) ContributionDollars() float64 {
	return float64(r.Contribution) / 100.0
}

func (r CRASummaryRow) WithdrawalDollars() float64 {
	return float64(r.Withdrawal) / 100.0
}

type CRASummaryTotal struct {
	TotalContribution int64
	TotalWithdrawal   int64
	TotalTransCount   int
}

func (t CRASummaryTotal) ContributionDollars() float64 {
	return float64(t.TotalContribution) / 100.0
}

func (t CRASummaryTotal) WithdrawalDollars() float64 {
	return float64(t.TotalWithdrawal) / 100.0
}
