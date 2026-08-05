package models

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("failed to set PRAGMA: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	if err := seedAnnualLimits(db); err != nil {
		return nil, fmt.Errorf("failed to seed annual limits: %w", err)
	}

	log.Println("✅ Database initialized with multi-account support and per-user unique constraints")
	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('ADMIN', 'USER')) DEFAULT 'USER',
		status TEXT NOT NULL CHECK(status IN ('PENDING', 'PENDING_ACTIVATION', 'APPROVED', 'REJECTED')) DEFAULT 'PENDING',
		start_year INTEGER DEFAULT 2009,
		phone_number TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS email_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- Accounts Table linked to User with per-user unique account & CRA names
	CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		account_name TEXT NOT NULL,
		account_name_cra TEXT NOT NULL,
		account_type TEXT DEFAULT 'TFSA',
		institution TEXT DEFAULT '',
		account_number TEXT DEFAULT '',
		opening_date DATE,
		close_date DATE,
		notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(user_id, account_name),
		UNIQUE(user_id, account_name_cra)
	);

	-- Transactions Table linked to Account and User
	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('DEPOSIT', 'WITHDRAWAL')),
		amount INTEGER NOT NULL CHECK(amount > 0), -- Stored in CENTS
		date DATE NOT NULL,
		note TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);

	-- Indexes for query performance
	CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts(user_id);
	CREATE INDEX IF NOT EXISTS idx_transactions_user_date ON transactions(user_id, date);
	CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account_id);

	CREATE TABLE IF NOT EXISTS tfsa_annual_limits (
		year INTEGER PRIMARY KEY,
		amount INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	INSERT OR IGNORE INTO system_settings (key, value) VALUES ('registration_enabled', 'true');
	`
	_, err := db.Exec(schema)
	return err
}

func EnsureAnnualLimitExists(db *sql.DB, targetYear int) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tfsa_annual_limits WHERE year = ?", targetYear).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check annual limit for year %d: %w", targetYear, err)
	}

	if count > 0 {
		return nil
	}

	var previousYear int
	var previousAmountCents int64
	err = db.QueryRow("SELECT year, amount FROM tfsa_annual_limits WHERE year < ? ORDER BY year DESC LIMIT 1", targetYear).Scan(&previousYear, &previousAmountCents)

	if err == sql.ErrNoRows {
		previousAmountCents = 500000 // $5000.00
	} else if err != nil {
		return fmt.Errorf("failed to retrieve historical limit before %d: %w", targetYear, err)
	}

	_, err = db.Exec("INSERT INTO tfsa_annual_limits (year, amount) VALUES (?, ?)", targetYear, previousAmountCents)
	if err != nil {
		return fmt.Errorf("failed to auto-insert limit for year %d: %w", targetYear, err)
	}

	return nil
}

func seedAnnualLimits(db *sql.DB) error {
	limits := map[int]int64{
		2009: 500000, 2010: 500000, 2011: 500000, 2012: 500000,
		2013: 550000, 2014: 550000, 2015: 1000000, 2016: 550000,
		2017: 550000, 2018: 550000, 2019: 600000, 2020: 600000,
		2021: 600000, 2022: 600000, 2023: 650000, 2024: 700000,
		2025: 700000, 2026: 700000,
	}

	stmt, err := db.Prepare("INSERT OR IGNORE INTO tfsa_annual_limits (year, amount) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for year, amount := range limits {
		if _, err := stmt.Exec(year, amount); err != nil {
			return err
		}
	}

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		loc = time.UTC
	}
	currentYear := time.Now().In(loc).Year()

	for y := 2009; y <= currentYear; y++ {
		if err := EnsureAnnualLimitExists(db, y); err != nil {
			return err
		}
	}

	return nil
}
