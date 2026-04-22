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

	routes := []string{"/", "/timesheets", "/customers", "/projects", "/tasks", "/activities", "/tags", "/groups", "/reports", "/invoices", "/rates", "/admin/users", "/webhooks", "/api/tasks"}
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
	for _, visibleLink := range []string{`href="/"`, `href="/timesheets"`, `href="/customers"`, `href="/projects"`, `href="/tasks"`, `href="/activities"`, `href="/tags"`, `href="/reports"`} {
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

	for _, route := range []string{"/invoices", "/rates", "/admin/users", "/webhooks", "/groups"} {
		rec := getWithCookie(app, route, cookie)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s returned %d, want 403", route, rec.Code)
		}
	}
}

func TestScopedAuthorizationHidesPrivateProjectsUntilMembership(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.CreateUser(ctx, domain.User{
		Email:       "member@example.com",
		Username:    "member",
		DisplayName: "Member",
		Timezone:    "UTC",
		Enabled:     true,
	}, "member12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	customer := &domain.Customer{WorkspaceID: 1, Name: "Scoped", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Private Project", Visible: true, Private: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "member@example.com", "member12345")
	body := getWithCookie(app, "/projects", cookie).Body.String()
	if strings.Contains(body, "Private Project") {
		t.Fatal("private project leaked to unassigned member")
	}
	body = getWithCookie(app, "/customers", cookie).Body.String()
	if strings.Contains(body, "Scoped") {
		t.Fatal("customer for private-only project leaked to unassigned member")
	}
	user, err := store.FindUserByEmail(ctx, "member@example.com")
	if err != nil || user == nil {
		t.Fatal("expected member user")
	}
	if err := store.AddProjectMember(ctx, project.ID, user.ID, domain.ProjectRoleMember); err != nil {
		t.Fatal(err)
	}
	body = getWithCookie(app, "/projects", cookie).Body.String()
	if !strings.Contains(body, "Private Project") {
		t.Fatal("assigned member should see private project")
	}
	body = getWithCookie(app, "/customers", cookie).Body.String()
	if !strings.Contains(body, "Scoped") {
		t.Fatal("assigned member should see customer for assigned private project")
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
