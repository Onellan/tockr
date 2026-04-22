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
	Addr              string
	DatabasePath      string
	DataDir           string
	SessionSecret     string
	CookieSecure      bool
	DefaultTimezone   string
	DefaultCurrency   string
	FutureTimePolicy  string
	TOTPMode          string
	AdminEmail        string
	AdminPassword     string
	ShutdownTimeout   time.Duration
	WebhookMaxRetries int
}

func Load() Config {
	return Config{
		Addr:              getenv("TOCKR_ADDR", ":8080"),
		DatabasePath:      getenv("TOCKR_DB_PATH", "data/tockr.db"),
		DataDir:           getenv("TOCKR_DATA_DIR", "data"),
		SessionSecret:     getenv("TOCKR_SESSION_SECRET", randomSecret()),
		CookieSecure:      getenvBool("TOCKR_COOKIE_SECURE", false),
		DefaultTimezone:   getenv("TOCKR_DEFAULT_TIMEZONE", "UTC"),
		DefaultCurrency:   getenv("TOCKR_DEFAULT_CURRENCY", "USD"),
		FutureTimePolicy:  getenv("TOCKR_FUTURE_TIME_POLICY", "end_of_day"),
		TOTPMode:          getenv("TOCKR_TOTP_MODE", "disabled"),
		AdminEmail:        getenv("TOCKR_ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:     getenv("TOCKR_ADMIN_PASSWORD", "admin12345"),
		ShutdownTimeout:   10 * time.Second,
		WebhookMaxRetries: getenvInt("TOCKR_WEBHOOK_MAX_RETRIES", 5),
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
