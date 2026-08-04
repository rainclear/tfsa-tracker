package models

import (
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"time"

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

type UserRole string

const (
	RoleAdmin UserRole = "ADMIN"
	RoleUser  UserRole = "USER"
)

type UserStatus string

const (
	StatusPending           UserStatus = "PENDING"
	StatusPendingActivation UserStatus = "PENDING_ACTIVATION"
	StatusApproved          UserStatus = "APPROVED"
	StatusRejected          UserStatus = "REJECTED"
)

type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"` // Used as Username
	PasswordHash string     `json:"-"`
	Role         UserRole   `json:"role"`
	Status       UserStatus `json:"status"`
	StartYear    int        `json:"start_year"`
	PhoneNumber  string     `json:"phone_number"`
	CreatedAt    time.Time  `json:"created_at"`
}

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

	query := `INSERT INTO users (email, password_hash, role, status, start_year, phone_number) VALUES (?, ?, ?, ?, 2009, '')`
	res, err := db.Exec(query, email, string(hashedPassword), role, status)
	if err != nil {
		return nil, ErrUserExists
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:          id,
		Email:       email,
		Role:        role,
		Status:      status,
		StartYear:   2009,
		PhoneNumber: "",
	}, nil
}

// AuthenticateUser verifies email, password, and activation status
func AuthenticateUser(db *sql.DB, email, password string) (*User, error) {
	var user User
	var hash string

	query := `SELECT id, email, password_hash, role, status, start_year, phone_number, created_at FROM users WHERE email = ?`
	err := db.QueryRow(query, email).Scan(&user.ID, &user.Email, &hash, &user.Role, &user.Status, &user.StartYear, &user.PhoneNumber, &user.CreatedAt)
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
	query := `SELECT id, email, role, status, start_year, phone_number, created_at FROM users WHERE id = ?`
	err := db.QueryRow(query, id).Scan(&user.ID, &user.Email, &user.Role, &user.Status, &user.StartYear, &user.PhoneNumber, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserProfile updates start_year and phone_number for a user
func UpdateUserProfile(db *sql.DB, userID int64, startYear int, phoneNumber string) error {
	if startYear < 2009 {
		startYear = 2009
	}
	_, err := db.Exec(`UPDATE users SET start_year = ?, phone_number = ? WHERE id = ?`, startYear, phoneNumber, userID)
	return err
}

func GetAllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(`SELECT id, email, role, status, start_year, phone_number, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.StartYear, &u.PhoneNumber, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func IsRegistrationEnabled(db *sql.DB) (bool, error) {
	var val string
	err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'registration_enabled'`).Scan(&val)
	if err != nil {
		return true, nil
	}
	return val == "true", nil
}