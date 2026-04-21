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
	"tockr/internal/domain"
	"tockr/internal/platform/config"
)

func TestLoginFlow(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
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

func TestAdminNavigationLinksLoadAndMarkActiveState(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	routes := []string{"/", "/timesheets", "/customers", "/projects", "/activities", "/tags", "/reports", "/invoices", "/rates", "/admin/users", "/webhooks"}
	for _, route := range routes {
		rec := getWithCookie(app, route, cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", route, rec.Code)
		}
	}

	body := getWithCookie(app, "/timesheets", cookie).Body.String()
	if !strings.Contains(body, `class="nav-link active" aria-current="page" href="/timesheets"`) {
		t.Fatal("expected timesheets nav item to be active")
	}

	body = getWithCookie(app, "/reports?group=customer", cookie).Body.String()
	if !strings.Contains(body, `class="tab-link active" aria-current="page" href="/reports?group=customer"`) {
		t.Fatal("expected customer report tab to be active")
	}
}

func TestUserNavigationHidesForbiddenItems(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	if err := store.CreateUser(context.Background(), domain.User{
		Email:       "user@example.com",
		Username:    "user",
		DisplayName: "User",
		Timezone:    "UTC",
		Enabled:     true,
	}, "user12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "user@example.com", "user12345")
	body := getWithCookie(app, "/", cookie).Body.String()
	for _, forbiddenLink := range []string{`href="/invoices"`, `href="/rates"`, `href="/admin/users"`, `href="/webhooks"`} {
		if strings.Contains(body, forbiddenLink) {
			t.Fatalf("normal user should not see %s", forbiddenLink)
		}
	}
	for _, visibleLink := range []string{`href="/"`, `href="/timesheets"`, `href="/customers"`, `href="/projects"`, `href="/activities"`, `href="/tags"`, `href="/reports"`} {
		if !strings.Contains(body, visibleLink) {
			t.Fatalf("normal user should see %s", visibleLink)
		}
	}

	customers := getWithCookie(app, "/customers", cookie)
	if customers.Code != http.StatusOK {
		t.Fatalf("customers returned %d", customers.Code)
	}
	if strings.Contains(customers.Body.String(), "Create Customer") {
		t.Fatal("normal user should not see customer create form")
	}

	for _, route := range []string{"/invoices", "/rates", "/admin/users", "/webhooks"} {
		rec := getWithCookie(app, route, cookie)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s returned %d, want 403", route, rec.Code)
		}
	}
}

func testApp(t *testing.T) (*Server, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SessionSecret: "test-secret", DefaultTimezone: "UTC", DefaultCurrency: "USD", FutureTimePolicy: "allow", DataDir: t.TempDir(), WebhookMaxRetries: 1}
	if err := store.SeedAdmin(ctx, "admin@example.com", "admin12345", "UTC", "USD"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return New(cfg, store, slog.Default()), store
}

func loginCookie(t *testing.T, app *Server, email, password string) *http.Cookie {
	t.Helper()
	form := strings.NewReader("email=" + strings.ReplaceAll(email, "@", "%40") + "&password=" + password)
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login returned %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not return a cookie")
	}
	return cookies[0]
}

func getWithCookie(app *Server, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}
