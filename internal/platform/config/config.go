package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr               string
	PublicURL          string
	DatabasePath       string
	DataDir            string
	SessionSecret      string
	CookieSecure       bool
	DefaultTimezone    string
	DefaultCurrency    string
	FutureTimePolicy   string
	TOTPMode           string
	AdminEmail         string
	AdminPassword      string
	ShutdownTimeout    time.Duration
	WebhookMaxRetries  int
	RateLimitEnabled   bool
	SMTPHost           string
	SMTPPort           int
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
	SMTPStartTLS       bool
	SMTPGlobalFallback bool
}

func Load() Config {
	return Config{
		Addr:               getenv("TOCKR_ADDR", ":8029"),
		PublicURL:          getenv("TOCKR_PUBLIC_URL", ""),
		DatabasePath:       getenv("TOCKR_DB_PATH", "data/tockr.db"),
		DataDir:            getenv("TOCKR_DATA_DIR", "data"),
		SessionSecret:      getenv("TOCKR_SESSION_SECRET", randomSecret()),
		CookieSecure:       getenvBool("TOCKR_COOKIE_SECURE", false),
		DefaultTimezone:    getenv("TOCKR_DEFAULT_TIMEZONE", "UTC"),
		DefaultCurrency:    getenv("TOCKR_DEFAULT_CURRENCY", "USD"),
		FutureTimePolicy:   getenv("TOCKR_FUTURE_TIME_POLICY", "end_of_day"),
		TOTPMode:           getenv("TOCKR_TOTP_MODE", "optional"),
		AdminEmail:         getenv("TOCKR_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:      getenv("TOCKR_ADMIN_PASSWORD", "admin12345"),
		ShutdownTimeout:    10 * time.Second,
		WebhookMaxRetries:  getenvInt("TOCKR_WEBHOOK_MAX_RETRIES", 5),
		RateLimitEnabled:   getenvBool("TOCKR_RATE_LIMIT_ENABLED", true),
		SMTPHost:           getenv("TOCKR_SMTP_HOST", ""),
		SMTPPort:           getenvInt("TOCKR_SMTP_PORT", 587),
		SMTPUsername:       getenv("TOCKR_SMTP_USERNAME", ""),
		SMTPPassword:       getenv("TOCKR_SMTP_PASSWORD", ""),
		SMTPFrom:           getenv("TOCKR_SMTP_FROM", ""),
		SMTPStartTLS:       getenvBool("TOCKR_SMTP_STARTTLS", true),
		SMTPGlobalFallback: getenvBool("TOCKR_SMTP_GLOBAL_FALLBACK", false),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func randomSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("dev-secret-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
