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

	log.Println("✅ SQLite database initialized with User Profile fields")
	return db, nil
}

func createTables(db *sql.DB) error {
	schema := `
	-- 1. Users table (Email as Username)
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

	-- 2. Activation Email Tokens
	CREATE TABLE IF NOT EXISTS email_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- 3. Transactions table
	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('DEPOSIT', 'WITHDRAWAL')),
		amount REAL NOT NULL CHECK(amount > 0),
		date DATE NOT NULL,
		note TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- 4. CRA Annual Limits table
	CREATE TABLE IF NOT EXISTS tfsa_annual_limits (
		year INTEGER PRIMARY KEY,
		amount REAL NOT NULL
	);

	-- 5. Global system settings
	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	INSERT OR IGNORE INTO system_settings (key, value) VALUES ('registration_enabled', 'true');
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Dynamic column migrations for existing SQLite databases
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN start_year INTEGER DEFAULT 2009")
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN phone_number TEXT DEFAULT ''")

	return nil
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
	var previousAmount float64
	err = db.QueryRow("SELECT year, amount FROM tfsa_annual_limits WHERE year < ? ORDER BY year DESC LIMIT 1", targetYear).Scan(&previousYear, &previousAmount)

	if err == sql.ErrNoRows {
		previousAmount = 5000.0
	} else if err != nil {
		return fmt.Errorf("failed to retrieve historical limit before %d: %w", targetYear, err)
	}

	_, err = db.Exec("INSERT INTO tfsa_annual_limits (year, amount) VALUES (?, ?)", targetYear, previousAmount)
	if err != nil {
		return fmt.Errorf("failed to auto-insert limit for year %d: %w", targetYear, err)
	}

	log.Printf("⚠️ Auto-fallback triggered: TFSA limit for %d was missing. Populated using %d limit ($%.2f)", targetYear, previousYear, previousAmount)
	return nil
}

func seedAnnualLimits(db *sql.DB) error {
	limits := map[int]float64{
		2009: 5000, 2010: 5000, 2011: 5000, 2012: 5000,
		2013: 5500, 2014: 5500, 2015: 10000, 2016: 5500,
		2017: 5500, 2018: 5500, 2019: 6000, 2020: 6000,
		2021: 6000, 2022: 6000, 2023: 6500, 2024: 7000,
		2025: 7000, 2026: 7000,
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

	currentYear := time.Now().Year()
	for y := 2009; y <= currentYear; y++ {
		if err := EnsureAnnualLimitExists(db, y); err != nil {
			return err
		}
	}

	return nil
}
