package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tockr/internal/auth"
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

	routes := []string{"/", "/account", "/calendar", "/timesheets", "/customers", "/projects", "/project-dashboards", "/tasks", "/activities", "/workstreams", "/tags", "/groups", "/reports", "/reports/utilization", "/invoices", "/rates", "/project-templates", "/admin", "/admin/users", "/admin/email", "/admin/demo-data", "/admin/schedule", "/admin/exchange-rates", "/admin/recalculate", "/webhooks", "/api/tasks"}
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
	projectBody := getWithCookie(app, "/project-dashboards", cookie).Body.String()
	if !strings.Contains(projectBody, `href="/project-dashboards"`) {
		t.Fatal("expected project dashboards link in primary navigation")
	}

	body = getWithCookie(app, "/reports?group=customer", cookie).Body.String()
	if !strings.Contains(body, `class="tab-link active" aria-current="page" href="/reports?group=customer"`) {
		t.Fatal("expected customer report tab to be active")
	}

	adminBody := getWithCookie(app, "/admin/users", cookie).Body.String()
	for _, expected := range []string{
		`aria-label="Admin navigation"`,
		`class="nav-link active" aria-current="page" href="/admin/users"`,
		`href="/admin/workspaces"`,
		`href="/admin/email"`,
		`href="/admin/demo-data"`,
		`href="/admin"`,
		`href="/">Back to dashboard</a>`,
	} {
		if !strings.Contains(adminBody, expected) {
			t.Fatalf("admin page missing %q", expected)
		}
	}
	for _, unexpected := range []string{
		`aria-label="Primary navigation"`,
		`href="/timesheets" class="nav-link active"`,
	} {
		if strings.Contains(adminBody, unexpected) {
			t.Fatalf("admin page should not contain %q", unexpected)
		}
	}
}

func TestAdminDemoDataCanShowAndHideSeededWorkspaceData(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	adminCookie := loginCookie(t, app, "admin@example.com", "admin12345")

	body := getWithCookie(app, "/admin/demo-data", adminCookie).Body.String()
	for _, expected := range []string{"Default workspace target", "Show demo data", "Hide demo data"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("demo data admin page missing %q", expected)
		}
	}

	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/admin/demo-data/add", adminCookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("demo data add returned %d", rec.Code)
	}

	var demoUsers, demoCustomers, demoTags int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE lower(email) LIKE '%.demo@tockr.local'`).Scan(&demoUsers); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM customers WHERE lower(name) IN (lower('Northwind Mining'), lower('GreenLine Foods'), lower('MetroGrid Estates'))`).Scan(&demoCustomers); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE lower(name)=lower('demo-seed')`).Scan(&demoTags); err != nil {
		t.Fatal(err)
	}
	if demoUsers < 4 || demoCustomers < 3 || demoTags == 0 {
		t.Fatalf("expected demo data to be seeded, got users=%d customers=%d tags=%d", demoUsers, demoCustomers, demoTags)
	}

	body = getWithCookie(app, "/admin/demo-data", adminCookie).Body.String()
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/admin/demo-data/remove", adminCookie, url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("demo data remove returned %d", rec.Code)
	}

	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE lower(email) LIKE '%.demo@tockr.local'`).Scan(&demoUsers); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM customers WHERE lower(name) IN (lower('Northwind Mining'), lower('GreenLine Foods'), lower('MetroGrid Estates'))`).Scan(&demoCustomers); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE lower(name)=lower('demo-seed')`).Scan(&demoTags); err != nil {
		t.Fatal(err)
	}
	if demoUsers != 0 || demoCustomers != 0 || demoTags != 0 {
		t.Fatalf("expected demo data to be removed, got users=%d customers=%d tags=%d", demoUsers, demoCustomers, demoTags)
	}
}

func TestUIOutlierPagesUseSharedPatternsAndNoDeadActions(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	_, _, _, _ = seedSelectorFixtures(t, store)
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO workstreams(workspace_id, name, code, description, visible, created_at) VALUES(1,'Civil Delivery','CIV','Long description that should wrap rather than break a table cell',1,?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	workstreams := getWithCookie(app, "/workstreams", cookie).Body.String()
	if strings.Contains(workstreams, `href="/workstreams/`) {
		t.Fatal("workstreams page should not render dead edit links")
	}
	for _, expected := range []string{
		`class="inline-edit"`,
		`class="compact-form inline-edit-form" method="post" action="/workstreams/`,
		`class="actions-cell"`,
	} {
		if !strings.Contains(workstreams, expected) {
			t.Fatalf("workstreams page missing %q", expected)
		}
	}

	for route, expected := range map[string][]string{
		"/tasks":                {`class="panel form-panel"`, `class="table-card"`, `class="inline-edit"`, `name="project_id" value="`},
		"/reports/utilization":  {`class="panel form-panel"`, `class="toolbar-form"`, `class="empty-state"`},
		"/admin/exchange-rates": {`class="page-head"`, `class="panel form-panel"`, `class="empty-state"`},
		"/admin/recalculate":    {`class="page-head"`, `class="panel form-panel"`, `class="toolbar-form selector-form"`},
	} {
		rec := getWithCookie(app, route, cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", route, rec.Code)
		}
		body := rec.Body.String()
		for _, snippet := range expected {
			if !strings.Contains(body, snippet) {
				t.Fatalf("%s missing shared UI snippet %q", route, snippet)
			}
		}
		for _, stale := range []string{`class="filter-bar"`, `class="content-shell"`, `class="checkbox-label"`} {
			if strings.Contains(body, stale) {
				t.Fatalf("%s still contains stale UI class %q", route, stale)
			}
		}
	}
}

func TestResponsiveNavigationMarkupAndAssets(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/", cookie).Body.String()
	for _, expected := range []string{
		`class="mobile-nav-backdrop" data-mobile-nav-close hidden`,
		`class="app-shell" data-app-shell`,
		`<aside class="sidebar" id="app-sidebar"`,
		`class="mobile-menu-toggle" type="button" data-mobile-nav-toggle aria-controls="app-sidebar" aria-expanded="false"`,
		`class="mobile-nav-close" type="button" data-mobile-nav-close`,
		`class="topbar-title"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("responsive layout markup missing %q", expected)
		}
	}

	css := getPublic(app, "/static/style.css").Body.String()
	for _, expected := range []string{
		`@media (max-width: 920px) and (hover: none), (max-width: 920px) and (pointer: coarse)`,
		`.page-projects .content`,
		`.app-shell.nav-open .sidebar`,
		`transform: translateX(-105%)`,
		`body.nav-open`,
		`.section-spacer {` + "\n" + `  margin-top: var(--space-5);`,
		`.actions-cell`,
		`.inline-edit-form`,
		`.action-menu-button`,
		`.util-bar-wrap`,
		`.table-card {` + "\n" + `  overflow: visible;`,
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("responsive CSS missing %q", expected)
		}
	}

	js := getPublic(app, "/static/menu.js").Body.String()
	for _, expected := range []string{
		`function setupMobileNav()`,
		`[data-mobile-nav-toggle]`,
		`optionValue.split(",")`,
		`nav-open`,
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("mobile nav JS missing %q", expected)
		}
	}
}

func TestProjectRowOverflowAndMembershipPage(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer := &domain.Customer{WorkspaceID: 1, Name: "Overflow", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Action Project", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	projects := getWithCookie(app, "/projects", cookie).Body.String()
	for _, expected := range []string{
		`href="/projects/` + strconv.FormatInt(project.ID, 10) + `/dashboard"`,
		`href="/projects/` + strconv.FormatInt(project.ID, 10) + `/edit"`,
		`data-dropdown="project-` + strconv.FormatInt(project.ID, 10) + `-actions"`,
		`href="/projects/` + strconv.FormatInt(project.ID, 10) + `/members"`,
		`href="/projects/` + strconv.FormatInt(project.ID, 10) + `/workstreams"`,
	} {
		if !strings.Contains(projects, expected) {
			t.Fatalf("projects page missing %q", expected)
		}
	}

	members := getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/members", cookie)
	if members.Code != http.StatusOK {
		t.Fatalf("members page returned %d", members.Code)
	}
	if !strings.Contains(members.Body.String(), "Project access") {
		t.Fatal("members page did not render project access UI")
	}

	projectWorkstreams := getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/workstreams", cookie)
	if projectWorkstreams.Code != http.StatusOK {
		t.Fatalf("project workstreams page returned %d", projectWorkstreams.Code)
	}
	if !strings.Contains(projectWorkstreams.Body.String(), `href="/projects/`+strconv.FormatInt(project.ID, 10)+`/dashboard"`) {
		t.Fatal("project workstreams page should include back button to project dashboard when no workstreams are assigned")
	}

	edit := getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/edit", cookie)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit project page returned %d", edit.Code)
	}
	if !strings.Contains(edit.Body.String(), `href="/projects/`+strconv.FormatInt(project.ID, 10)+`/dashboard"`) {
		t.Fatal("edit project page should include back button to project dashboard")
	}

	dashboard := getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/dashboard", cookie)
	if dashboard.Code != http.StatusOK {
		t.Fatalf("project dashboard page returned %d", dashboard.Code)
	}
	if !strings.Contains(dashboard.Body.String(), `href="/projects"`) {
		t.Fatal("project dashboard should include back button to projects page")
	}
}

func TestWorkspaceSwitcherChangesSessionScope(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	_, err = store.DB().ExecContext(ctx, `INSERT INTO workspaces(organization_id, name, slug, default_currency, timezone, created_at) VALUES(1,'Alt Workspace','alt','USD','UTC',?)`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	var workspaceID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM workspaces WHERE slug='alt'`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at) VALUES(?,?,?,?)`, workspaceID, admin.ID, "admin", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	if !strings.Contains(body, `class="workspace-switcher"`) || !strings.Contains(body, "Alt Workspace") {
		t.Fatal("workspace switcher should render on the account page when user has multiple workspaces")
	}
	csrf := csrfFromBody(t, body)
	form := strings.NewReader("csrf=" + csrf + "&workspace_id=" + strconv.FormatInt(workspaceID, 10))
	req := httptest.NewRequest(http.MethodPost, "/workspace", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("workspace switch returned %d", rec.Code)
	}
	if body := getWithCookie(app, "/account", cookie).Body.String(); !strings.Contains(body, `<option value="`+strconv.FormatInt(workspaceID, 10)+`" selected>Alt Workspace</option>`) {
		t.Fatal("selected workspace was not persisted on the session")
	}
	if body := getWithCookie(app, "/", cookie).Body.String(); strings.Contains(body, `class="workspace-switcher"`) || strings.Contains(body, "Alt Workspace</option>") {
		t.Fatal("workspace switcher should not render in the shared topbar")
	}
}

func TestTOTPOptionalLoginRequiresCodeForEnabledUser(t *testing.T) {
	app, store := testAppWithConfig(t, config.Config{TOTPMode: "optional"})
	defer store.Close()
	ctx := context.Background()
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatal("missing admin")
	}
	secret := auth.NewTOTPSecret()
	if err := store.EnableTOTP(ctx, user.ID, secret, []string{"backup-code"}); err != nil {
		t.Fatal(err)
	}
	form := strings.NewReader("email=admin%40example.com&password=admin12345")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("login without totp should redirect to code error, code=%d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	if body := getPublic(app, "/login", rec.Result().Cookies()...).Body.String(); !strings.Contains(body, "Two-factor code required") {
		t.Fatal("login page should show two-factor flash message")
	}
	code, ok := auth.CurrentTOTPCode(secret, time.Now().UTC())
	if !ok {
		t.Fatal("could not generate totp code")
	}
	form = strings.NewReader("email=admin%40example.com&password=admin12345&totp=" + code)
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login with totp failed code=%d", rec.Code)
	}
}

func TestDropdownsAndFaviconHeadRender(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	loginBody := login.Body.String()
	for _, expected := range []string{
		`rel="icon" href="/favicon.ico?v=20260423-brand"`,
		`rel="icon" type="image/png" sizes="32x32" href="/static/favicon-32x32.png?v=20260423-brand"`,
		`rel="apple-touch-icon" sizes="180x180" href="/static/apple-touch-icon.png?v=20260423-brand"`,
		`rel="manifest" href="/static/site.webmanifest?v=20260423-brand"`,
		`<meta name="theme-color" content="#2F80ED">`,
		`/static/style.css?v=20260423-brand`,
		`<script src="/static/menu.js?v=20260422-navfix" defer>`,
	} {
		if strings.Count(loginBody, expected) != 1 {
			t.Fatalf("expected one %q in login head", expected)
		}
	}
	if strings.Contains(loginBody, "htmx-lite.js") {
		t.Fatal("login head should not include unused htmx-lite asset")
	}

	body := getWithCookie(app, "/", cookie).Body.String()
	for _, expected := range []string{
		`data-dropdown="account"`,
		`data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="account-menu"`,
		`id="account-menu" role="menu" hidden data-dropdown-menu`,
		`role="menuitem" href="/timesheets"`,
		`role="menuitem" href="/admin"`,
		`role="menuitem" type="submit">Logout</button>`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("dashboard missing dropdown markup %q", expected)
		}
	}

	accountBody := getWithCookie(app, "/account", cookie).Body.String()
	for _, expected := range []string{
		`<select name="timezone">`,
		`<option value="UTC" selected>UTC</option>`,
		`<option value="Africa/Johannesburg">Africa/Johannesburg</option>`,
	} {
		if !strings.Contains(accountBody, expected) {
			t.Fatalf("account page missing timezone selector markup %q", expected)
		}
	}

	adminBody := getWithCookie(app, "/admin", cookie).Body.String()
	for _, expected := range []string{
		`<h1>Admin</h1>`,
		`href="/admin/workspaces"`,
		`href="/rates"`,
		`href="/admin/users"`,
	} {
		if !strings.Contains(adminBody, expected) {
			t.Fatalf("admin hub missing %q", expected)
		}
	}
}

func TestSavedReportsDropdownRendersMenuLinks(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatal("missing admin")
	}
	if err := store.CreateSavedReport(ctx, &domain.SavedReport{
		WorkspaceID: 1,
		UserID:      user.ID,
		Name:        "Task focus",
		GroupBy:     "task",
		FiltersJSON: `{"task_id":"7","project_id":"2"}`,
	}); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/reports?group=task", cookie).Body.String()
	for _, expected := range []string{
		`data-dropdown="saved-reports"`,
		`aria-controls="saved-reports-menu"`,
		`href="/reports?group=task&amp;project_id=2&amp;task_id=7"`,
		`Task focus`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("reports missing saved-report dropdown markup %q", expected)
		}
	}
}

func TestIDFieldsRenderAsHumanReadableSelectors(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, project, _, task := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	dashboard := getWithCookie(app, "/", cookie).Body.String()
	for _, expected := range []string{
		`<select name="customer_id" required>`,
		`<select name="project_id" required data-filter-attr="customer-id" data-filter-parent="customer_id">`,
		`<select name="activity_id" required data-filter-attr="project-id" data-filter-parent="project_id">`,
		`<select name="task_id" data-filter-attr="project-id" data-filter-parent="project_id">`,
		`Alpha Customer`,
		`Buildout`,
		`Implementation`,
		`Launch task`,
		`data-customer-id="` + strconv.FormatInt(customer.ID, 10) + `"`,
		`data-project-id="` + strconv.FormatInt(project.ID, 10) + `"`,
	} {
		if !strings.Contains(dashboard, expected) {
			t.Fatalf("dashboard selector markup missing %q", expected)
		}
	}

	rates := getWithCookie(app, "/rates", cookie).Body.String()
	if !strings.Contains(rates, `<select name="user_id">`) || !strings.Contains(rates, `Administrator - admin@example.com`) {
		t.Fatal("rates page should render human-readable user selector")
	}
	if !strings.Contains(getWithCookie(app, "/timesheets", cookie).Body.String(), task.Name) {
		t.Fatal("timesheets should render task options by name")
	}
}

func TestSelectorOptionsRespectProjectScope(t *testing.T) {
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
	customer := &domain.Customer{WorkspaceID: 1, Name: "Scoped Customer", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	privateProject := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Private Selector Project", Visible: true, Private: true, Billable: true}
	if err := store.UpsertProject(ctx, privateProject); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "member@example.com", "member12345")
	body := getWithCookie(app, "/", cookie).Body.String()
	if strings.Contains(body, "Private Selector Project") {
		t.Fatal("private project leaked into selector before membership")
	}
	user, err := store.FindUserByEmail(ctx, "member@example.com")
	if err != nil || user == nil {
		t.Fatal("missing member")
	}
	if err := store.AddProjectMember(ctx, privateProject.ID, user.ID, domain.ProjectRoleMember); err != nil {
		t.Fatal(err)
	}
	body = getWithCookie(app, "/", cookie).Body.String()
	if !strings.Contains(body, "Private Selector Project") {
		t.Fatal("assigned private project should appear in selector")
	}
}

func TestSelectorFormSubmissionAndRelationshipValidation(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	otherTask := &domain.Task{WorkspaceID: 1, ProjectID: project.ID, Name: "Other project task", Visible: true, Billable: true}
	otherProject := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Other Project", Visible: true, Billable: true}
	if err := store.UpsertProject(context.Background(), otherProject); err != nil {
		t.Fatal(err)
	}
	otherTask.ProjectID = otherProject.ID
	if err := store.UpsertTask(context.Background(), otherTask); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/timesheets", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	form := strings.NewReader(
		"csrf=" + csrf +
			"&customer_id=" + strconv.FormatInt(customer.ID, 10) +
			"&project_id=" + strconv.FormatInt(project.ID, 10) +
			"&activity_id=" + strconv.FormatInt(activity.ID, 10) +
			"&task_id=" + strconv.FormatInt(task.ID, 10) +
			"&start=2026-04-22T09%3A00&end=2026-04-22T10%3A00&break_minutes=0&description=Selector+entry",
	)
	req := httptest.NewRequest(http.MethodPost, "/timesheets", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("valid selector submission returned %d", rec.Code)
	}
	if body := getWithCookie(app, "/timesheets", cookie).Body.String(); !strings.Contains(body, "Launch task") || strings.Contains(body, strconv.FormatInt(task.ID, 10)+`</td>`) {
		t.Fatal("timesheet row should show task label instead of raw task ID")
	}

	form = strings.NewReader(
		"csrf=" + csrf +
			"&customer_id=" + strconv.FormatInt(customer.ID, 10) +
			"&project_id=" + strconv.FormatInt(project.ID, 10) +
			"&activity_id=" + strconv.FormatInt(activity.ID, 10) +
			"&task_id=" + strconv.FormatInt(otherTask.ID, 10) +
			"&start=2026-04-22T11%3A00&end=2026-04-22T12%3A00&break_minutes=0",
	)
	req = httptest.NewRequest(http.MethodPost, "/timesheets", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched task submission returned %d, want 400", rec.Code)
	}
}

func TestFaviconAssetsAreServed(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	cases := []struct {
		path        string
		contentType string
	}{
		{"/favicon.ico", "image/x-icon"},
		{"/favicon-16x16.png", "image/png"},
		{"/favicon-32x32.png", "image/png"},
		{"/apple-touch-icon.png", "image/png"},
		{"/static/icon-192.png", "image/png"},
		{"/static/menu.js", "text/javascript"},
		{"/site.webmanifest", "application/manifest+json"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", tc.path, rec.Code)
		}
		if !strings.HasPrefix(rec.Header().Get("Content-Type"), tc.contentType) {
			t.Fatalf("%s content-type %q, want prefix %q", tc.path, rec.Header().Get("Content-Type"), tc.contentType)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s returned empty body", tc.path)
		}
		if !strings.Contains(rec.Header().Get("Cache-Control"), "max-age") {
			t.Fatalf("%s missing cache header", tc.path)
		}
	}

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/site.webmanifest", nil))
	var manifest map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest is not valid json: %v", err)
	}
	if manifest["short_name"] != "Tockr" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if manifest["background_color"] != "#F6FAFF" {
		t.Fatalf("unexpected manifest background color: %#v", manifest["background_color"])
	}
	if manifest["theme_color"] != "#2F80ED" {
		t.Fatalf("unexpected manifest theme color: %#v", manifest["theme_color"])
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
	if strings.Contains(body, `href="/admin"`) {
		t.Fatal("normal user should not see admin hub link")
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

	for _, route := range []string{"/admin", "/invoices", "/rates", "/admin/users", "/webhooks", "/groups"} {
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

func TestWorkspaceAdminCreateAndManageMembers(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.CreateUser(ctx, domain.User{
		Email:       "member-admin@example.com",
		Username:    "member-admin",
		DisplayName: "Member Admin",
		Timezone:    "UTC",
		Enabled:     true,
	}, "member12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	member, err := store.FindUserByEmail(ctx, "member-admin@example.com")
	if err != nil || member == nil {
		t.Fatal("missing member")
	}

	adminCookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/workspaces", adminCookie).Body.String()
	if !strings.Contains(body, "Create workspace") {
		t.Fatal("workspace admin screen did not render create form")
	}
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/admin/workspaces", adminCookie, url.Values{
		"csrf":             {csrf},
		"name":             {"Client Ops"},
		"slug":             {"client-ops"},
		"default_currency": {"USD"},
		"timezone":         {"UTC"},
		"description":      {"Client operations workspace"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("workspace create returned %d", rec.Code)
	}
	var workspaceID int64
	if err := store.DB().QueryRowContext(ctx, `SELECT id FROM workspaces WHERE slug='client-ops'`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	body = getWithCookie(app, "/admin/workspaces/"+strconv.FormatInt(workspaceID, 10), adminCookie).Body.String()
	if !strings.Contains(body, "Add or update member") || !strings.Contains(body, "Client operations workspace") {
		t.Fatal("workspace detail did not render settings and member UI")
	}
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/admin/workspaces/"+strconv.FormatInt(workspaceID, 10)+"/members", adminCookie, url.Values{
		"csrf":    {csrf},
		"user_id": {strconv.FormatInt(member.ID, 10)},
		"role":    {"analyst"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("workspace member save returned %d", rec.Code)
	}
	var role string
	if err := store.DB().QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?`, workspaceID, member.ID).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "analyst" {
		t.Fatalf("workspace role = %q, want analyst", role)
	}

	memberCookie := loginCookie(t, app, "member-admin@example.com", "member12345")
	rec = getWithCookie(app, "/admin/workspaces", memberCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("normal member workspace admin access = %d, want 403", rec.Code)
	}
}

func TestBulkGroupAndProjectMembershipEditors(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	for _, user := range []domain.User{
		{Email: "bulk-one@example.com", Username: "bulk-one", DisplayName: "Bulk One", Timezone: "UTC", Enabled: true},
		{Email: "bulk-two@example.com", Username: "bulk-two", DisplayName: "Bulk Two", Timezone: "UTC", Enabled: true},
	} {
		if err := store.CreateUser(ctx, user, "member12345", []domain.Role{domain.RoleUser}); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := store.FindUserByEmail(ctx, "bulk-one@example.com")
	second, _ := store.FindUserByEmail(ctx, "bulk-two@example.com")
	groupID, err := store.CreateGroup(ctx, 1, "Bulk Delivery", "")
	if err != nil {
		t.Fatal(err)
	}
	_, project, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	body := getWithCookie(app, "/groups/"+strconv.FormatInt(groupID, 10)+"/members", cookie).Body.String()
	if !strings.Contains(body, "Use groups for team-level project assignment") {
		t.Fatal("group bulk editor did not render")
	}
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/groups/"+strconv.FormatInt(groupID, 10)+"/members", cookie, url.Values{
		"csrf":    {csrf},
		"user_id": {strconv.FormatInt(first.ID, 10), strconv.FormatInt(second.ID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("bulk group add returned %d", rec.Code)
	}
	groupMembers, err := store.ListGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groupMembers) != 2 {
		t.Fatalf("group member count = %d, want 2", len(groupMembers))
	}

	body = getWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/members", cookie).Body.String()
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/members", cookie, url.Values{
		"csrf":    {csrf},
		"user_id": {strconv.FormatInt(first.ID, 10), strconv.FormatInt(second.ID, 10)},
		"role":    {"manager"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("bulk project member add returned %d", rec.Code)
	}
	members, err := store.ListProjectMembers(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("project member count = %d, want 2", len(members))
	}
	rec = postFormWithCookie(app, "/projects/"+strconv.FormatInt(project.ID, 10)+"/groups", cookie, url.Values{
		"csrf":     {csrf},
		"group_id": {strconv.FormatInt(groupID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("project group add returned %d", rec.Code)
	}
	groups, err := store.ListProjectGroups(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("project group count = %d, want 1", len(groups))
	}
}

func TestEngineeringWorkflowSurfacesRenderRecentWorkAndBillingContext(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	project.EstimateSeconds = 16 * 3600
	project.BudgetCents = 150000
	project.BudgetAlertPercent = 50
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	start := time.Now().UTC().Add(-6 * time.Hour)
	end := start.Add(2 * time.Hour)
	entry := &domain.Timesheet{
		WorkspaceID: 1,
		UserID:      admin.ID,
		CustomerID:  customer.ID,
		ProjectID:   project.ID,
		ActivityID:  activity.ID,
		TaskID:      &task.ID,
		StartedAt:   start,
		EndedAt:     &end,
		Timezone:    "UTC",
		Billable:    true,
		Description: "Pump sizing review",
	}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	favorite := &domain.Favorite{
		WorkspaceID: 1,
		UserID:      admin.ID,
		Name:        "Repeat pump sizing",
		CustomerID:  customer.ID,
		ProjectID:   project.ID,
		ActivityID:  activity.ID,
		TaskID:      &task.ID,
		Description: "Pump sizing review",
	}
	if err := store.CreateFavorite(ctx, favorite); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	dashboard := getWithCookie(app, "/", cookie).Body.String()
	for _, expected := range []string{
		"Project operations",
		"Continue recent work",
		"Project watchlist",
		"Pump sizing review",
	} {
		if !strings.Contains(dashboard, expected) {
			t.Fatalf("dashboard missing %q", expected)
		}
	}

	timesheets := getWithCookie(app, "/timesheets?customer_id="+strconv.FormatInt(customer.ID, 10)+"&project_id="+strconv.FormatInt(project.ID, 10)+"&activity_id="+strconv.FormatInt(activity.ID, 10)+"&task_id="+strconv.FormatInt(task.ID, 10)+"&date=2026-04-23&description=Pump+sizing+review", cookie).Body.String()
	for _, expected := range []string{
		`<h2>Log project time</h2>`,
		`name="customer_id"`,
		`name="activity_id"`,
		`Pump sizing review`,
		`Use again`,
		`Use favorite`,
		`Billable`,
		`Exported`,
	} {
		if !strings.Contains(timesheets, expected) {
			t.Fatalf("timesheets missing %q", expected)
		}
	}

	calendar := getWithCookie(app, "/calendar", cookie).Body.String()
	for _, expected := range []string{
		"Weekly review",
		"Buildout",
		"Implementation",
		"Launch task",
	} {
		if !strings.Contains(calendar, expected) {
			t.Fatalf("calendar missing %q", expected)
		}
	}
	if weekday := time.Now().UTC().Weekday(); weekday != time.Saturday && weekday != time.Sunday {
		if !strings.Contains(calendar, "Add more time") {
			t.Fatal("calendar missing weekday follow-up action")
		}
	}

	reports := getWithCookie(app, "/reports?group=activity&billable=true", cookie).Body.String()
	for _, expected := range []string{
		"Billing class",
		"Billable only",
		"Deliverable",
	} {
		if !strings.Contains(reports, expected) {
			t.Fatalf("reports missing %q", expected)
		}
	}
}

func TestTimesheetEntryModesRenderDefaultsAndDashboardQuickLog(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	dashboard := getWithCookie(app, "/", cookie).Body.String()
	for _, expected := range []string{
		`action="/timesheets"`,
		`name="entry_mode" value="manual"`,
		`name="date"`,
		`name="hours"`,
		`name="minutes"`,
		`Quick log`,
		`formaction="/timesheets/start"`,
	} {
		if !strings.Contains(dashboard, expected) {
			t.Fatalf("dashboard quick log should render manual capture field %q", expected)
		}
	}

	body := getWithCookie(app, "/timesheets", cookie).Body.String()
	for _, expected := range []string{
		`name="entry_mode" value="manual" checked`,
		`data-entry-mode-panel="manual"`,
		`name="date"`,
		`name="hours"`,
		`name="minutes"`,
		`data-entry-mode-panel="range" hidden`,
		`name="start"`,
		`name="end"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("timesheet manual default form missing %q", expected)
		}
	}

	rangeBody := getWithCookie(app, "/timesheets?entry_mode=range&date=2026-04-22", cookie).Body.String()
	for _, expected := range []string{
		`name="entry_mode" value="range" checked`,
		`data-entry-mode-panel="manual" hidden`,
		`data-entry-mode-panel="range"`,
		`value="2026-04-22T08:00"`,
		`value="2026-04-22T17:00"`,
	} {
		if !strings.Contains(rangeBody, expected) {
			t.Fatalf("timesheet range form missing %q", expected)
		}
	}
}

func TestTimesheetEntryModesValidateAndSave(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/timesheets", cookie).Body.String()
	csrf := csrfFromBody(t, body)

	base := url.Values{
		"csrf":        {csrf},
		"customer_id": {strconv.FormatInt(customer.ID, 10)},
		"project_id":  {strconv.FormatInt(project.ID, 10)},
		"activity_id": {strconv.FormatInt(activity.ID, 10)},
		"task_id":     {strconv.FormatInt(task.ID, 10)},
	}
	pastDate := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

	invalidManual := cloneValues(base)
	invalidManual.Set("entry_mode", "manual")
	invalidManual.Set("date", pastDate)
	invalidManual.Set("hours", "0")
	invalidManual.Set("minutes", "0")
	rec := postFormWithCookie(app, "/timesheets", cookie, invalidManual)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "duration must be greater than zero") {
		t.Fatalf("manual zero duration should render validation error, got %d", rec.Code)
	}

	invalidManual.Set("minutes", "60")
	rec = postFormWithCookie(app, "/timesheets", cookie, invalidManual)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "minutes must be between 0 and 59") {
		t.Fatalf("manual invalid minutes should render validation error, got %d", rec.Code)
	}

	manual := cloneValues(base)
	manual.Set("entry_mode", "manual")
	manual.Set("date", pastDate)
	manual.Set("hours", "1")
	manual.Set("minutes", "30")
	manual.Set("description", "Manual duration save")
	rec = postFormWithCookie(app, "/timesheets", cookie, manual)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("manual timesheet create returned %d", rec.Code)
	}
	manualEntry := mustFindTimesheetByDescription(t, store, ctx, "Manual duration save")
	if manualEntry.DurationSeconds != 90*60 {
		t.Fatalf("manual duration = %d, want 5400", manualEntry.DurationSeconds)
	}
	if manualEntry.StartedAt.Local().Format("2006-01-02") != pastDate {
		t.Fatalf("manual started date = %s, want %s", manualEntry.StartedAt.Local().Format("2006-01-02"), pastDate)
	}

	invalidRange := cloneValues(base)
	invalidRange.Set("entry_mode", "range")
	invalidRange.Set("start", pastDate+"T12:00")
	invalidRange.Set("end", pastDate+"T11:00")
	invalidRange.Set("break_minutes", "0")
	rec = postFormWithCookie(app, "/timesheets", cookie, invalidRange)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "end must be after start") {
		t.Fatalf("range invalid order should render validation error, got %d", rec.Code)
	}

	rangeForm := cloneValues(base)
	rangeForm.Set("entry_mode", "range")
	rangeForm.Set("start", pastDate+"T23:00")
	rangeForm.Set("end", time.Now().AddDate(0, 0, -1).Format("2006-01-02")+"T01:15")
	rangeForm.Set("break_minutes", "15")
	rangeForm.Set("description", "Range duration save")
	rec = postFormWithCookie(app, "/timesheets", cookie, rangeForm)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("range timesheet create returned %d", rec.Code)
	}
	rangeEntry := mustFindTimesheetByDescription(t, store, ctx, "Range duration save")
	if rangeEntry.DurationSeconds != 2*3600 {
		t.Fatalf("range duration = %d, want 7200", rangeEntry.DurationSeconds)
	}
	if getWithCookie(app, "/timesheets", cookie).Code != http.StatusOK {
		t.Fatal("timesheets should render after saving both entry modes")
	}
}

func TestTimesheetEditFlowUpdatesDashboardListCalendarReportsAndCreateStillWorks(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	updatedActivity := &domain.Activity{WorkspaceID: 1, ProjectID: &project.ID, Name: "QA Review", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, updatedActivity); err != nil {
		t.Fatal(err)
	}
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Minute)
	end := start.Add(90 * time.Minute)
	entry := &domain.Timesheet{
		WorkspaceID: 1,
		UserID:      admin.ID,
		CustomerID:  customer.ID,
		ProjectID:   project.ID,
		ActivityID:  activity.ID,
		TaskID:      &task.ID,
		StartedAt:   start,
		EndedAt:     &end,
		Timezone:    "UTC",
		Billable:    true,
		Description: "Original field visit",
	}
	if err := store.CreateTimesheet(ctx, entry, []string{"legacy"}); err != nil {
		t.Fatal(err)
	}

	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	editHref := "/timesheets/" + strconv.FormatInt(entry.ID, 10) + "/edit"

	dashboard := getWithCookie(app, "/", cookie).Body.String()
	if !strings.Contains(dashboard, editHref) {
		t.Fatal("dashboard should expose edit action for recent timesheet work")
	}

	timesheets := getWithCookie(app, "/timesheets", cookie).Body.String()
	if !strings.Contains(timesheets, editHref) {
		t.Fatal("timesheets list should expose edit action")
	}

	editPage := getWithCookie(app, editHref, cookie)
	if editPage.Code != http.StatusOK {
		t.Fatalf("edit page returned %d", editPage.Code)
	}
	editBody := editPage.Body.String()
	for _, expected := range []string{
		`action="/timesheets/` + strconv.FormatInt(entry.ID, 10) + `"`,
		`Edit timesheet entry`,
		`Original field visit`,
		`value="legacy"`,
		`name="billable" value="1" checked`,
	} {
		if !strings.Contains(editBody, expected) {
			t.Fatalf("edit page missing %q", expected)
		}
	}
	csrf := csrfFromBody(t, editBody)
	updatedStart := start.Add(30 * time.Minute)
	updatedEnd := updatedStart.Add(2 * time.Hour)
	rec := postFormWithCookie(app, "/timesheets/"+strconv.FormatInt(entry.ID, 10), cookie, url.Values{
		"csrf":          {csrf},
		"customer_id":   {strconv.FormatInt(customer.ID, 10)},
		"project_id":    {strconv.FormatInt(project.ID, 10)},
		"activity_id":   {strconv.FormatInt(updatedActivity.ID, 10)},
		"task_id":       {strconv.FormatInt(task.ID, 10)},
		"start":         {updatedStart.Local().Format("2006-01-02T15:04")},
		"end":           {updatedEnd.Local().Format("2006-01-02T15:04")},
		"break_minutes": {"15"},
		"tags":          {"edited,review"},
		"description":   {"Edited field visit"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("timesheet edit returned %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/timesheets" {
		t.Fatalf("timesheet edit location = %s", rec.Header().Get("Location"))
	}
	timesheets = getWithCookies(app, rec.Header().Get("Location"), withCookies(cookie, rec.Result().Cookies())...).Body.String()
	for _, expected := range []string{"Entry updated", "Edited field visit", "QA Review", "Internal"} {
		if !strings.Contains(timesheets, expected) {
			t.Fatalf("timesheets view missing %q after edit", expected)
		}
	}
	if strings.Contains(timesheets, "Original field visit") {
		t.Fatal("timesheets view still shows stale description after edit")
	}

	calendar := getWithCookie(app, "/calendar?date="+updatedStart.Format("2006-01-02"), cookie).Body.String()
	for _, expected := range []string{"Edited field visit", "QA Review", editHref} {
		if !strings.Contains(calendar, expected) {
			t.Fatalf("calendar missing %q after edit", expected)
		}
	}

	reports := getWithCookie(app, "/reports?group=activity&billable=false", cookie).Body.String()
	if !strings.Contains(reports, "QA Review") {
		t.Fatal("reports should reflect updated non-billable activity after edit")
	}

	createBody := getWithCookie(app, "/timesheets?date="+time.Now().UTC().Format("2006-01-02"), cookie).Body.String()
	createCSRF := csrfFromBody(t, createBody)
	rec = postFormWithCookie(app, "/timesheets", cookie, url.Values{
		"csrf":          {createCSRF},
		"customer_id":   {strconv.FormatInt(customer.ID, 10)},
		"project_id":    {strconv.FormatInt(project.ID, 10)},
		"activity_id":   {strconv.FormatInt(activity.ID, 10)},
		"task_id":       {strconv.FormatInt(task.ID, 10)},
		"start":         {time.Now().Add(-90 * time.Minute).Format("2006-01-02T15:04")},
		"end":           {time.Now().Add(-30 * time.Minute).Format("2006-01-02T15:04")},
		"break_minutes": {"0"},
		"description":   {"Creation still works"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("timesheet create after edit returned %d", rec.Code)
	}
	if body := getWithCookie(app, "/timesheets", cookie).Body.String(); !strings.Contains(body, "Creation still works") {
		t.Fatal("timesheet creation regression: new entry missing after edit flow")
	}
}

func TestTimesheetEditPermissionsValidationAndExportLocking(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	ctx := context.Background()
	customer, project, activity, task := seedSelectorFixtures(t, store)
	otherProject := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Other Project", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, otherProject); err != nil {
		t.Fatal(err)
	}
	otherTask := &domain.Task{WorkspaceID: 1, ProjectID: otherProject.ID, Name: "Wrong Task", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, otherTask); err != nil {
		t.Fatal(err)
	}
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	start := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Minute)
	end := start.Add(1 * time.Hour)
	entry := &domain.Timesheet{WorkspaceID: 1, UserID: admin.ID, CustomerID: customer.ID, ProjectID: project.ID, ActivityID: activity.ID, TaskID: &task.ID, StartedAt: start, EndedAt: &end, Timezone: "UTC", Billable: true, Description: "Protected entry"}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	editHref := "/timesheets/" + strconv.FormatInt(entry.ID, 10) + "/edit"

	if err := store.CreateUser(ctx, domain.User{
		Email:       "manager@example.com",
		Username:    "manager",
		DisplayName: "Manager",
		Timezone:    "UTC",
		Enabled:     true,
	}, "manager12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	manager, err := store.FindUserByEmail(ctx, "manager@example.com")
	if err != nil || manager == nil {
		t.Fatal("missing manager")
	}
	managerCookie := loginCookie(t, app, "manager@example.com", "manager12345")
	managerTimesheets := getWithCookie(app, "/timesheets", managerCookie).Body.String()
	if strings.Contains(managerTimesheets, editHref) {
		t.Fatal("unassigned user should not see edit affordance for another user's entry")
	}
	if rec := getWithCookie(app, editHref, managerCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized edit page returned %d, want 403", rec.Code)
	}
	managerCSRF := csrfFromBody(t, managerTimesheets)
	editPost := postFormWithCookie(app, "/timesheets/"+strconv.FormatInt(entry.ID, 10), managerCookie, url.Values{
		"csrf":          {managerCSRF},
		"customer_id":   {strconv.FormatInt(customer.ID, 10)},
		"project_id":    {strconv.FormatInt(project.ID, 10)},
		"activity_id":   {strconv.FormatInt(activity.ID, 10)},
		"task_id":       {strconv.FormatInt(task.ID, 10)},
		"start":         {start.Local().Format("2006-01-02T15:04")},
		"end":           {end.Local().Format("2006-01-02T15:04")},
		"break_minutes": {"0"},
		"description":   {"Should fail"},
	})
	if editPost.Code != http.StatusForbidden {
		t.Fatalf("unauthorized edit post returned %d, want 403", editPost.Code)
	}

	if err := store.AddProjectMember(ctx, project.ID, manager.ID, domain.ProjectRoleManager); err != nil {
		t.Fatal(err)
	}
	managerBody := getWithCookie(app, "/timesheets", managerCookie).Body.String()
	if !strings.Contains(managerBody, editHref) {
		t.Fatal("project manager should see edit affordance for project entry")
	}
	editPage := getWithCookie(app, editHref, managerCookie)
	if editPage.Code != http.StatusOK {
		t.Fatalf("manager edit page returned %d", editPage.Code)
	}
	csrf := csrfFromBody(t, editPage.Body.String())
	invalid := postFormWithCookie(app, "/timesheets/"+strconv.FormatInt(entry.ID, 10), managerCookie, url.Values{
		"csrf":          {csrf},
		"customer_id":   {strconv.FormatInt(customer.ID, 10)},
		"project_id":    {strconv.FormatInt(project.ID, 10)},
		"activity_id":   {strconv.FormatInt(activity.ID, 10)},
		"task_id":       {strconv.FormatInt(otherTask.ID, 10)},
		"start":         {start.Local().Format("2006-01-02T15:04")},
		"end":           {end.Local().Format("2006-01-02T15:04")},
		"break_minutes": {"0"},
		"description":   {"Invalid task update"},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid hierarchy edit returned %d, want 400", invalid.Code)
	}
	if !strings.Contains(invalid.Body.String(), "task does not belong to the selected project") {
		t.Fatal("invalid hierarchy edit should show validation message")
	}

	if _, err := store.DB().ExecContext(ctx, `UPDATE timesheets SET exported=1 WHERE id=?`, entry.ID); err != nil {
		t.Fatal(err)
	}
	if rec := getWithCookie(app, editHref, loginCookie(t, app, "admin@example.com", "admin12345")); rec.Code != http.StatusBadRequest {
		t.Fatalf("export-locked edit page returned %d, want 400", rec.Code)
	}
}

func TestProjectTemplateCreateEditAndUse(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	customer, _, _, _ := seedSelectorFixtures(t, store)
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/project-templates", cookie).Body.String()
	if !strings.Contains(body, "Create template") {
		t.Fatal("project templates screen did not render")
	}
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/project-templates", cookie, url.Values{
		"csrf":                 {csrf},
		"name":                 {"Implementation Template"},
		"project_name":         {"Implementation Project"},
		"estimate_hours":       {"12"},
		"budget_cents":         {"50000"},
		"budget_alert_percent": {"75"},
		"visible":              {"on"},
		"billable":             {"on"},
		"tasks":                {"Plan\nBuild"},
		"activities":           {"Consulting\nReview"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("template create returned %d", rec.Code)
	}
	var templateID int64
	if err := store.DB().QueryRowContext(context.Background(), `SELECT id FROM project_templates WHERE name='Implementation Template'`).Scan(&templateID); err != nil {
		t.Fatal(err)
	}
	body = getWithCookie(app, "/project-templates/"+strconv.FormatInt(templateID, 10), cookie).Body.String()
	if !strings.Contains(body, "Plan") || !strings.Contains(body, "Consulting") {
		t.Fatal("template detail did not render copied defaults")
	}
	csrf = csrfFromBody(t, body)
	rec = postFormWithCookie(app, "/project-templates/"+strconv.FormatInt(templateID, 10)+"/use", cookie, url.Values{
		"csrf":         {csrf},
		"customer_id":  {strconv.FormatInt(customer.ID, 10)},
		"project_name": {"Client Launch"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("template use returned %d", rec.Code)
	}
	var projectID int64
	if err := store.DB().QueryRowContext(context.Background(), `SELECT id FROM projects WHERE name='Client Launch'`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var taskCount, activityCount int
	if err := store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks WHERE project_id=?`, projectID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM activities WHERE project_id=?`, projectID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 2 || activityCount != 2 {
		t.Fatalf("template copied %d tasks and %d activities, want 2 and 2", taskCount, activityCount)
	}
}

func seedSelectorFixtures(t *testing.T, store *sqlite.Store) (*domain.Customer, *domain.Project, *domain.Activity, *domain.Task) {
	t.Helper()
	ctx := context.Background()
	customer := &domain.Customer{WorkspaceID: 1, Name: "Alpha Customer", Company: "ACME", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Buildout", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	activity := &domain.Activity{WorkspaceID: 1, ProjectID: &project.ID, Name: "Implementation", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activity); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{WorkspaceID: 1, ProjectID: project.ID, Name: "Launch task", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	return customer, project, activity, task
}

func testApp(t *testing.T) (*Server, *sqlite.Store) {
	t.Helper()
	return testAppWithConfig(t, config.Config{})
}

func testAppWithConfig(t *testing.T, overrides config.Config) (*Server, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SessionSecret: "test-secret", DefaultTimezone: "UTC", DefaultCurrency: "USD", FutureTimePolicy: "allow", DataDir: t.TempDir(), WebhookMaxRetries: 1, TOTPMode: "disabled"}
	cfg.RateLimitEnabled = overrides.RateLimitEnabled
	if overrides.TOTPMode != "" {
		cfg.TOTPMode = overrides.TOTPMode
	}
	if overrides.SMTPHost != "" {
		cfg.SMTPHost = overrides.SMTPHost
		cfg.SMTPPort = overrides.SMTPPort
		cfg.SMTPUsername = overrides.SMTPUsername
		cfg.SMTPPassword = overrides.SMTPPassword
		cfg.SMTPFrom = overrides.SMTPFrom
		cfg.SMTPStartTLS = overrides.SMTPStartTLS
	}
	if overrides.PublicURL != "" {
		cfg.PublicURL = overrides.PublicURL
	}
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
	return getWithCookies(app, target, cookie)
}

func getWithCookies(app *Server, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, cookie := range cookies {
		if cookie != nil {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func withCookies(first *http.Cookie, rest []*http.Cookie) []*http.Cookie {
	cookies := []*http.Cookie{}
	if first != nil {
		cookies = append(cookies, first)
	}
	return append(cookies, rest...)
}

func postFormWithCookie(app *Server, target string, cookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, items := range values {
		out[key] = append([]string(nil), items...)
	}
	return out
}

func mustFindTimesheetByDescription(t *testing.T, store *sqlite.Store, ctx context.Context, description string) domain.Timesheet {
	t.Helper()
	items, _, err := store.ListTimesheets(ctx, sqlite.TimesheetFilter{WorkspaceID: 1, Page: 1, Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Description == description {
			return item
		}
	}
	t.Fatalf("timesheet with description %q not found", description)
	return domain.Timesheet{}
}

func csrfFromBody(t *testing.T, body string) string {
	t.Helper()
	marker := `name="csrf" value="`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatal("missing csrf token")
	}
	start += len(marker)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		t.Fatal("unterminated csrf token")
	}
	return body[start : start+end]
}
