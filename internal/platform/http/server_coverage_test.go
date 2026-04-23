package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"tockr/internal/domain"
)

// ─── Health ────────────────────────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz returned %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("healthz not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("healthz status = %q, want ok", body["status"])
	}
}

// ─── Auth / Session ─────────────────────────────────────────────────────────

func TestLogoutClearsSession(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	// Authenticated request succeeds before logout
	if rec := getWithCookie(app, "/", cookie); rec.Code != http.StatusOK {
		t.Fatalf("pre-logout GET / returned %d", rec.Code)
	}
	// Logout
	body := getWithCookie(app, "/", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader("csrf="+csrf))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout returned %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/login") {
		t.Fatalf("logout redirected to %q, want /login", loc)
	}
}

func TestUnauthenticatedAccessRedirectsToLogin(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	protectedRoutes := []string{
		"/", "/account", "/customers", "/projects", "/tasks", "/activities",
		"/tags", "/groups", "/rates", "/timesheets", "/calendar", "/reports",
		"/invoices", "/workstreams", "/admin", "/admin/users",
		"/admin/schedule", "/admin/exchange-rates", "/admin/recalculate",
		"/reports/utilization",
	}
	for _, route := range protectedRoutes {
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s unauthenticated returned %d, want 303 redirect", route, rec.Code)
		}
		if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/login") {
			t.Errorf("%s redirected to %q, want /login", route, loc)
		}
	}
}

func TestInvalidLoginCredentials(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cases := []struct {
		email    string
		password string
	}{
		{"admin@example.com", "wrongpassword"},
		{"notexist@example.com", "admin12345"},
		{"", ""},
	}
	for _, tc := range cases {
		form := strings.NewReader("email=" + tc.email + "&password=" + tc.password)
		req := httptest.NewRequest(http.MethodPost, "/login", form)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "Invalid") {
			t.Errorf("bad credentials (%q/%q) returned %d loc=%s", tc.email, tc.password, rec.Code, rec.Header().Get("Location"))
		}
	}
}

// ─── Account ──────────────────────────────────────────────────────────────────

func TestUpdateAccountProfile(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/account", cookie, url.Values{
		"csrf":         {csrf},
		"display_name": {"New Name"},
		"timezone":     {"Europe/London"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("account update returned %d", rec.Code)
	}
	accountBody := getWithCookie(app, "/account", cookie).Body.String()
	if !strings.Contains(accountBody, "New Name") {
		t.Fatal("account page should show updated display name")
	}
	if !strings.Contains(accountBody, `<option value="Europe/London" selected>`) {
		t.Fatal("account page should show updated timezone")
	}
}

func TestUpdateAccountPasswordSuccess(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/account/password", cookie, url.Values{
		"csrf":             {csrf},
		"current_password": {"admin12345"},
		"password":         {"newpass99"},
		"confirm":          {"newpass99"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("password update returned %d", rec.Code)
	}
	// Old password no longer works
	oldForm := strings.NewReader("email=admin%40example.com&password=admin12345")
	req := httptest.NewRequest(http.MethodPost, "/login", oldForm)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r2 := httptest.NewRecorder()
	app.Handler().ServeHTTP(r2, req)
	if !strings.Contains(r2.Header().Get("Location"), "Invalid") {
		t.Fatal("old password should not work after change")
	}
}

func TestUpdateAccountPasswordWrongCurrent(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/account/password", cookie, url.Values{
		"csrf":             {csrf},
		"current_password": {"wrongpassword"},
		"password":         {"newpass99"},
		"confirm":          {"newpass99"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "incorrect") {
		t.Fatalf("wrong current password returned %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestUpdateAccountPasswordMismatch(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/account/password", cookie, url.Values{
		"csrf":             {csrf},
		"current_password": {"admin12345"},
		"password":         {"newpass99"},
		"confirm":          {"different"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "confirmation") {
		t.Fatalf("password mismatch returned %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

// ─── Utilization ──────────────────────────────────────────────────────────────

func TestUtilizationReportLoads(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/reports/utilization", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("utilization returned %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Utilization") {
		t.Fatal("utilization page missing heading")
	}
}

func TestUtilizationReportWithDateRange(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	start := time.Now().UTC().Truncate(24 * time.Hour).Add(-2 * time.Hour)
	end := start.Add(1 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID,
		ProjectID: project.ID, ActivityID: activity.ID,
		StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true,
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	begin := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")
	endDate := time.Now().UTC().Format("2006-01-02")
	rec := getWithCookie(app, "/reports/utilization?begin="+begin+"&end="+endDate, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("utilization with date range returned %d", rec.Code)
	}
}

// ─── Work Schedule ─────────────────────────────────────────────────────────────

func TestWorkScheduleSettingsLoadAndSave(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	rec := getWithCookie(app, "/admin/schedule", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/schedule returned %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Work Schedule") {
		t.Fatal("schedule page missing heading")
	}
	csrf := csrfFromBody(t, body)

	rec = postFormWithCookie(app, "/admin/schedule", cookie, url.Values{
		"csrf":          {csrf},
		"hours_per_day": {"7"},
		"day_mon":       {"1"},
		"day_tue":       {"1"},
		"day_wed":       {"1"},
		"day_thu":       {"1"},
		"day_fri":       {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /admin/schedule returned %d: %s", rec.Code, rec.Body.String())
	}

	// Reload and confirm saved value
	body = getWithCookie(app, "/admin/schedule", cookie).Body.String()
	if !strings.Contains(body, "7") {
		t.Fatal("schedule page should reflect saved hours per day")
	}
}

func TestWorkScheduleSettingsForbiddenForRegularUser(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	if err := store.CreateUser(context.Background(), domain.User{
		Email: "reg@example.com", Username: "reg", DisplayName: "Reg", Timezone: "UTC", Enabled: true,
	}, "reg12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "reg@example.com", "reg12345")
	if rec := getWithCookie(app, "/admin/schedule", cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin/schedule for regular user = %d, want 403", rec.Code)
	}
}

// ─── Workstreams ────────────────────────────────────────────────────────────────

func TestWorkstreamCRUD(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	// GET list (empty)
	rec := getWithCookie(app, "/workstreams", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /workstreams returned %d", rec.Code)
	}
	body := rec.Body.String()
	csrf := csrfFromBody(t, body)

	// Create workstream
	rec = postFormWithCookie(app, "/workstreams", cookie, url.Values{
		"csrf":        {csrf},
		"name":        {"Alpha Stream"},
		"description": {"Primary stream"},
		"visible":     {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /workstreams create returned %d: %s", rec.Code, rec.Body.String())
	}

	// List now contains it
	body = getWithCookie(app, "/workstreams", cookie).Body.String()
	if !strings.Contains(body, "Alpha Stream") {
		t.Fatal("workstreams list should show created workstream")
	}

	// Get workstream ID
	var wsID int64
	if err := store.DB().QueryRowContext(context.Background(), `SELECT id FROM workstreams WHERE name='Alpha Stream'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}

	// Update workstream
	body = getWithCookie(app, "/workstreams", cookie).Body.String()
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/workstreams/"+strconv.FormatInt(wsID, 10), cookie, url.Values{
		"csrf":        {csrf},
		"name":        {"Alpha Stream Updated"},
		"code":        {"AS1"},
		"description": {"Updated description"},
		"visible":     {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /workstreams/{id} update returned %d: %s", rec.Code, rec.Body.String())
	}

	body = getWithCookie(app, "/workstreams", cookie).Body.String()
	if !strings.Contains(body, "Alpha Stream Updated") {
		t.Fatal("workstreams list should show updated workstream name")
	}

	// Delete workstream
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/workstreams/"+strconv.FormatInt(wsID, 10)+"/delete", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("DELETE workstream returned %d: %s", rec.Code, rec.Body.String())
	}

	body = getWithCookie(app, "/workstreams", cookie).Body.String()
	if strings.Contains(body, "Alpha Stream Updated") {
		t.Fatal("workstreams list should not show deleted workstream")
	}
}

func TestWorkstreamCreateRequiresName(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/workstreams", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/workstreams", cookie, url.Values{
		"csrf":    {csrf},
		"name":    {""},
		"visible": {"on"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("workstream create without name returned %d, want 400", rec.Code)
	}
}

func TestWorkstreamForbiddenForRegularUser(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	if err := store.CreateUser(context.Background(), domain.User{
		Email: "wsreg@example.com", Username: "wsreg", DisplayName: "WsReg", Timezone: "UTC", Enabled: true,
	}, "reg12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "wsreg@example.com", "reg12345")
	if rec := getWithCookie(app, "/workstreams", cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("GET /workstreams for regular user = %d, want 403", rec.Code)
	}
}

// ─── Project Workstreams ──────────────────────────────────────────────────────

func TestProjectWorkstreamAssignment(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	// Create a workstream first
	body := getWithCookie(app, "/workstreams", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	postFormWithCookie(app, "/workstreams", cookie, url.Values{
		"csrf":    {csrf},
		"name":    {"Dev Stream"},
		"visible": {"on"},
	})
	var wsID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM workstreams WHERE name='Dev Stream'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}

	// GET project workstreams
	pid := strconv.FormatInt(project.ID, 10)
	rec := getWithCookie(app, "/projects/"+pid+"/workstreams", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /projects/%s/workstreams returned %d", pid, rec.Code)
	}

	// Assign workstream to project
	body = rec.Body.String()
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/projects/"+pid+"/workstreams", cookie, url.Values{
		"csrf":         {csrf},
		"workstream_id": {strconv.FormatInt(wsID, 10)},
		"budget_cents": {"50000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /projects/%s/workstreams returned %d: %s", pid, rec.Code, rec.Body.String())
	}

	body = getWithCookie(app, "/projects/"+pid+"/workstreams", cookie).Body.String()
	if !strings.Contains(body, "Dev Stream") {
		t.Fatal("project workstreams should show assigned stream")
	}

	// Get the project_workstream ID for removal
	var pwID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM project_workstreams WHERE project_id=? AND workstream_id=?`, project.ID, wsID).Scan(&pwID); err != nil {
		t.Fatal(err)
	}

	// Remove workstream from project
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/projects/"+pid+"/workstreams/"+strconv.FormatInt(wsID, 10)+"/remove", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("remove project workstream returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectWorkstreamRequiresWorkstreamID(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	pid := strconv.FormatInt(project.ID, 10)
	body := getWithCookie(app, "/projects/"+pid+"/workstreams", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/projects/"+pid+"/workstreams", cookie, url.Values{
		"csrf":         {csrf},
		"workstream_id": {"0"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("project workstream without ID returned %d, want 400", rec.Code)
	}
}

// ─── CSV Exports ──────────────────────────────────────────────────────────────

func TestCSVReportExport(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := start.Add(1 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID,
		ProjectID: project.ID, ActivityID: activity.ID,
		StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true,
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/reports/export?group=user", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reports/export returned %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("reports export content-type = %q, want text/csv", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "report.csv") {
		t.Fatal("reports export missing attachment filename")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Name") || !strings.Contains(body, "TrackedSeconds") {
		t.Fatal("reports CSV missing headers")
	}
}

func TestCSVTimesheetExport(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := start.Add(1 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID,
		ProjectID: project.ID, ActivityID: activity.ID,
		StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true,
		Description: "Export test entry",
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/timesheets/export", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /timesheets/export returned %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("timesheets export content-type = %q, want text/csv", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "timesheets.csv") {
		t.Fatal("timesheets export missing attachment filename")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ID") || !strings.Contains(body, "Duration") {
		t.Fatal("timesheets CSV missing headers")
	}
	if !strings.Contains(body, "Export test entry") {
		t.Fatal("timesheets CSV missing entry description")
	}
}

// ─── Exchange Rates ───────────────────────────────────────────────────────────

func TestExchangeRateCRUD(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	rec := getWithCookie(app, "/admin/exchange-rates", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/exchange-rates returned %d", rec.Code)
	}
	body := rec.Body.String()
	csrf := csrfFromBody(t, body)

	// Create rate
	rec = postFormWithCookie(app, "/admin/exchange-rates", cookie, url.Values{
		"csrf":           {csrf},
		"from_currency":  {"USD"},
		"to_currency":    {"EUR"},
		"rate":           {"0.92"},
		"effective_from": {"2026-01-01"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create exchange rate returned %d: %s", rec.Code, rec.Body.String())
	}

	body = getWithCookie(app, "/admin/exchange-rates", cookie).Body.String()
	if !strings.Contains(body, "USD") || !strings.Contains(body, "EUR") {
		t.Fatal("exchange rates page missing created rate")
	}

	// Get ID
	var rateID int64
	if err := store.DB().QueryRowContext(context.Background(), `SELECT id FROM exchange_rates WHERE from_currency='USD' AND to_currency='EUR'`).Scan(&rateID); err != nil {
		t.Fatal(err)
	}

	// Delete rate
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/admin/exchange-rates/"+strconv.FormatInt(rateID, 10)+"/delete", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete exchange rate returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExchangeRateValidation(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/exchange-rates", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	badCases := []url.Values{
		// Short currency code
		{"csrf": {csrf}, "from_currency": {"US"}, "to_currency": {"EUR"}, "rate": {"0.92"}, "effective_from": {"2026-01-01"}},
		// Zero rate
		{"csrf": {csrf}, "from_currency": {"USD"}, "to_currency": {"EUR"}, "rate": {"0"}, "effective_from": {"2026-01-01"}},
		// Bad date
		{"csrf": {csrf}, "from_currency": {"USD"}, "to_currency": {"EUR"}, "rate": {"0.92"}, "effective_from": {"not-a-date"}},
	}
	for _, vals := range badCases {
		if rec := postFormWithCookie(app, "/admin/exchange-rates", cookie, vals); rec.Code != http.StatusBadRequest {
			t.Errorf("invalid exchange rate input returned %d, want 400 (input: %v)", rec.Code, vals)
		}
	}
}

// ─── Recalculate ──────────────────────────────────────────────────────────────

func TestRecalculatePageLoads(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/admin/recalculate", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/recalculate returned %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Recalculate") {
		t.Fatal("recalculate page missing heading")
	}
}

func TestRecalculateApplyRequiresProject(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/recalculate", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/admin/recalculate", cookie, url.Values{
		"csrf":       {csrf},
		"project_id": {"0"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("recalculate without project returned %d, want 400", rec.Code)
	}
}

func TestRecalculateApplyWithProject(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/recalculate", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/admin/recalculate", cookie, url.Values{
		"csrf":       {csrf},
		"project_id": {strconv.FormatInt(project.ID, 10)},
		"since":      {"2026-01-01"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("recalculate apply returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Location"), "project_id=") {
		t.Fatal("recalculate redirect should contain project_id")
	}
}

// ─── Saved Report Edit / Delete / Share ───────────────────────────────────────

func TestSavedReportEditAndDelete(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	if err := store.CreateSavedReport(ctx, &domain.SavedReport{
		WorkspaceID: 1, UserID: admin.ID, Name: "My Report",
		GroupBy:     "user",
		FiltersJSON: `{}`,
	}); err != nil {
		t.Fatal(err)
	}
	var reportID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM saved_reports WHERE name='My Report'`).Scan(&reportID); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/reports", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Edit
	rec := postFormWithCookie(app, "/reports/saved/"+strconv.FormatInt(reportID, 10), cookie, url.Values{
		"csrf":  {csrf},
		"name":  {"My Report Updated"},
		"group": {"user"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("edit saved report returned %d: %s", rec.Code, rec.Body.String())
	}
	body = getWithCookie(app, "/reports", cookie).Body.String()
	if !strings.Contains(body, "My Report Updated") {
		t.Fatal("reports page should show updated report name")
	}

	// Delete
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/reports/saved/"+strconv.FormatInt(reportID, 10)+"/delete", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete saved report returned %d: %s", rec.Code, rec.Body.String())
	}
	body = getWithCookie(app, "/reports", cookie).Body.String()
	if strings.Contains(body, "My Report Updated") {
		t.Fatal("reports page should not show deleted report")
	}
}

func TestSavedReportShareAndViewAndRevoke(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	if err := store.CreateSavedReport(ctx, &domain.SavedReport{
		WorkspaceID: 1, UserID: admin.ID, Name: "Shared Report",
		GroupBy: "user", FiltersJSON: `{"group":"user"}`,
	}); err != nil {
		t.Fatal(err)
	}
	var reportID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM saved_reports WHERE name='Shared Report'`).Scan(&reportID); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/reports", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Share
	rec := postFormWithCookie(app, "/reports/saved/"+strconv.FormatInt(reportID, 10)+"/share", cookie, url.Values{
		"csrf":   {csrf},
		"action": {"share"},
		"days":   {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("share saved report returned %d: %s", rec.Code, rec.Body.String())
	}

	// Get token from DB
	var token string
	if err := store.DB().QueryRowContext(ctx, `SELECT share_token FROM saved_reports WHERE id=?`, reportID).Scan(&token); err != nil || token == "" {
		t.Fatal("no share token created")
	}

	// View shared report (unauthenticated)
	rec2 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/reports/share/"+token, nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("view shared report returned %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "Shared Report") {
		t.Fatal("shared report view should show report name")
	}

	// Expired token returns 404 or 410
	body = getWithCookie(app, "/reports", cookie).Body.String()
	csrf = csrfFromBody(t, body)
	rec3 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/reports/share/nonexistenttoken12345", nil))
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("invalid share token returned %d, want 404", rec3.Code)
	}

	// Revoke
	csrf = csrfFromBody(t, body)
	rec4 := postFormWithCookie(app, "/reports/saved/"+strconv.FormatInt(reportID, 10)+"/share", cookie, url.Values{
		"csrf":   {csrf},
		"action": {"revoke"},
	})
	if rec4.Code != http.StatusSeeOther {
		t.Fatalf("revoke share returned %d: %s", rec4.Code, rec4.Body.String())
	}
	// Token no longer works
	rec5 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec5, httptest.NewRequest(http.MethodGet, "/reports/share/"+token, nil))
	if rec5.Code == http.StatusOK {
		t.Fatal("revoked token should not give access")
	}
}

// ─── Timer Start / Stop ──────────────────────────────────────────────────────

func TestTimerStartAndStop(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Start timer
	rec := postFormWithCookie(app, "/timesheets/start", cookie, url.Values{
		"csrf":        {csrf},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"project_id":  {strconv.FormatInt(project.ID, 10)},
		"activity_id": {strconv.FormatInt(activity.ID, 10)},
		"description": {"Timer test"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start timer returned %d: %s", rec.Code, rec.Body.String())
	}

	// Dashboard shows running timer
	dashboard := getWithCookie(app, "/", cookie).Body.String()
	if !strings.Contains(dashboard, "Stop") {
		t.Fatal("dashboard should show stop timer button when timer is running")
	}

	// Stop timer
	csrf = csrfFromBody(t, getWithCookie(app, "/", cookie).Body.String())
	rec = postFormWithCookie(app, "/timesheets/stop", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("stop timer returned %d: %s", rec.Code, rec.Body.String())
	}

	// Dashboard no longer shows stop button
	dashboard = getWithCookie(app, "/", cookie).Body.String()
	if strings.Contains(dashboard, "Stop") {
		t.Fatal("dashboard should not show stop button after timer stopped")
	}
}

// ─── API Endpoints ────────────────────────────────────────────────────────────

func TestAPIStatusEndpoint(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/status returned %d", rec.Code)
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("/api/status not valid JSON: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("/api/status returned status=%q", result["status"])
	}
	if result["app"] != "tockr" {
		t.Fatalf("/api/status returned app=%q", result["app"])
	}
}

func TestAPIListEndpoints(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	_ = customer
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	apiRoutes := []string{
		"/api/customers",
		"/api/projects",
		"/api/activities",
		"/api/tasks",
		"/api/timesheets",
	}
	for _, route := range apiRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", route, rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s content-type = %q, want application/json", route, ct)
		}
		var result map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
			t.Fatalf("%s not valid JSON: %v", route, err)
		}
		if _, ok := result["data"]; !ok {
			t.Fatalf("%s JSON missing 'data' field", route)
		}
	}

	// Verify data is populated
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	var projects map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&projects)
	data, _ := projects["data"].([]any)
	if len(data) == 0 {
		t.Fatal("/api/projects should return the seeded project")
	}
	_ = project
	_ = activity
	_ = task
}

func TestAPITimerStartStop(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	// Get CSRF from a page
	body := getWithCookie(app, "/", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	req := httptest.NewRequest(http.MethodPost, "/api/timer/start", strings.NewReader(url.Values{
		"csrf":        {csrf},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"project_id":  {strconv.FormatInt(project.ID, 10)},
		"activity_id": {strconv.FormatInt(activity.ID, 10)},
		"description": {"API timer test"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/timer/start returned %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("/api/timer/start not valid JSON: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("/api/timer/start result = %v", result)
	}

	// Stop timer via API
	csrf = csrfFromBody(t, getWithCookie(app, "/", cookie).Body.String())
	req2 := httptest.NewRequest(http.MethodPost, "/api/timer/stop", strings.NewReader("csrf="+csrf))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST /api/timer/stop returned %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestAPIWebhooksList(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	req := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/webhooks returned %d", rec.Code)
	}
}

func TestAPIInvoiceMetaUpdate(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	start := time.Now().UTC().Add(-48 * time.Hour)
	end := start.Add(4 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID,
		ProjectID: project.ID, ActivityID: activity.ID,
		StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true,
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	// Set a billing rate first
	if err := store.UpsertRate(ctx, &domain.Rate{
		WorkspaceID:   1,
		AmountCents:   10000,
		Kind:          "hourly",
		EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/invoices", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Create invoice
	beginDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")
	rec := postFormWithCookie(app, "/invoices", cookie, url.Values{
		"csrf":        {csrf},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"begin":       {beginDate},
		"end":         {endDate},
		"tax":         {"0"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create invoice returned %d: %s", rec.Code, rec.Body.String())
	}

	var invoiceID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM invoices ORDER BY id DESC LIMIT 1`).Scan(&invoiceID); err != nil {
		t.Fatal(err)
	}

	// Update invoice meta via API
	patchBody := `{"name":"paid","value":"2026-04-30"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/invoices/"+strconv.FormatInt(invoiceID, 10)+"/meta", strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfFromBody(t, getWithCookie(app, "/invoices", cookie).Body.String()))
	req.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("PATCH /api/invoices/meta returned %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestAPIInvoiceDownload(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, _ := seedSelectorFixtures(t, store)
	admin, _ := store.FindUserByEmail(ctx, "admin@example.com")
	start := time.Now().UTC().Add(-72 * time.Hour)
	end := start.Add(2 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID,
		ProjectID: project.ID, ActivityID: activity.ID,
		StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true,
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/invoices", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/invoices", cookie, url.Values{
		"csrf":        {csrf},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"begin":       {start.Format("2006-01-02")},
		"end":         {end.Format("2006-01-02")},
		"tax":         {"0"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create invoice returned %d: %s", rec.Code, rec.Body.String())
	}
	var invoiceID int64
	var filename string
	if err := store.DB().QueryRowContext(ctx, `SELECT id, filename FROM invoices ORDER BY id DESC LIMIT 1`).Scan(&invoiceID, &filename); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/invoices/"+strconv.FormatInt(invoiceID, 10)+"/download", nil)
	req.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /api/invoices/{id}/download returned %d", rec2.Code)
	}
	if !strings.Contains(rec2.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("invoice download missing Content-Disposition attachment")
	}
}

// ─── Project Dashboard ────────────────────────────────────────────────────────

func TestProjectDashboardRendersCorrectly(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/dashboard", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /projects/{id}/dashboard returned %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, project.Name) {
		t.Fatalf("project dashboard missing project name %q", project.Name)
	}
}

func TestProjectDashboardNotFoundForInvalidID(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	rec := getWithCookie(app, "/projects/9999999/dashboard", cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("project dashboard for invalid ID returned %d, want 404", rec.Code)
	}
}

// ─── Task Archive ─────────────────────────────────────────────────────────────

func TestTaskArchive(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, _, _, task := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/tasks", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/tasks/"+strconv.FormatInt(task.ID, 10)+"/archive", cookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("archive task returned %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── Customer / Project / Activity Create ────────────────────────────────────

func TestCreateCustomer(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/customers", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/customers", cookie, url.Values{
		"csrf":     {csrf},
		"name":     {"New Customer"},
		"currency": {"USD"},
		"timezone": {"UTC"},
		"visible":  {"on"},
		"billable": {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create customer returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(getWithCookie(app, "/customers", cookie).Body.String(), "New Customer") {
		t.Fatal("customers page should show created customer")
	}
}

func TestCreateProjectWithAutoNumber(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer := &domain.Customer{WorkspaceID: 1, Name: "Create Customer", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(context.Background(), customer); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/projects", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/projects", cookie, url.Values{
		"csrf":        {csrf},
		"name":        {"Create Test Project"},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"number":      {""},
		"visible":     {"on"},
		"billable":    {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create project returned %d: %s", rec.Code, rec.Body.String())
	}
	// Auto-ID should start with "PR-"
	var number string
	if err := store.DB().QueryRowContext(context.Background(), `SELECT number FROM projects WHERE name='Create Test Project'`).Scan(&number); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(number, "PR-") {
		t.Fatalf("project number = %q, want PR- prefix", number)
	}
}

func TestCreateActivity(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/activities", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/activities", cookie, url.Values{
		"csrf":     {csrf},
		"name":     {"New Activity"},
		"visible":  {"on"},
		"billable": {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create activity returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(getWithCookie(app, "/activities", cookie).Body.String(), "New Activity") {
		t.Fatal("activities page should show created activity")
	}
}

func TestCreateTaskWithAutoID(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/tasks", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/tasks", cookie, url.Values{
		"csrf":       {csrf},
		"name":       {"Auto ID Task"},
		"project_id": {strconv.FormatInt(project.ID, 10)},
		"number":     {""},
		"visible":    {"on"},
		"billable":   {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create task returned %d: %s", rec.Code, rec.Body.String())
	}
	var number string
	if err := store.DB().QueryRowContext(context.Background(), `SELECT number FROM tasks WHERE name='Auto ID Task'`).Scan(&number); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(number, "TSK-") {
		t.Fatalf("task number = %q, want TSK- prefix", number)
	}
}

// ─── Project Group Assignment ─────────────────────────────────────────────────

func TestProjectGroupAssignment(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	_, project, _, _ := seedSelectorFixtures(t, store)
	groupID, err := store.CreateGroup(ctx, 1, "Dev Group", "")
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	pid := strconv.FormatInt(project.ID, 10)

	body := getWithCookie(app, "/projects/"+pid+"/members", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Add group
	rec := postFormWithCookie(app, "/projects/"+pid+"/groups", cookie, url.Values{
		"csrf":     {csrf},
		"group_id": {strconv.FormatInt(groupID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add project group returned %d: %s", rec.Code, rec.Body.String())
	}
	groups, err := store.ListProjectGroups(ctx, project.ID)
	if err != nil || len(groups) == 0 {
		t.Fatal("group should be assigned to project")
	}

	// Remove group
	body = getWithCookie(app, "/projects/"+pid+"/members", cookie).Body.String()
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/projects/"+pid+"/groups/remove", cookie, url.Values{
		"csrf":     {csrf},
		"group_id": {strconv.FormatInt(groupID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("remove project group returned %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── Rate Creation ────────────────────────────────────────────────────────────

func TestCreateRate(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/rates", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/rates", cookie, url.Values{
		"csrf":           {csrf},
		"amount_cents":   {"15000"},
		"effective_from": {"2026-01-01"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create rate returned %d: %s", rec.Code, rec.Body.String())
	}
	body = getWithCookie(app, "/rates", cookie).Body.String()
	if !strings.Contains(body, "150.00") {
		t.Fatal("rates page should show created rate")
	}
}

// ─── Tag creation ─────────────────────────────────────────────────────────────

func TestCreateTag(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/tags", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/tags", cookie, url.Values{
		"csrf": {csrf},
		"name": {"mytag"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create tag returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(getWithCookie(app, "/tags", cookie).Body.String(), "mytag") {
		t.Fatal("tags page should show created tag")
	}
}

// ─── Webhook creation ─────────────────────────────────────────────────────────

func TestCreateWebhook(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/webhooks", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/webhooks", cookie, url.Values{
		"csrf":   {csrf},
		"name":   {"My Webhook"},
		"url":    {"https://example.com/hook"},
		"secret": {"mysecret"},
		"events": {"timesheet.created,invoice.created"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create webhook returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(getWithCookie(app, "/webhooks", cookie).Body.String(), "My Webhook") {
		t.Fatal("webhooks page should show created webhook")
	}
}

// ─── User creation ────────────────────────────────────────────────────────────

func TestCreateUser(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/users", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	rec := postFormWithCookie(app, "/admin/users", cookie, url.Values{
		"csrf":         {csrf},
		"email":        {"newuser@example.com"},
		"username":     {"newuser"},
		"display_name": {"New User"},
		"password":     {"newuser12345"},
		"role":         {"user"},
		"timezone":     {"UTC"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create user returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(getWithCookie(app, "/admin/users", cookie).Body.String(), "newuser@example.com") {
		t.Fatal("users page should show created user")
	}
}

// ─── Edge Cases ────────────────────────────────────────────────────────────────

func TestCSRFRejectedOnMutatingRequests(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	// POST without CSRF token
	req := httptest.NewRequest(http.MethodPost, "/customers", strings.NewReader("name=NoCSRF&currency=USD&timezone=UTC&visible=on"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF returned %d, want 403", rec.Code)
	}
}

func TestSwitchWorkspaceForbiddenForUnauthorizedWorkspace(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	// Create a regular user (non-admin)
	if err := store.CreateUser(ctx, domain.User{
		Email: "regmember@example.com", Username: "regmember", DisplayName: "RegMember", Timezone: "UTC", Enabled: true,
	}, "reg12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	// Create a second workspace and only add admin to it (not regmember)
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO workspaces(organization_id, name, slug, default_currency, timezone, created_at) VALUES(1,'Other WS','other-ws','USD','UTC',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	var wsID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM workspaces WHERE slug='other-ws'`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	// Login as regular user (not an org admin, so can't access all workspaces)
	memberCookie := loginCookie(t, app, "regmember@example.com", "reg12345")
	body := getWithCookie(app, "/account", memberCookie).Body.String()
	csrf := csrfFromBody(t, body)

	// Regular user tries to switch to a workspace they're not a member of
	rec := postFormWithCookie(app, "/workspace", memberCookie, url.Values{
		"csrf":         {csrf},
		"workspace_id": {strconv.FormatInt(wsID, 10)},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("switch to unauthorized workspace returned %d, want 403", rec.Code)
	}
}

// ─── Recalculate Preview ─────────────────────────────────────────────────────

func TestRecalculatePreviewWithProjectFilter(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	url := "/admin/recalculate?project_id=" + strconv.FormatInt(project.ID, 10) + "&since=2026-01-01"
	rec := getWithCookie(app, url, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/recalculate with filter returned %d", rec.Code)
	}
}
