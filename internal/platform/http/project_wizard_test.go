package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"tockr/internal/domain"
)

func TestProjectCreateWizardStepGuardAndEntryPoint(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer := &domain.Customer{WorkspaceID: 1, Name: "Wizard Customer", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	projects := getWithCookie(app, "/projects", cookie)
	if projects.Code != http.StatusOK {
		t.Fatalf("GET /projects returned %d", projects.Code)
	}
	if !strings.Contains(projects.Body.String(), `href="/projects/create"`) {
		t.Fatal("projects page should link to the new create-project wizard")
	}

	rec := getWithCookie(app, "/projects/create?step=activities", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET guarded wizard step returned %d, want 303", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/projects/create?step=details" {
		t.Fatalf("guard redirect = %q, want /projects/create?step=details", location)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("guard redirect should not create a draft cookie before details are entered")
	}
}

func TestProjectCreateWizardCreatesProjectEndToEnd(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()

	customer := &domain.Customer{WorkspaceID: 1, Name: "End To End Customer", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	workstream := &domain.Workstream{WorkspaceID: 1, Name: "Discovery", Code: "DISC", Visible: true}
	if err := store.UpsertWorkstream(ctx, workstream); err != nil {
		t.Fatal(err)
	}
	globalActivity := &domain.Activity{WorkspaceID: 1, Name: "Research", Number: "RES", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, globalActivity); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(ctx, domain.User{Email: "wizard-user@example.com", Username: "wizard-user", DisplayName: "Wizard User", Timezone: "UTC", Enabled: true}, "wizard12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	assignedUser, err := store.FindUserByEmail(ctx, "wizard-user@example.com")
	if err != nil || assignedUser == nil {
		t.Fatal("failed to load wizard user")
	}

	sessionCookie := loginCookie(t, app, "admin@example.com", "admin12345")
	draftCookie := (*http.Cookie)(nil)
	csrf := csrfFromBody(t, getWithCookie(app, "/projects/create", sessionCookie).Body.String())

	rec := postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":                 {csrf},
		"step":                 {"details"},
		"action":               {"next"},
		"customer_id":          {strconv.FormatInt(customer.ID, 10)},
		"name":                 {"Wizard Launch"},
		"number":               {"WIZ-001"},
		"order_number":         {"PO-42"},
		"estimate_hours":       {"24"},
		"budget":               {"120000"},
		"budget_alert_percent": {"85"},
		"comment":              {"Created through the wizard"},
		"visible":              {"on"},
		"billable":             {"on"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("details submit returned %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/projects/create?step=workstreams" {
		t.Fatalf("details redirect = %q", location)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=workstreams", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":          {csrf},
		"step":          {"workstreams"},
		"action":        {"add-existing"},
		"workstream_id": {strconv.FormatInt(workstream.ID, 10)},
		"budget":        {"45000"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("workstream add returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=workstreams", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":   {csrf},
		"step":   {"workstreams"},
		"action": {"next"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("workstreams next returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=activities", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":        {csrf},
		"step":        {"activities"},
		"action":      {"add-existing"},
		"activity_id": {strconv.FormatInt(globalActivity.ID, 10)},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("activity add existing returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=activities", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":         {csrf},
		"step":         {"activities"},
		"action":       {"add-new"},
		"new_name":     {"Workshop"},
		"new_number":   {"WRK"},
		"new_comment":  {"Project-specific deliverable"},
		"new_visible":  {"on"},
		"new_billable": {"on"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("activity add new returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=activities", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":   {csrf},
		"step":   {"activities"},
		"action": {"next"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("activities next returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=users", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":    {csrf},
		"step":    {"users"},
		"action":  {"add-users"},
		"user_id": {strconv.FormatInt(assignedUser.ID, 10)},
		"role":    {"manager"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("users add returned %d", rec.Code)
	}
	draftCookie = projectWizardDraftCookie(t, rec)

	csrf = csrfFromBody(t, getWithCookies(app, "/projects/create?step=users", sessionCookie, draftCookie).Body.String())
	rec = postFormWithCookies(app, "/projects/create", url.Values{
		"csrf":   {csrf},
		"step":   {"users"},
		"action": {"submit"},
	}, sessionCookie, draftCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("wizard submit returned %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/projects/") || !strings.Contains(location, "/dashboard") {
		t.Fatalf("final redirect = %q, want project dashboard", location)
	}

	var projectID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM projects WHERE name='Wizard Launch'`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var projectNumber, orderNumber, comment string
	var estimateSeconds, budgetCents, budgetAlertPercent int
	if err := store.DB().QueryRowContext(ctx, `SELECT number, order_number, comment, estimate_seconds, budget_cents, budget_alert_percent FROM projects WHERE id=?`, projectID).Scan(&projectNumber, &orderNumber, &comment, &estimateSeconds, &budgetCents, &budgetAlertPercent); err != nil {
		t.Fatal(err)
	}
	if projectNumber != "WIZ-001" || orderNumber != "PO-42" || comment != "Created through the wizard" {
		t.Fatalf("unexpected project fields: number=%q order=%q comment=%q", projectNumber, orderNumber, comment)
	}
	if estimateSeconds != 24*3600 || budgetCents != 120000 || budgetAlertPercent != 85 {
		t.Fatalf("unexpected budget/estimate fields: seconds=%d budget=%d alert=%d", estimateSeconds, budgetCents, budgetAlertPercent)
	}

	var workstreamCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM project_workstreams WHERE project_id=?`, projectID).Scan(&workstreamCount); err != nil {
		t.Fatal(err)
	}
	if workstreamCount != 1 {
		t.Fatalf("project_workstreams count = %d, want 1", workstreamCount)
	}

	var projectActivityCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE project_id=?`, projectID).Scan(&projectActivityCount); err != nil {
		t.Fatal(err)
	}
	if projectActivityCount != 2 {
		t.Fatalf("project activities count = %d, want 2", projectActivityCount)
	}

	var copiedActivityCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE project_id=? AND name='Research'`, projectID).Scan(&copiedActivityCount); err != nil {
		t.Fatal(err)
	}
	if copiedActivityCount != 1 {
		t.Fatalf("copied global activity count = %d, want 1", copiedActivityCount)
	}

	var memberCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id=?`, projectID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if memberCount != 1 {
		t.Fatalf("project members count = %d, want 1", memberCount)
	}

	var assignedRole string
	if err := store.DB().QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id=? AND user_id=?`, projectID, assignedUser.ID).Scan(&assignedRole); err != nil {
		t.Fatal(err)
	}
	if assignedRole != string(domain.ProjectRoleManager) {
		t.Fatalf("assigned role = %q, want %q", assignedRole, domain.ProjectRoleManager)
	}

	if cleared := projectWizardDraftCookieOptional(rec); cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("final submit should clear the project draft cookie")
	}
}

func postFormWithCookies(app *Server, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		if cookie != nil {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func projectWizardDraftCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookie := projectWizardDraftCookieOptional(rec)
	if cookie == nil {
		t.Fatal("missing project wizard draft cookie")
	}
	return cookie
}

func projectWizardDraftCookieOptional(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == projectCreateDraftCookieName {
			return cookie
		}
	}
	return nil
}
