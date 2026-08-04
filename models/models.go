package models

import (
	"time"
)

type EmailToken struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}

type TransactionType string

const (
	Deposit    TransactionType = "DEPOSIT"
	Withdrawal TransactionType = "WITHDRAWAL"
)

type Transaction struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id"`
	Type      TransactionType `json:"type"`
	Amount    int64           `json:"amount"` // Stored in CENTS (e.g., $101.23 -> 10123)
	Date      string          `json:"date"`
	Note      string          `json:"note"`
	CreatedAt time.Time       `json:"created_at"`
}

// AmountInDollars returns a formatted float or string for UI display
func (t Transaction) AmountInDollars() float64 {
	return float64(t.Amount) / 100.0
}

type TFSASummary struct {
	Year                 int   `json:"year"`
	UnusedRoomFromPrior  int64 `json:"unused_room_from_prior"` // Cents
	PriorYearWithdrawals int64 `json:"prior_year_withdrawals"` // Cents
	NewRoomThisYear      int64 `json:"new_room_this_year"`     // Cents
	TotalRoomAvailable   int64 `json:"total_room_available"`   // Cents
	TotalDeposited       int64 `json:"total_deposited"`        // Cents
	TotalWithdrawn       int64 `json:"total_withdrawn"`        // Cents
	RemainingRoom        int64 `json:"remaining_room"`         // Cents
}

// Helpers for Template Rendering ($ dollars)
func (s TFSASummary) RemainingRoomDollars() float64   { return float64(s.RemainingRoom) / 100.0 }
func (s TFSASummary) TotalDepositedDollars() float64  { return float64(s.TotalDeposited) / 100.0 }
func (s TFSASummary) TotalWithdrawnDollars() float64  { return float64(s.TotalWithdrawn) / 100.0 }
func (s TFSASummary) NewRoomThisYearDollars() float64 { return float64(s.NewRoomThisYear) / 100.0 }

type AnnualLimit struct {
	Year   int   `json:"year"`
	Amount int64 `json:"amount"` // Cents
}

func (a AnnualLimit) AmountInDollars() float64 {
	return float64(a.Amount) / 100.0
}

func (t Transaction) FormattedDate() string {
	if len(t.Date) >= 10 {
		return t.Date[:10]
	}
	return t.Date
}
