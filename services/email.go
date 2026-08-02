package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"time"

	"tfsa-tracker/config"
	"tfsa-tracker/models"
)

type EmailService struct {
	DB     *sql.DB
	Config *config.Config
}

func NewEmailService(db *sql.DB, cfg *config.Config) *EmailService {
	return &EmailService{DB: db, Config: cfg}
}

// GenerateActivationToken creates a 24-hour single-use token in SQLite
func (s *EmailService) GenerateActivationToken(userID int64) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err := s.DB.Exec(`INSERT INTO email_tokens (user_id, token, expires_at) VALUES (?, ?, ?)`, userID, token, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

// SendActivationEmail sends the confirmation link to the approved user
func (s *EmailService) SendActivationEmail(toEmail, token string) error {
	activationURL := fmt.Sprintf("%s/activate?token=%s", s.Config.AppBaseURL, token)

	subject := "Subject: Activate Your TFSA Tracker Account\n"
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	body := fmt.Sprintf(`
		<h2>Welcome to TFSA Tracker!</h2>
		<p>Your account registration has been approved by an administrator.</p>
		<p>Please click the link below to confirm your email and activate your account:</p>
		<p><a href="%s" style="padding: 10px 15px; background-color: #2563eb; color: white; text-decoration: none; border-radius: 5px;">Activate Account</a></p>
		<br/>
		<p>Or copy this URL into your browser: %s</p>
	`, activationURL, activationURL)

	msg := []byte(subject + mime + body)

	// If SMTP is not configured in environment, print to stdout for testing
	if s.Config.SMTPHost == "" {
		fmt.Printf("\n[DEV MODE] Activation Email for %s:\n%s\n\n", toEmail, activationURL)
		return nil
	}

	auth := smtp.PlainAuth("", s.Config.SMTPUser, s.Config.SMTPPass, s.Config.SMTPHost)
	addr := fmt.Sprintf("%s:%s", s.Config.SMTPHost, s.Config.SMTPPort)
	return smtp.SendMail(addr, auth, s.Config.SMTPFrom, []string{toEmail}, msg)
}

// VerifyActivationToken checks and consumes the activation token
func (s *EmailService) VerifyActivationToken(token string) (*models.User, error) {
	var userID int64
	var expiresAt time.Time

	err := s.DB.QueryRow(`SELECT user_id, expires_at FROM email_tokens WHERE token = ?`, token).Scan(&userID, &expiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired token")
	}

	if time.Now().After(expiresAt) {
		s.DB.Exec(`DELETE FROM email_tokens WHERE token = ?`, token)
		return nil, fmt.Errorf("activation token has expired")
	}

	// Update user status to APPROVED
	_, err = s.DB.Exec(`UPDATE users SET status = 'APPROVED' WHERE id = ?`, userID)
	if err != nil {
		return nil, err
	}

	// Delete used token
	s.DB.Exec(`DELETE FROM email_tokens WHERE token = ?`, token)

	return models.GetUserByID(s.DB, userID)
}
