package models

import "time"

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
	Amount    float64         `json:"amount"`
	Date      string          `json:"date"`
	Note      string          `json:"note"`
	CreatedAt time.Time       `json:"created_at"`
}

type TFSASummary struct {
	Year                 int     `json:"year"`
	UnusedRoomFromPrior  float64 `json:"unused_room_from_prior"`
	PriorYearWithdrawals float64 `json:"prior_year_withdrawals"`
	NewRoomThisYear      float64 `json:"new_room_this_year"`
	TotalRoomAvailable   float64 `json:"total_room_available"`
	TotalDeposited       float64 `json:"total_deposited"`
	TotalWithdrawn       float64 `json:"total_withdrawn"`
	RemainingRoom        float64 `json:"remaining_room"`
}
