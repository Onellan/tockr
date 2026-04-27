package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tockr/internal/domain"
	"tockr/internal/platform/config"
)

func TestSecurityHeadersPresent(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	headers := rec.Result().Header
	for name, expected := range map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := headers.Get(name); got != expected {
			t.Fatalf("%s = %q, want %q", name, got, expected)
		}
	}
	if csp := headers.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("missing strict CSP, got %q", csp)
	}
	if cacheControl := headers.Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
}

func TestCSRFRequiredForStateChangingRoute(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	req := httptest.NewRequest(http.MethodPost, "/account", strings.NewReader("display_name=NoToken"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /account without CSRF returned %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLoginRateLimit(t *testing.T) {
	app, store := testAppWithConfig(t, config.Config{RateLimitEnabled: true})
	defer store.Close()

	for i := 0; i < 8; i++ {
		rec := postPublicForm(app, "/login", url.Values{
			"email":    {"admin@example.com"},
			"password": {"bad-password"},
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("attempt %d returned %d, want %d", i+1, rec.Code, http.StatusSeeOther)
		}
	}

	rec := postPublicForm(app, "/login", url.Values{
		"email":    {"admin@example.com"},
		"password": {"bad-password"},
	})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("ninth attempt returned %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRegularUserCannotAccessAdminUsers(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()

	if err := store.CreateUser(context.Background(), domain.User{
		Email: "regular@example.com", Username: "regular", DisplayName: "Regular User", Timezone: "UTC", Enabled: true,
	}, "regular123", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "regular@example.com", "regular123")
	rec := getWithCookie(app, "/admin/users", cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin/users returned %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLoginRejectsSQLInjectionStyleInput(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()

	rec := postPublicForm(app, "/login", url.Values{
		"email":    {"' OR 1=1 --"},
		"password": {"anything"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SQLi-style login returned %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("SQLi-style login redirected to %q, want /login", loc)
	}
}
