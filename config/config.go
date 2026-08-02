package config

import "os"

type Config struct {
	Port          string
	DBPath        string
	SessionSecret string
	AppBaseURL    string
	SMTPHost      string
	SMTPPort      string
	SMTPUser      string
	SMTPPass      string
	SMTPFrom      string
}

func LoadConfig() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DBPath:        getEnv("DB_PATH", "./tfsa.db"),
		SessionSecret: getEnv("SESSION_SECRET", "default-secret-key-change-in-prod"),
		AppBaseURL:    getEnv("APP_BASE_URL", "http://localhost:8080"),
		SMTPHost:      getEnv("SMTP_HOST", ""),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPUser:      getEnv("SMTP_USER", ""),
		SMTPPass:      getEnv("SMTP_PASS", ""),
		SMTPFrom:      getEnv("SMTP_FROM", "no-reply@tfsa-tracker.local"),
	}
}

func getEnv(key, defaultValue string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultValue
}
