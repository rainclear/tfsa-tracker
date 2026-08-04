package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"tfsa-tracker/models"
)

type TFSAService struct {
	DB *sql.DB
}

func NewTFSAService(db *sql.DB) *TFSAService {
	return &TFSAService{DB: db}
}

func (s *TFSAService) GetAnnualLimits() (map[int]float64, error) {
	rows, err := s.DB.Query(`SELECT year, amount FROM tfsa_annual_limits ORDER BY year ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	limits := make(map[int]float64)
	for rows.Next() {
		var year int
		var amount float64
		if err := rows.Scan(&year, &amount); err != nil {
			return nil, err
		}
		limits[year] = amount
	}
	return limits, nil
}

// CalculateSummaryForYear calculates contribution room breakdown based on user's StartYear
func (s *TFSAService) CalculateSummaryForYear(userID int64, targetYear int) (*models.TFSASummary, error) {
	user, err := models.GetUserByID(s.DB, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	startYear := user.StartYear
	if startYear < 2009 {
		startYear = 2009
	}
	if targetYear < startYear {
		targetYear = startYear
	}

	// Dynamically ensure annual limits exist up to targetYear
	for y := startYear; y <= targetYear; y++ {
		if err := models.EnsureAnnualLimitExists(s.DB, y); err != nil {
			log.Printf("Warning: Failed to ensure annual limit for year %d: %v", y, err)
		}
	}

	limits, err := s.GetAnnualLimits()
	if err != nil {
		return nil, err
	}

	var unusedRoomPrior float64 = 0
	var priorYearWithdrawals float64 = 0

	// Chronologically calculate year by year starting from user's StartYear
	for y := startYear; y < targetYear; y++ {
		newRoom := limits[y]
		totalAvailable := unusedRoomPrior + priorYearWithdrawals + newRoom

		var deposited, withdrawn float64
		s.DB.QueryRow(`
			SELECT COALESCE(SUM(CASE WHEN type = 'DEPOSIT' THEN amount ELSE 0 END), 0),
			       COALESCE(SUM(CASE WHEN type = 'WITHDRAWAL' THEN amount ELSE 0 END), 0)
			FROM transactions 
			WHERE user_id = ? AND strftime('%Y', date) = ?
		`, userID, fmt.Sprintf("%d", y)).Scan(&deposited, &withdrawn)

		remaining := totalAvailable - deposited
		if remaining < 0 {
			remaining = 0
		}

		unusedRoomPrior = remaining
		priorYearWithdrawals = withdrawn
	}

	// Current target year calculation
	newRoomThisYear := limits[targetYear]
	totalAvailableThisYear := unusedRoomPrior + priorYearWithdrawals + newRoomThisYear

	var depositedThisYear, withdrawnThisYear float64
	s.DB.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN type = 'DEPOSIT' THEN amount ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN type = 'WITHDRAWAL' THEN amount ELSE 0 END), 0)
		FROM transactions 
		WHERE user_id = ? AND strftime('%Y', date) = ?
	`, userID, fmt.Sprintf("%d", targetYear)).Scan(&depositedThisYear, &withdrawnThisYear)

	remainingRoom := totalAvailableThisYear - depositedThisYear

	return &models.TFSASummary{
		Year:                 targetYear,
		UnusedRoomFromPrior:  unusedRoomPrior,
		PriorYearWithdrawals: priorYearWithdrawals,
		NewRoomThisYear:      newRoomThisYear,
		TotalRoomAvailable:   totalAvailableThisYear,
		TotalDeposited:       depositedThisYear,
		TotalWithdrawn:       withdrawnThisYear,
		RemainingRoom:        remainingRoom,
	}, nil
}

func (s *TFSAService) CalculateCurrentSummary(userID int64) (*models.TFSASummary, error) {
	currentYear := time.Now().Year()
	return s.CalculateSummaryForYear(userID, currentYear)
}

func (s *TFSAService) GetUserTransactions(userID int64) ([]models.Transaction, error) {
	rows, err := s.DB.Query(`
		SELECT id, user_id, type, amount, date, note, created_at 
		FROM transactions 
		WHERE user_id = ? 
		ORDER BY date DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []models.Transaction
	for rows.Next() {
		var tx models.Transaction
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.Type, &tx.Amount, &tx.Date, &tx.Note, &tx.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

func (s *TFSAService) AddTransaction(userID int64, txType models.TransactionType, amount float64, dateStr, note string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}

	_, err := s.DB.Exec(`
		INSERT INTO transactions (user_id, type, amount, date, note)
		VALUES (?, ?, ?, ?, ?)
	`, userID, txType, amount, dateStr, note)
	return err
}

func (s *TFSAService) DeleteTransaction(userID, transactionID int64) error {
	res, err := s.DB.Exec(`DELETE FROM transactions WHERE id = ? AND user_id = ?`, transactionID, userID)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("transaction not found or access denied")
	}
	return nil
}
