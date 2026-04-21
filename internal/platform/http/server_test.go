package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tockr/internal/db/sqlite"
	"tockr/internal/platform/config"
)

func TestLoginFlow(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{SessionSecret: "test-secret", DefaultTimezone: "UTC", DefaultCurrency: "USD", FutureTimePolicy: "allow", DataDir: t.TempDir(), WebhookMaxRetries: 1}
	if err := store.SeedAdmin(ctx, "admin@example.com", "admin12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	app := New(cfg, store, slog.Default())
	form := strings.NewReader("email=admin%40example.com&password=admin12345")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}
