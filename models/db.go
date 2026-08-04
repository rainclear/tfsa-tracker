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

	if err := migrateREALToCents(db); err != nil {
		log.Printf("Migration warning: %v", err)
	}

	if err := seedAnnualLimits(db); err != nil {
		return nil, fmt.Errorf("failed to seed annual limits: %w", err)
	}

	log.Println("✅ SQLite database initialized with CENTS Integer Money Representation")
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

	-- Stores amount as INTEGER cents
	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('DEPOSIT', 'WITHDRAWAL')),
		amount INTEGER NOT NULL CHECK(amount > 0),
		date DATE NOT NULL,
		note TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	-- Stores limit as INTEGER cents
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

// Automatically migrates old float/REAL values in SQLite to integer cents
func migrateREALToCents(db *sql.DB) error {
	// Check if transactions table contains REAL data (by checking typeof value)
	var txType string
	err := db.QueryRow(`SELECT typeof(amount) FROM transactions LIMIT 1`).Scan(&txType)
	if err == nil && txType == "real" {
		log.Println("🔄 Migrating transactions.amount from REAL to INTEGER (cents)...")
		_, err := db.Exec(`
			CREATE TABLE transactions_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				type TEXT NOT NULL CHECK(type IN ('DEPOSIT', 'WITHDRAWAL')),
				amount INTEGER NOT NULL CHECK(amount > 0),
				date DATE NOT NULL,
				note TEXT DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			INSERT INTO transactions_new (id, user_id, type, amount, date, note, created_at)
			SELECT id, user_id, type, CAST(ROUND(amount * 100) AS INTEGER), date, note, created_at FROM transactions;
			DROP TABLE transactions;
			ALTER TABLE transactions_new RENAME TO transactions;
		`)
		if err != nil {
			return fmt.Errorf("failed to migrate transactions: %w", err)
		}
	}

	var limitType string
	err = db.QueryRow(`SELECT typeof(amount) FROM tfsa_annual_limits LIMIT 1`).Scan(&limitType)
	if err == nil && limitType == "real" {
		log.Println("🔄 Migrating tfsa_annual_limits.amount from REAL to INTEGER (cents)...")
		_, err := db.Exec(`
			CREATE TABLE tfsa_annual_limits_new (
				year INTEGER PRIMARY KEY,
				amount INTEGER NOT NULL
			);
			INSERT INTO tfsa_annual_limits_new (year, amount)
			SELECT year, CAST(ROUND(amount * 100) AS INTEGER) FROM tfsa_annual_limits;
			DROP TABLE tfsa_annual_limits;
			ALTER TABLE tfsa_annual_limits_new RENAME TO tfsa_annual_limits;
		`)
		if err != nil {
			return fmt.Errorf("failed to migrate tfsa_annual_limits: %w", err)
		}
	}

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

	log.Printf("⚠️ Auto-fallback triggered: TFSA limit for %d missing. Populated using %d limit ($%.2f)", targetYear, previousYear, float64(previousAmountCents)/100.0)
	return nil
}

func seedAnnualLimits(db *sql.DB) error {
	// Base historical records in CENTS
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

	currentYear := time.Now().Year()
	for y := 2009; y <= currentYear; y++ {
		if err := EnsureAnnualLimitExists(db, y); err != nil {
			return err
		}
	}

	return nil
}
