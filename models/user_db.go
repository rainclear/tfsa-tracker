package models

import (
	"database/sql"
	"errors"
	"fmt"
	"net/mail"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidEmail       = errors.New("invalid email address format")
	ErrUserExists         = errors.New("email address is already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidPass        = errors.New("invalid email or password")
	ErrAccountPending     = errors.New("account is pending admin approval")
	ErrAccountActivation  = errors.New("account approved! Please check your email and click the confirmation link")
	ErrAccountRejected    = errors.New("account registration was rejected")
	ErrRegistrationClosed = errors.New("registration is currently closed by admin")
	ErrCannotModifyAdmin  = errors.New("forbidden: cannot modify or delete another admin")
)

// ValidateEmail checks if the username string is a valid email
func ValidateEmail(email string) error {
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}
	return nil
}

// CreateUser registers a new account
func CreateUser(db *sql.DB, email, password string) (*User, error) {
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}

	regOpen, err := IsRegistrationEnabled(db)
	if err != nil {
		return nil, err
	}

	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return nil, err
	}

	if userCount > 0 && !regOpen {
		return nil, ErrRegistrationClosed
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	role := RoleUser
	status := StatusPending

	// First user is automatically APPROVED ADMIN
	if userCount == 0 {
		role = RoleAdmin
		status = StatusApproved
	}

	query := `INSERT INTO users (email, password_hash, role, status) VALUES (?, ?, ?, ?)`
	res, err := db.Exec(query, email, string(hashedPassword), role, status)
	if err != nil {
		return nil, ErrUserExists
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:     id,
		Email:  email,
		Role:   role,
		Status: status,
	}, nil
}

// AuthenticateUser verifies email, password, and activation status
func AuthenticateUser(db *sql.DB, email, password string) (*User, error) {
	var user User
	var hash string

	query := `SELECT id, email, password_hash, role, status, created_at FROM users WHERE email = ?`
	err := db.QueryRow(query, email).Scan(&user.ID, &user.Email, &hash, &user.Role, &user.Status, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPass
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidPass
	}

	// Block login based on status
	if user.Status == StatusPending {
		return nil, ErrAccountPending
	}
	if user.Status == StatusPendingActivation {
		return nil, ErrAccountActivation
	}
	if user.Status == StatusRejected {
		return nil, ErrAccountRejected
	}

	return &user, nil
}

func GetUserByID(db *sql.DB, id int64) (*User, error) {
	var user User
	query := `SELECT id, email, role, status, created_at FROM users WHERE id = ?`
	err := db.QueryRow(query, id).Scan(&user.ID, &user.Email, &user.Role, &user.Status, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func ListAllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(`SELECT id, email, role, status, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func UpdateUserStatus(db *sql.DB, targetUserID int64, newStatus UserStatus) error {
	targetUser, err := GetUserByID(db, targetUserID)
	if err != nil {
		return err
	}

	if targetUser.Role == RoleAdmin {
		return ErrCannotModifyAdmin
	}

	_, err = db.Exec(`UPDATE users SET status = ? WHERE id = ?`, newStatus, targetUserID)
	return err
}

func DeleteUser(db *sql.DB, targetUserID int64) error {
	targetUser, err := GetUserByID(db, targetUserID)
	if err != nil {
		return err
	}

	if targetUser.Role == RoleAdmin {
		return ErrCannotModifyAdmin
	}

	_, err = db.Exec(`DELETE FROM users WHERE id = ?`, targetUserID)
	return err
}

func IsRegistrationEnabled(db *sql.DB) (bool, error) {
	var val string
	err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'registration_enabled'`).Scan(&val)
	if err != nil {
		return true, nil
	}
	return val == "true", nil
}

func SetRegistrationEnabled(db *sql.DB, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	_, err := db.Exec(`INSERT INTO system_settings (key, value) VALUES ('registration_enabled', ?) 
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, val)
	return err
}
