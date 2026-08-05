package services

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"tfsa-tracker/models"
	"tfsa-tracker/utils"
)

type TFSAService struct {
	DB  *sql.DB
	Loc *time.Location
}

func NewTFSAService(db *sql.DB) *TFSAService {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		log.Printf("Warning: Could not load America/Toronto location, falling back to UTC: %v", err)
		loc = time.UTC
	}
	return &TFSAService{
		DB:  db,
		Loc: loc,
	}
}

func (s *TFSAService) GetAnnualLimits() (map[int]int64, error) {
	rows, err := s.DB.Query(`SELECT year, amount FROM tfsa_annual_limits ORDER BY year ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	limits := make(map[int]int64)
	for rows.Next() {
		var year int
		var amount int64
		if err := rows.Scan(&year, &amount); err != nil {
			return nil, err
		}
		limits[year] = amount
	}
	return limits, nil
}

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

	for y := startYear; y <= targetYear; y++ {
		if err := models.EnsureAnnualLimitExists(s.DB, y); err != nil {
			log.Printf("Warning: Failed to ensure annual limit for year %d: %v", y, err)
		}
	}

	limits, err := s.GetAnnualLimits()
	if err != nil {
		return nil, err
	}

	var unusedRoomPrior int64 = 0
	var priorYearWithdrawals int64 = 0

	for y := startYear; y < targetYear; y++ {
		newRoom := limits[y]
		totalAvailable := unusedRoomPrior + priorYearWithdrawals + newRoom

		var deposited, withdrawn int64
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

	newRoomThisYear := limits[targetYear]
	totalAvailableThisYear := unusedRoomPrior + priorYearWithdrawals + newRoomThisYear

	var depositedThisYear, withdrawnThisYear int64
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
	currentYear := time.Now().In(s.Loc).Year()
	return s.CalculateSummaryForYear(userID, currentYear)
}

func (s *TFSAService) CalculateYearlyCheckingHistory(userID int64) ([]models.YearlyCheckingRow, error) {
	user, err := models.GetUserByID(s.DB, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	startYear := user.StartYear
	if startYear < 2009 {
		startYear = 2009
	}
	currentYear := time.Now().In(s.Loc).Year()

	for y := startYear; y <= currentYear; y++ {
		_ = models.EnsureAnnualLimitExists(s.DB, y)
	}

	limits, err := s.GetAnnualLimits()
	if err != nil {
		return nil, err
	}

	var history []models.YearlyCheckingRow
	var unusedRoomPrior int64 = 0
	var priorYearWithdrawals int64 = 0

	for y := startYear; y <= currentYear; y++ {
		newRoom := limits[y]
		totalStartRoom := unusedRoomPrior + priorYearWithdrawals + newRoom

		var deposited, withdrawn int64
		s.DB.QueryRow(`
			SELECT COALESCE(SUM(CASE WHEN type = 'DEPOSIT' THEN amount ELSE 0 END), 0),
			       COALESCE(SUM(CASE WHEN type = 'WITHDRAWAL' THEN amount ELSE 0 END), 0)
			FROM transactions 
			WHERE user_id = ? AND strftime('%Y', date) = ?
		`, userID, fmt.Sprintf("%d", y)).Scan(&deposited, &withdrawn)

		remainingRoom := totalStartRoom - deposited
		isOverLimit := remainingRoom < 0

		history = append(history, models.YearlyCheckingRow{
			Year:           y,
			NewRoom:        newRoom,
			TotalStartRoom: totalStartRoom,
			Deposit:        deposited,
			Withdrawal:     withdrawn,
			RemainingRoom:  remainingRoom,
			IsOverLimit:    isOverLimit,
		})

		if remainingRoom < 0 {
			unusedRoomPrior = 0
		} else {
			unusedRoomPrior = remainingRoom
		}
		priorYearWithdrawals = withdrawn
	}

	return history, nil
}

func (s *TFSAService) GetUserTransactions(userID int64) ([]models.Transaction, error) {
	rows, err := s.DB.Query(`
		SELECT t.id, t.user_id, t.account_id, COALESCE(a.account_name, 'Unknown Account'), t.type, t.amount, t.date, t.note, t.created_at 
		FROM transactions t
		LEFT JOIN accounts a ON t.account_id = a.id
		WHERE t.user_id = ? 
		ORDER BY t.date DESC, t.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []models.Transaction
	for rows.Next() {
		var tx models.Transaction
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.AccountID, &tx.AccountName, &tx.Type, &tx.Amount, &tx.Date, &tx.Note, &tx.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

func (s *TFSAService) AddTransaction(userID, accountID int64, txType models.TransactionType, amountCents int64, dateStr, note string) error {
	if amountCents <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}

	if accountID <= 0 {
		return fmt.Errorf("a valid account must be selected")
	}

	if dateStr == "" {
		dateStr = time.Now().In(s.Loc).Format("2006-01-02")
	}

	_, err := s.DB.Exec(`
		INSERT INTO transactions (user_id, account_id, type, amount, date, note)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, accountID, txType, amountCents, dateStr, note)
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

func (s *TFSAService) ImportTransactionsCSV(userID int64, fileReader io.Reader) (int, error) {
	reader := csv.NewReader(fileReader)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("failed to read CSV header: %w", err)
	}

	dateIdx, acctNameIdx, acctCraIdx, depIdx, wdIdx := -1, -1, -1, -1, -1

	for i, h := range header {
		cleanH := strings.ToLower(strings.TrimSpace(h))

		// Check for CRA Account Name first to avoid collision with standard Account Name
		if strings.Contains(cleanH, "account name at cra") || cleanH == "account_name_cra" {
			acctCraIdx = i
		} else if strings.Contains(cleanH, "transaction date") || cleanH == "date" {
			dateIdx = i
		} else if strings.HasPrefix(cleanH, "deposit") || cleanH == "deposit" {
			depIdx = i
		} else if strings.HasPrefix(cleanH, "withdrawal") || cleanH == "withdrawal" {
			wdIdx = i
		} else if strings.Contains(cleanH, "account") && acctNameIdx == -1 {
			acctNameIdx = i
		}
	}

	// Fallbacks if exact headers were slightly different
	if wdIdx == -1 {
		for i, h := range header {
			cleanH := strings.ToLower(strings.TrimSpace(h))
			if strings.Contains(cleanH, "withdrawal") && !strings.Contains(cleanH, "there cannot be") {
				wdIdx = i
				break
			}
		}
	}

	if dateIdx == -1 || acctNameIdx == -1 || acctCraIdx == -1 || depIdx == -1 || wdIdx == -1 {
		return 0, fmt.Errorf("CSV missing required columns. Header identified -> Date: %d, Account: %d, CRA Name: %d, Deposit: %d, Withdrawal: %d",
			dateIdx, acctNameIdx, acctCraIdx, depIdx, wdIdx)
	}

	accountsMap := make(map[string]int64)
	existingAccounts, err := models.GetUserAccounts(s.DB, userID)
	if err != nil {
		return 0, err
	}
	for _, a := range existingAccounts {
		accountsMap[strings.ToLower(a.AccountName)] = a.ID
	}

	importedCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		maxRequiredIdx := max(dateIdx, max(acctNameIdx, max(acctCraIdx, max(depIdx, wdIdx))))
		if len(record) <= maxRequiredIdx {
			continue
		}

		dateStr := strings.TrimSpace(record[dateIdx])
		acctName := strings.TrimSpace(record[acctNameIdx])
		acctCraName := strings.TrimSpace(record[acctCraIdx])

		if dateStr == "" || acctName == "" {
			continue
		}

		acctKey := strings.ToLower(acctName)
		accountID, exists := accountsMap[acctKey]
		if !exists {
			inst := acctName
			if parts := strings.Split(acctName, " "); len(parts) > 0 {
				inst = parts[0]
			}

			newAcct := models.Account{
				UserID:         userID,
				AccountName:    acctName,
				AccountNameCRA: acctCraName,
				AccountType:    "TFSA",
				Institution:    inst,
				Notes:          "Auto-created from CSV import",
			}

			if err := models.AddAccount(s.DB, &newAcct); err != nil {
				_ = s.DB.QueryRow(`SELECT id FROM accounts WHERE user_id = ? AND account_name = ?`, userID, acctName).Scan(&accountID)
			} else {
				_ = s.DB.QueryRow(`SELECT id FROM accounts WHERE user_id = ? AND account_name = ?`, userID, acctName).Scan(&accountID)
			}
			accountsMap[acctKey] = accountID
		}

		// 1. Process Deposit
		depStr := cleanCurrencyString(record[depIdx])
		if depStr != "" && depStr != "0" && depStr != "0.00" {
			depCents, err := utils.DollarsToCents(depStr)
			if err == nil && depCents > 0 {
				_, err = s.DB.Exec(`
					INSERT INTO transactions (user_id, account_id, type, amount, date, note)
					VALUES (?, ?, 'DEPOSIT', ?, ?, ?)
				`, userID, accountID, depCents, dateStr, "Imported from CSV")
				if err == nil {
					importedCount++
				}
			}
		}

		// 2. Process Withdrawal
		wdStr := cleanCurrencyString(record[wdIdx])
		if wdStr != "" && wdStr != "0" && wdStr != "0.00" {
			wdCents, err := utils.DollarsToCents(wdStr)
			if err == nil && wdCents > 0 {
				_, err = s.DB.Exec(`
					INSERT INTO transactions (user_id, account_id, type, amount, date, note)
					VALUES (?, ?, 'WITHDRAWAL', ?, ?, ?)
				`, userID, accountID, wdCents, dateStr, "Imported from CSV")
				if err == nil {
					importedCount++
				}
			}
		}
	}

	return importedCount, nil
}

func (s *TFSAService) ExportTransactionsCSV(userID int64, w io.Writer) error {
	// Query transactions ordered by date ASC, then account name ASC
	rows, err := s.DB.Query(`
		SELECT t.type, t.amount, t.date, COALESCE(a.account_name, ''), COALESCE(a.account_name_cra, '')
		FROM transactions t
		LEFT JOIN accounts a ON t.account_id = a.id
		WHERE t.user_id = ?
		ORDER BY t.date ASC, a.account_name ASC, t.id ASC
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to query transactions for export: %w", err)
	}
	defer rows.Close()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write exact header format matching the import CSV template
	header := []string{
		"Index (PK)",
		"Transaction Date",
		"Account (Dropdown based on Account Name in Accounts Sheet)",
		"Account Name at CRA (Auto Fill)",
		"Deposit (Non Negative)",
		"Withdrawal (Non Negative)",
		"There cannot be both Contribution and Withdrawal in one transaction",
	}

	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	index := 1
	for rows.Next() {
		var txType string
		var amountCents int64
		var dateStr, acctName, acctCraName string

		if err := rows.Scan(&txType, &amountCents, &dateStr, &acctName, &acctCraName); err != nil {
			return fmt.Errorf("failed to scan transaction row: %w", err)
		}

		formattedAmount := fmt.Sprintf("$%.2f", float64(amountCents)/100.0)

		depositVal := ""
		withdrawalVal := ""

		if txType == "DEPOSIT" {
			depositVal = formattedAmount
		} else if txType == "WITHDRAWAL" {
			withdrawalVal = formattedAmount
		}

		record := []string{
			strconv.Itoa(index),
			dateStr,
			acctName,
			acctCraName,
			depositVal,
			withdrawalVal,
			"", // Explanatory note column left empty
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write transaction row to CSV: %w", err)
		}

		index++
	}

	return rows.Err()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cleanCurrencyString(val string) string {
	val = strings.TrimSpace(val)
	val = strings.ReplaceAll(val, "$", "")
	val = strings.ReplaceAll(val, ",", "")
	val = strings.ReplaceAll(val, "\"", "")
	return val
}
