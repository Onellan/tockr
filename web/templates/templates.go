package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tockr/internal/auth"
	"tockr/internal/domain"
)

type NavUser struct {
	DisplayName          string
	Email                string
	CSRF                 string
	CurrentPath          string
	Permissions          map[string]bool
	CurrentWorkspaceID   int64
	CurrentWorkspaceName string
	Workspaces           []domain.Workspace
}

type navItem struct {
	Label      string
	Path       string
	Group      string
	Permission string
}

type timezoneOption struct {
	Value string
	Label string
}

type SelectOption struct {
	Value int64
	Label string
	Attrs map[string]string
}

type Notice struct {
	Kind    string
	Message string
}

type TimesheetPrefill struct {
	EntryID       int64
	EntryMode     string
	CustomerID    int64
	ProjectID     int64
	WorkstreamID  int64
	ActivityID    int64
	TaskID        int64
	Date          string
	ManualHours   string
	ManualMinutes string
	Start         string
	End           string
	BreakMinutes  string
	Tags          string
	Description   string
	Billable      bool
	Notice        Notice
	Message       string
}

type SelectorData struct {
	Customers   []SelectOption
	Projects    []SelectOption
	Workstreams []SelectOption
	Activities  []SelectOption
	Tasks       []SelectOption
	Users       []SelectOption
	Groups      []SelectOption

	CustomerLabels   map[int64]string
	ProjectLabels    map[int64]string
	WorkstreamLabels map[int64]string
	ActivityLabels   map[int64]string
	TaskLabels       map[int64]string
	UserLabels       map[int64]string
	GroupLabels      map[int64]string
}

type SMTPSettingsView struct {
	Host               string
	Port               int
	UsernameConfigured bool
	PasswordConfigured bool
	From               string
	StartTLS           bool
	PublicURL          string
	Configured         bool
	Valid              bool
	Error              string
}

var primaryNav = []navItem{
	{"Dashboard", "/", "Work", ""},
	{"Timesheets", "/timesheets", "Work", auth.PermTrackTime},
	{"Calendar", "/calendar", "Work", auth.PermTrackTime},
	{"Clients", "/customers", "Projects / Delivery", ""},
	{"Projects", "/projects", "Projects / Delivery", ""},
	{"Project Dashboards", "/project-dashboards", "Projects / Delivery", auth.PermManageProjects},
	{"Workstreams", "/workstreams", "Projects / Delivery", auth.PermManageMaster},
	{"Deliverables", "/activities", "Projects / Delivery", ""},
	{"Tasks", "/tasks", "Projects / Delivery", ""},
	{"Groups", "/groups", "Projects / Delivery", auth.PermManageGroups},
	{"Templates", "/project-templates", "Projects / Delivery", auth.PermManageProjects},
	{"Tags", "/tags", "Projects / Delivery", auth.PermTrackTime},
	{"Reports", "/reports", "Billing / Analysis", auth.PermViewReports},
	{"Utilization", "/reports/utilization", "Billing / Analysis", auth.PermViewReports},
	{"Invoices", "/invoices", "Billing / Analysis", auth.PermManageInvoices},
	{"Rates", "/rates", "Billing / Analysis", auth.PermManageRates},
}

var adminNav = []navItem{
	{"Admin Home", "/admin", "Administration", ""},
	{"Workspaces", "/admin/workspaces", "Administration", auth.PermManageOrg},
	{"Work Schedule", "/admin/schedule", "Administration", auth.PermManageOrg},
	{"Email", "/admin/email", "Administration", auth.PermManageOrg},
	{"Demo Data", "/admin/demo-data", "Administration", auth.PermManageOrg},
	{"Rates", "/rates", "Administration", auth.PermManageRates},
	{"Exchange Rates", "/admin/exchange-rates", "Administration", auth.PermManageRates},
	{"Recalculate", "/admin/recalculate", "Administration", auth.PermManageRates},
	{"Users", "/admin/users", "Administration", auth.PermManageUsers},
	{"Webhooks", "/webhooks", "Administration", auth.PermManageWebhooks},
}

var timezoneOptions = []timezoneOption{
	{"UTC", "UTC"},
	{"Africa/Johannesburg", "Africa/Johannesburg"},
	{"Africa/Nairobi", "Africa/Nairobi"},
	{"Africa/Lagos", "Africa/Lagos"},
	{"Europe/London", "Europe/London"},
	{"Europe/Berlin", "Europe/Berlin"},
	{"Europe/Paris", "Europe/Paris"},
	{"Europe/Amsterdam", "Europe/Amsterdam"},
	{"Europe/Madrid", "Europe/Madrid"},
	{"Europe/Rome", "Europe/Rome"},
	{"Europe/Zurich", "Europe/Zurich"},
	{"Europe/Athens", "Europe/Athens"},
	{"Europe/Helsinki", "Europe/Helsinki"},
	{"Europe/Moscow", "Europe/Moscow"},
	{"Asia/Dubai", "Asia/Dubai"},
	{"Asia/Kolkata", "Asia/Kolkata"},
	{"Asia/Singapore", "Asia/Singapore"},
	{"Asia/Hong_Kong", "Asia/Hong_Kong"},
	{"Asia/Tokyo", "Asia/Tokyo"},
	{"Asia/Seoul", "Asia/Seoul"},
	{"Australia/Perth", "Australia/Perth"},
	{"Australia/Sydney", "Australia/Sydney"},
	{"Pacific/Auckland", "Pacific/Auckland"},
	{"America/Sao_Paulo", "America/Sao_Paulo"},
	{"America/New_York", "America/New_York"},
	{"America/Chicago", "America/Chicago"},
	{"America/Denver", "America/Denver"},
	{"America/Phoenix", "America/Phoenix"},
	{"America/Los_Angeles", "America/Los_Angeles"},
	{"America/Toronto", "America/Toronto"},
}

func Layout(title string, user *NavUser, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>%s</title><link rel=\"icon\" href=\"/favicon.ico?v=20260423-brand\" sizes=\"any\"><link rel=\"icon\" type=\"image/png\" sizes=\"32x32\" href=\"/static/favicon-32x32.png?v=20260423-brand\"><link rel=\"icon\" type=\"image/png\" sizes=\"16x16\" href=\"/static/favicon-16x16.png?v=20260423-brand\"><link rel=\"apple-touch-icon\" sizes=\"180x180\" href=\"/static/apple-touch-icon.png?v=20260423-brand\"><link rel=\"manifest\" href=\"/static/site.webmanifest?v=20260423-brand\"><meta name=\"theme-color\" content=\"#2F80ED\"><link rel=\"stylesheet\" href=\"/static/style.css?v=20260423-brand\"><script src=\"/static/menu.js?v=20260422-navfix\" defer></script></head><body class=\"%s\">", esc(title), esc(bodyClass(title)))
		if user == nil {
			_, _ = fmt.Fprint(w, `<main class="auth-main">`)
			if err := body.Render(ctx, w); err != nil {
				return err
			}
			_, _ = fmt.Fprint(w, `</main></body></html>`)
			return nil
		}
		navItems := primaryNav
		navLabel := "Primary navigation"
		showBackToDashboard := false
		if isAdminArea(user) {
			navItems = adminSidebarNav(user)
			navLabel = "Admin navigation"
			showBackToDashboard = true
		}
		_, _ = fmt.Fprint(w, `<a class="skip-link" href="#main-content">Skip to content</a><div class="mobile-nav-backdrop" data-mobile-nav-close hidden></div><div class="app-shell" data-app-shell><aside class="sidebar" id="app-sidebar" aria-label="Application navigation"><div class="sidebar-head"><a class="brand" href="/" aria-label="Tockr dashboard"><span class="brand-mark">T</span><span><strong>Tockr</strong><small>Time operations</small></span></a><button class="mobile-nav-close" type="button" data-mobile-nav-close aria-label="Close menu">Close</button></div>`)
		_, _ = fmt.Fprintf(w, `<nav class="side-nav" aria-label="%s">`, esc(navLabel))
		renderNav(w, user, navItems)
		if showBackToDashboard {
			_, _ = fmt.Fprint(w, `<div class="sidebar-back-link"><a class="nav-link nav-link-back" href="/">Back to dashboard</a></div>`)
		}
		_, _ = fmt.Fprintf(w, `</nav></aside><div class="workspace"><header class="topbar"><button class="mobile-menu-toggle" type="button" data-mobile-nav-toggle aria-controls="app-sidebar" aria-expanded="false">Menu</button><div class="topbar-title"><span class="topbar-kicker">Workspace</span><strong>%s</strong></div><div class="account-area">`, esc(title))
		renderAccountDropdown(w, user)
		_, _ = fmt.Fprint(w, `</div></header><main class="content" id="main-content" tabindex="-1">`)
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</main></div></div></body></html>`)
		return nil
	})
}

func Login(notice Notice) templ.Component {
	return Layout("Login", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Know exactly where your team's time goes — and bill every hour with confidence.</p><ul><li>Capture time before it slips away</li><li>Turn hours into accurate, ready-to-send invoices</li><li>Keep projects on budget with real-time visibility</li></ul></div><form method="post" action="/login" class="login-card"><div><h2>Welcome back</h2><p>Sign in to your account.</p></div>`)
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><label>Two-factor code <input name="totp" inputmode="numeric" autocomplete="one-time-code" placeholder="Only if enabled"></label><button class="primary full">Login</button><a class="auth-link" href="/forgot-password">Forgot password?</a></form></section>`)
		return nil
	}))
}

func LoginOTPChallenge(notice Notice) templ.Component {
	return Layout("Sign-in code", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Check your email for a 6-digit sign-in code.</p></div><form method="post" action="/login/otp" class="login-card"><div><h2>Check your email</h2><p>Enter the code we sent to your email address. It expires in 10 minutes.</p></div>`)
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<label>Sign-in code<input name="code" inputmode="numeric" autocomplete="one-time-code" placeholder="000000" required autofocus></label><button class="primary full">Verify code</button><a class="auth-link" href="/login">Back to login</a></form></section>`)
		return nil
	}))
}

func ForgotPassword(notice Notice) templ.Component {
	return Layout("Forgot password", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Reset access with a time-limited email link.</p></div><form method="post" action="/forgot-password" class="login-card"><div><h2>Forgot password</h2><p>Enter your account email and check your inbox.</p></div>`)
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<label>Email<input name="email" type="email" autocomplete="username" required></label><button class="primary full">Send reset link</button><a class="auth-link" href="/login">Back to login</a></form></section>`)
		return nil
	}))
}

func ResetPassword(token string, notice Notice) templ.Component {
	return Layout("Reset password", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Choose a new password for local authentication.</p></div><form method="post" action="/reset-password" class="login-card"><div><h2>Reset password</h2><p>Reset links expire and can be used once.</p></div>`)
		renderNotice(w, notice)
		_, _ = fmt.Fprintf(w, `<input type="hidden" name="token" value="%s"><label>New password<input name="password" type="password" minlength="8" autocomplete="new-password" required></label><label>Confirm password<input name="confirm" type="password" minlength="8" autocomplete="new-password" required></label><button class="primary full">Update password</button><a class="auth-link" href="/login">Back to login</a></form></section>`, esc(token))
		return nil
	}))
}

func Dashboard(user *NavUser, summary domain.DashboardSummary, active *domain.Timesheet, selectors *SelectorData) templ.Component {
	return Layout("Dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Dashboard", "Engineering operations", "Start work quickly, stay on top of missing time, and keep billing risk visible.")
		_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
		metric(w, "Today tracked", duration(summary.Stats["today_seconds"]), "Logged by you today")
		metric(w, "Week to date", duration(summary.WeekTracked), "Current consulting week")
		metric(w, "Missing this week", duration(summary.MissingSeconds), "Hours still to capture")
		metric(w, "Active timers", fmt.Sprint(summary.Stats["active_timers"]), "Currently running across workspace")
		_, _ = fmt.Fprint(w, `</section><section class="two-col"><div class="panel"><div class="panel-head"><div><h2>Quick log and timer <span class="tooltip-icon" data-tooltip="Use this for the work package you are on right now. Choose the client, project, deliverable, and optional task before starting or logging time.">i</span></h2><p>Log a date and duration quickly, or start a live timer with the same work classification.</p></div></div>`)
		if active != nil {
			_, _ = fmt.Fprintf(w, `<div class="timer-running"><span class="status-dot"></span><div><strong>Running since %s</strong><p>Your current timer is active. Stop it when the work package is complete.</p></div></div><form method="post" action="/timesheets/stop" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Stop timer</button><a class="ghost-button" href="/timesheets">Open weekly review</a></form>`, esc(active.StartedAt.Format("15:04")), esc(user.CSRF))
		}
		renderDashboardQuickLogForm(w, user, selectors, active == nil)
		_, _ = fmt.Fprint(w, `<div class="summary-list section-spacer">`)
		if len(summary.RecentWork) == 0 {
			_, _ = fmt.Fprint(w, `<div><span>No recent work yet</span><strong>Your recently tracked tasks will appear here for quick reuse.</strong></div>`)
		}
		for index, item := range summary.RecentWork {
			if index == 0 {
				_, _ = fmt.Fprint(w, `<div><span>Continue recent work</span><strong>Reuse your last client / project / deliverable combination in one click.</strong></div>`)
			}
			desc := item.Description
			if desc == "" {
				desc = recentWorkLabel(item, selectors)
			}
			actions := `<a class="table-action" href="` + esc(timesheetPrefillHref(item)) + `">Use again</a>`
			if item.TimesheetID > 0 {
				actions += ` <a class="table-action" href="` + esc(timesheetEditHref(item.TimesheetID)) + `">Edit</a>`
			}
			_, _ = fmt.Fprintf(w, `<div><span>%s</span><strong>%s</strong>%s</div>`,
				esc(desc),
				esc(duration(item.DurationSeconds)+" · "+item.StartedAt.Format("Mon 02 Jan 15:04")),
				actions)
		}
		_, _ = fmt.Fprint(w, `</div></div><div class="panel"><div class="panel-head"><div><h2>Project watchlist</h2><p>Projects nearing estimate or fixed-fee budget thresholds.</p></div></div>`)
		if len(summary.ProjectWatchlist) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No project watch items</strong><span>Projects with delivery risk or billing attention will appear here.</span></div>`)
		} else {
			_, _ = fmt.Fprint(w, `<div class="summary-list">`)
			for _, item := range summary.ProjectWatchlist {
				_, _ = fmt.Fprintf(w, `<div><span>%s</span><strong>%s</strong><small>%s</small></div>`,
					esc(label(selectors.ProjectLabels, item.ProjectID)),
					esc(projectWatchSummary(item)),
					esc(projectWatchMeta(item)))
			}
			_, _ = fmt.Fprint(w, `</div>`)
		}
		_, _ = fmt.Fprint(w, `</div></section>`)
		return nil
	}))
}

func renderDashboardQuickLogForm(w io.Writer, user *NavUser, selectors *SelectorData, canStartTimer bool) {
	_, _ = fmt.Fprintf(w, `<form method="post" action="/timesheets" class="compact-form selector-form quick-log-form"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="entry_mode" value="manual">`, esc(user.CSRF))
	renderWorkSelectors(w, selectors, true)
	_, _ = fmt.Fprintf(w, `<label>Date<input type="date" name="date" value="%s" required></label><label>Hours<input type="number" name="hours" min="0" step="1" inputmode="numeric" value="0" required></label><label>Minutes<input type="number" name="minutes" min="0" max="59" step="1" inputmode="numeric" value="0" required></label><input name="description" placeholder="Describe the work">`, esc(time.Now().UTC().Format("2006-01-02")))
	_, _ = fmt.Fprint(w, `<button class="primary">Quick log</button>`)
	if canStartTimer {
		_, _ = fmt.Fprint(w, `<button class="ghost-button" formaction="/timesheets/start">Start timer</button>`)
	}
	_, _ = fmt.Fprint(w, `<a class="ghost-button" href="/timesheets">Open timesheets</a></form>`)
}

func EntityList[T any](title string, user *NavUser, headers []string, rows [][]string, form templ.Component) templ.Component {
	return Layout(title, user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, title, "Directory", "Create and maintain the records used by time entries and reporting.")
		if form != nil {
			createTitle := "Create " + singularTitle(title)
			renderCreateCardStart(w, createTitle, "Keep required fields tight and searchable.", createCardCollapsedByDefault(title))
			if err := form.Render(ctx, w); err != nil {
				return err
			}
			renderCreateCardEnd(w, createCardCollapsedByDefault(title))
		}
		dataTable(w, headers, rows)
		return nil
	}))
}

func EntityListRaw(title string, user *NavUser, headers []string, rows [][]string, form templ.Component) templ.Component {
	return Layout(title, user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, title, "Directory", "Create and maintain the records used by time entries and reporting.")
		if form != nil {
			if strings.EqualFold(strings.TrimSpace(title), "projects") {
				_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create project</h2><p>Start the guided project setup workflow.</p></div></div>`)
				if err := form.Render(ctx, w); err != nil {
					return err
				}
				_, _ = fmt.Fprint(w, `</section>`)
			} else {
				createTitle := "Create " + singularTitle(title)
				renderCreateCardStart(w, createTitle, "Keep required fields tight and searchable.", createCardCollapsedByDefault(title))
				if err := form.Render(ctx, w); err != nil {
					return err
				}
				renderCreateCardEnd(w, createCardCollapsedByDefault(title))
			}
		}
		dataTableRaw(w, headers, rows)
		return nil
	}))
}

func createCardCollapsedByDefault(title string) bool {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "clients", "deliverables", "tasks", "groups", "users":
		return true
	default:
		return false
	}
}

func renderCreateCardStart(w io.Writer, title, description string, collapsed bool) {
	if collapsed {
		_, _ = fmt.Fprintf(w, `<details class="panel form-panel collapsible-create-card"><summary class="panel-head collapsible-create-summary"><div><h2>%s</h2><p>%s</p></div><span class="collapse-indicator" aria-hidden="true"></span></summary>`, esc(title), esc(description))
		return
	}
	_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>%s</h2><p>%s</p></div></div>`, esc(title), esc(description))
}

func renderCreateCardEnd(w io.Writer, collapsed bool) {
	if collapsed {
		_, _ = fmt.Fprint(w, `</details>`)
		return
	}
	_, _ = fmt.Fprint(w, `</section>`)
}

func CustomerForm(user *NavUser, c *domain.Customer) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/customers"
		visibleChecked := ` checked`
		billableChecked := ` checked`
		if c != nil && c.ID > 0 {
			action = fmt.Sprintf("/customers/%d", c.ID)
			visibleChecked = checkedIf(c.Visible)
			billableChecked = checkedIf(c.Billable)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Client name<input name="name" value="%s" required></label><label>Company<input name="company" value="%s"></label><label>Billing email<input name="email" value="%s"></label><label>Billing unit <span class="field-hint">ISO 4217 currency code — e.g. ZAR, USD, EUR</span><input name="currency" value="%s" placeholder="ZAR" maxlength="3"></label>`,
			action, esc(user.CSRF), val(c, "Name"), val(c, "Company"), val(c, "Email"), defaultVal(val(c, "Currency"), "ZAR"))
		renderTimezoneSelect(w, "Timezone", "timezone", defaultVal(val(c, "Timezone"), "UTC"), false)
		_, _ = fmt.Fprintf(w, `<label>Client ID <span class="field-hint">leave blank to auto-generate (e.g. CL-000001)</span><input name="number" value="%s"></label><label class="wide">Comment<textarea name="comment">%s</textarea></label><label class="check"><input type="checkbox" name="visible"%s> Visible</label><label class="check"><input type="checkbox" name="billable"%s> Billable</label><div class="form-actions"><button class="primary">Save client</button></div></form>`,
			val(c, "Number"), val(c, "Comment"), visibleChecked, billableChecked)
		return nil
	})
}

func ProjectForm(user *NavUser, selectors *SelectorData, project *domain.Project) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/projects"
		selectedCustomer := int64(0)
		name := ""
		number := ""
		orderNo := ""
		estimateHours := int64(0)
		budgetCents := int64(0)
		budgetAlert := int64(80)
		comment := ""
		visibleChecked := ` checked`
		privateChecked := ""
		billableChecked := ` checked`
		if project != nil {
			action = fmt.Sprintf("/projects/%d", project.ID)
			selectedCustomer = project.CustomerID
			name = project.Name
			number = project.Number
			orderNo = project.OrderNo
			estimateHours = project.EstimateSeconds / 3600
			budgetCents = project.BudgetCents
			budgetAlert = defaultInt(project.BudgetAlertPercent, 80)
			comment = project.Comment
			visibleChecked = checkedIf(project.Visible)
			privateChecked = checkedIf(project.Private)
			billableChecked = checkedIf(project.Billable)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid project-form" method="post" action="%s"><input type="hidden" name="csrf" value="%s">`, esc(action), esc(user.CSRF))
		renderSelect(w, "Customer"+tipHTML("The client this project is billed to. Determines the billing unit and default billing contact."), "customer_id", optionList(selectors, "customer"), selectedCustomer, true, "Select a customer", nil)
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" value="%s" required></label>`, tipHTML("Project display name shown in timesheets, reports and invoices."), esc(name))
		_, _ = fmt.Fprintf(w, `<label>Project ID %s<input name="number" value="%s" placeholder="auto-generated if blank"></label>`, tipHTML("Internal reference code (e.g. PR-000001). Leave blank to auto-generate."), esc(number))
		_, _ = fmt.Fprintf(w, `<label>Order number %s<input name="order_number" value="%s"></label>`, tipHTML("Purchase order or contract reference number for invoice line items."), esc(orderNo))
		_, _ = fmt.Fprintf(w, `<label>Estimate hours %s<input name="estimate_hours" value="%d"></label>`, tipHTML("Total hours budgeted. Tockr shows burn against this in the project dashboard."), estimateHours)
		_, _ = fmt.Fprintf(w, `<label>Budget %s<input name="budget" value="%d" placeholder="e.g. 10000"></label>`, tipHTML("Monetary budget in your primary billing currency unit. Triggers an alert when spend reaches the alert threshold."), budgetCents)
		_, _ = fmt.Fprintf(w, `<label>Budget alert (%%) %s<input name="budget_alert_percent" value="%d"></label>`, tipHTML("Send a budget warning when this percentage of the monetary budget is consumed (e.g. 80 = alert at 80%)."), budgetAlert)
		_, _ = fmt.Fprintf(w, `<label class="wide">Comment<textarea name="comment">%s</textarea></label>`, esc(comment))
		_, _ = fmt.Fprint(w, `<div class="project-form-flags">`)
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible"%s> Visible %s</label>`, visibleChecked, tipHTML("Visible projects appear in timesheet entry selectors. Uncheck to archive a project."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="private"%s> Private %s</label>`, privateChecked, tipHTML("Private projects are only visible to explicitly assigned members."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable"%s> Billable %s</label>`, billableChecked, tipHTML("Billable projects are included in invoice and rate calculations."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save project</button></div></div></form>`)
		return nil
	})
}

func EditProject(user *NavUser, selectors *SelectorData, project domain.Project) templ.Component {
	return Layout("Edit Project", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Edit project", "Projects / Delivery", "Update the project configuration and return to its delivery dashboard.")
		_, _ = fmt.Fprintf(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/projects/%d/dashboard">Back to project dashboard</a></div>`, project.ID)
		_, _ = fmt.Fprint(w, `<section class="panel form-panel">`)
		if err := ProjectForm(user, selectors, &project).Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func ActivityForm(user *NavUser, selectors *SelectorData, activity *domain.Activity) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/activities"
		selectedProject := int64(0)
		name := ""
		number := ""
		comment := ""
		visibleChecked := ` checked`
		billableChecked := ` checked`
		if activity != nil {
			action = fmt.Sprintf("/activities/%d", activity.ID)
			if activity.ProjectID != nil {
				selectedProject = *activity.ProjectID
			}
			name = activity.Name
			number = activity.Number
			comment = activity.Comment
			visibleChecked = checkedIf(activity.Visible)
			billableChecked = checkedIf(activity.Billable)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s">`, esc(action), esc(user.CSRF))
		renderSelect(w, "Project"+tipHTML("Scope this activity to one project, or leave blank to make it available across all projects."), "project_id", optionList(selectors, "project"), selectedProject, false, "Global activity", nil)
		_, _ = fmt.Fprintf(w, `<label>Deliverable %s<input name="name" value="%s" required></label>`, tipHTML("Deliverable shown in timesheets (e.g. design review, site visit, coordination, QA/rework)."), esc(name))
		_, _ = fmt.Fprintf(w, `<label>Deliverable ID %s<input name="number" value="%s" placeholder="auto-generated if blank"></label>`, tipHTML("Optional reference code. Auto-generated as DL-XXXXXX if left blank."), esc(number))
		_, _ = fmt.Fprintf(w, `<label class="wide">Comment<textarea name="comment">%s</textarea></label>`, esc(comment))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible"%s> Visible %s</label>`, visibleChecked, tipHTML("Visible activities appear in timesheet selectors. Uncheck to retire an activity without deleting it."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable"%s> Billable %s</label>`, billableChecked, tipHTML("Billable activities are included in invoice and rate calculations."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save deliverable</button></div></form>`)
		return nil
	})
}

func TaskForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/tasks"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Project"+tipHTML("Tasks must belong to a project. Choose which project this task falls under."), "project_id", optionList(selectors, "project"), 0, true, "Select a project", nil)
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" required></label>`, tipHTML("Task name shown in timesheet selectors (e.g. a sprint ticket or deliverable)."))
		_, _ = fmt.Fprintf(w, `<label>Task ID %s<input name="number" placeholder="auto-generated if blank"></label>`, tipHTML("Optional reference code — auto-generated as TSK-XXXXX if left blank. Shown in reports and exports."))
		_, _ = fmt.Fprintf(w, `<label>Estimate hours %s<input name="estimate_hours" value="0"></label>`, tipHTML("Estimated hours for this task. Used alongside the project estimate for burn-down tracking."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible" checked> Visible %s</label>`, tipHTML("Visible tasks appear in timesheet selectors. Uncheck to archive without deleting."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable" checked> Billable %s</label>`, tipHTML("Billable tasks contribute to invoice totals. Non-billable tasks are tracked but not charged."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save task</button></div></form>`)
		return nil
	})
}

func TagForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="toolbar-form" method="post" action="/tags"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Tag name" required><button class="primary">Save tag</button></form>`, esc(user.CSRF))
		return nil
	})
}

func GroupForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/groups"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" required></label>`, tipHTML("Group name used to assign multiple users to projects at once. E.g. \"Backend team\" or \"Contractors\"."))
		_, _ = fmt.Fprintf(w, `<label class="wide">Description %s<textarea name="description"></textarea></label>`, tipHTML("Optional description of this group's purpose or membership criteria."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save group</button></div></form>`)
		return nil
	})
}

func RateForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid selector-form" method="post" action="/rates"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), 0, false, "Any customer", nil)
		renderSelect(w, "Project", "project_id", optionList(selectors, "project"), 0, false, "Any project", map[string]string{"data-filter-parent": "customer_id", "data-filter-attr": "customer-id"})
		renderSelect(w, "Activity", "activity_id", optionList(selectors, "activity"), 0, false, "Any activity", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "Task", "task_id", optionList(selectors, "task"), 0, false, "Any task", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "User", "user_id", optionList(selectors, "user"), 0, false, "Any user", nil)
		_, _ = fmt.Fprint(w, `<label>Billable rate per hour<input name="amount" required placeholder="e.g. 100"></label><label>Internal cost rate per hour <span class="field-hint">optional, for margin reporting</span><input name="internal_amount" placeholder="e.g. 60"></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to <span class="field-hint">leave blank for open-ended</span><input type="date" name="effective_to"></label><label class="check"><input type="checkbox" name="fixed"> Fixed total <span class="field-hint">charges the rate as a flat fee, not per hour</span></label><div class="form-actions"><button class="primary">Save rate</button></div></form>`)
		return nil
	})
}

func UserCostForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/rates/costs"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "User", "user_id", optionList(selectors, "user"), 0, true, "Select a user", nil)
		_, _ = fmt.Fprint(w, `<label>User cost per hour<input name="amount" required placeholder="e.g. 75"></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to<input type="date" name="effective_to"></label><div class="form-actions"><button class="primary">Save user cost</button></div></form>`)
		return nil
	})
}

func Rates(user *NavUser, rates []domain.Rate, costs []domain.UserCostRate, selectors *SelectorData) templ.Component {
	return Layout("Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Rates", "Financial controls", "Date-effective billable rates and user cost rates for auditable reporting.")
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How rate matching works:</strong> Tockr picks the most specific matching rate for each time entry. Leave a selector blank to make a rate apply more broadly — a rate with all fields blank applies to everyone. More specific rates always win over broader ones.</div>`)
		_, _ = fmt.Fprint(w, `<section class="two-col">`)
		renderCreateCardStart(w, "Billable rate", "Scope by customer, project, activity, task, or user.", true)
		if err := RateForm(user, selectors).Render(ctx, w); err != nil {
			return err
		}
		renderCreateCardEnd(w, true)
		renderCreateCardStart(w, "User cost", "Use effective dates before profitability reporting.", true)
		if err := UserCostForm(user, selectors).Render(ctx, w); err != nil {
			return err
		}
		renderCreateCardEnd(w, true)
		_, _ = fmt.Fprint(w, `</section>`)
		rateRows := [][]string{}
		for _, rate := range rates {
			rateRows = append(rateRows, []string{labelPtr(selectors.CustomerLabels, rate.CustomerID), labelPtr(selectors.ProjectLabels, rate.ProjectID), labelPtr(selectors.ActivityLabels, rate.ActivityID), labelPtr(selectors.TaskLabels, rate.TaskID), labelPtr(selectors.UserLabels, rate.UserID), money(rate.AmountCents), ptrText(rate.InternalAmountCents), dateInput(&rate.EffectiveFrom), dateInput(rate.EffectiveTo)})
		}
		dataTable(w, []string{"Customer", "Project", "Activity", "Task", "User", "Bill rate", "Internal", "From", "To"}, rateRows)
		costRows := [][]string{}
		for _, cost := range costs {
			costRows = append(costRows, []string{label(selectors.UserLabels, cost.UserID), money(cost.AmountCents), dateInput(&cost.EffectiveFrom), dateInput(cost.EffectiveTo)})
		}
		_, _ = fmt.Fprint(w, `<div class="section-spacer"></div>`)
		dataTable(w, []string{"User", "Cost", "From", "To"}, costRows)
		return nil
	}))
}

func Timesheets(user *NavUser, entries []domain.Timesheet, selectors *SelectorData, favorites []domain.Favorite, recent []domain.DashboardRecentWork, prefill TimesheetPrefill, editable map[int64]bool) templ.Component {
	return Layout("Timesheets", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Timesheets", "Time capture", "Manual entry first. Use recent work, favorites, and clear billing context to backfill quickly and accurately.")
		renderNotice(w, prefill.Notice)
		if strings.TrimSpace(prefill.Message) != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(prefill.Message))
		}
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Log engineering time</h2><p>Choose the client, project, deliverable, and optional task before saving the entry.</p></div></div><form method="post" action="/timesheets" class="form-grid selector-form"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderWorkSelectorsWithSelected(w, selectors, true, prefill)
		renderTimesheetTimeFields(w, prefill, false)
		_, _ = fmt.Fprintf(w, `<label>Tags <span class="field-hint">Optional reporting labels only.</span><input name="tags" placeholder="qa,site-visit" value="%s"></label><label class="wide">Description<textarea name="description">%s</textarea></label><div class="form-actions"><button class="primary">Add entry</button></div></form></div>`,
			esc(prefill.Tags), esc(prefill.Description))
		_, _ = fmt.Fprint(w, `<div class="panel"><div class="panel-head"><div><h2>Recent and repeat work</h2><p>Reuse common consulting tasks without rebuilding the full selection.</p></div></div><div class="summary-list work-reuse-list">`)
		if len(recent) == 0 && len(favorites) == 0 {
			_, _ = fmt.Fprint(w, `<div><span>No recent work yet</span><strong>Recent entries and favorites will appear here after you start logging time.</strong></div>`)
		}
		for _, item := range recent {
			_, _ = fmt.Fprintf(w, `<article class="work-reuse-item"><span>%s</span><strong>%s</strong><a class="table-action" href="%s">Use again</a></article>`,
				esc(recentWorkLabel(item, selectors)),
				esc(duration(item.DurationSeconds)+" · "+item.StartedAt.Format("Mon 02 Jan 15:04")),
				esc(timesheetPrefillHref(item)))
		}
		for _, favorite := range favorites {
			_, _ = fmt.Fprintf(w, `<article class="work-reuse-item"><span>Favorite</span><strong>%s</strong><a class="table-action" href="%s">Use favorite</a></article>`,
				esc(favorite.Name),
				esc(favoritePrefillHref(favorite)))
		}
		_, _ = fmt.Fprint(w, `</div></div></section>`)
		rows := [][]string{}
		for _, entry := range entries {
			task := ""
			if entry.TaskID != nil {
				task = label(selectors.TaskLabels, *entry.TaskID)
			} else {
				task = "No task"
			}
			end := "Running"
			if entry.EndedAt != nil {
				end = FormatTime(*entry.EndedAt)
			}
			rows = append(rows, []string{
				label(selectors.CustomerLabels, entry.CustomerID),
				label(selectors.ProjectLabels, entry.ProjectID),
				label(selectors.ActivityLabels, entry.ActivityID),
				task,
				FormatTime(entry.StartedAt),
				end,
				duration(entry.DurationSeconds),
				Money(entry.RateCents),
				humanBillable(entry.Billable),
				humanExport(entry.Exported),
				esc(entry.Description),
				timesheetActions(entry, editable),
			})
		}
		dataTableRaw(w, []string{"Client", "Project", "Deliverable", "Task", "Start", "End", "Duration", "Rate", "Billable", "Exported", "Description", "Actions"}, rows)
		_, _ = fmt.Fprint(w, `<div class="export-row"><a class="ghost-button" href="/timesheets/export">Export CSV</a></div>`)
		return nil
	}))
}

func EditTimesheet(user *NavUser, selectors *SelectorData, prefill TimesheetPrefill, message string, exported bool) templ.Component {
	return Layout("Edit Timesheet", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Edit entry", "Timesheets", "Update the captured work package without recreating the entry.")
		if strings.TrimSpace(message) != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		if exported {
			_, _ = fmt.Fprint(w, `<div class="alert">Exported entries are locked to preserve invoice and export integrity.</div>`)
		}
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Edit timesheet entry</h2><p>Adjust the entry details, then save or cancel back to the weekly timesheet view.</p></div></div><form method="post" action="/timesheets/%d" class="form-grid selector-form"><input type="hidden" name="csrf" value="%s">`, prefill.EntryID, esc(user.CSRF))
		renderWorkSelectorsWithSelected(w, selectors, true, prefill)
		renderTimesheetTimeFields(w, prefill, true)
		_, _ = fmt.Fprintf(w, `<label>Tags <span class="field-hint">Optional reporting labels only.</span><input name="tags" placeholder="qa,site-visit" value="%s"></label><label class="check"><input type="checkbox" name="billable" value="1"%s> Billable</label><label class="wide">Description<textarea name="description">%s</textarea></label><div class="form-actions"><button class="primary">Save changes</button><a class="ghost-button" href="/timesheets">Cancel</a></div></form></section>`,
			esc(prefill.Tags), checkedIf(prefill.Billable), esc(prefill.Description))
		return nil
	}))
}

func renderTimesheetTimeFields(w io.Writer, prefill TimesheetPrefill, editing bool) {
	mode := timesheetEntryMode(prefill.EntryMode)
	manualHidden := ""
	rangeHidden := ` hidden`
	if mode == "range" {
		manualHidden = ` hidden`
		rangeHidden = ""
	}
	manualChecked := checkedIf(mode == "manual")
	rangeChecked := checkedIf(mode == "range")
	if prefill.Date == "" {
		prefill.Date = time.Now().UTC().Format("2006-01-02")
	}
	if prefill.ManualHours == "" {
		prefill.ManualHours = "0"
	}
	if prefill.ManualMinutes == "" {
		prefill.ManualMinutes = "0"
	}
	if prefill.BreakMinutes == "" {
		prefill.BreakMinutes = "0"
	}
	rangeIntro := "Use exact start and end times."
	if editing {
		rangeIntro = "Keep or adjust exact start and end times."
	}
	_, _ = fmt.Fprint(w, `<div class="entry-mode wide" data-entry-mode><span class="entry-mode-label">Entry mode</span><div class="segmented-control" role="radiogroup" aria-label="Timesheet entry mode">`)
	_, _ = fmt.Fprintf(w, `<label><input type="radio" name="entry_mode" value="manual"%s><span>Date + duration</span></label>`, manualChecked)
	_, _ = fmt.Fprintf(w, `<label><input type="radio" name="entry_mode" value="range"%s><span>Start + end</span></label>`, rangeChecked)
	_, _ = fmt.Fprint(w, `</div></div>`)
	_, _ = fmt.Fprintf(w, `<fieldset class="entry-mode-panel wide" data-entry-mode-panel="manual"%s><legend>Manual duration</legend><p>Date, hours, and minutes.</p><div class="entry-duration-grid"><label>Date<input type="date" name="date" value="%s" data-required="true"></label><label>Hours<input type="number" name="hours" min="0" step="1" inputmode="numeric" value="%s" data-required="true"></label><label>Minutes<input type="number" name="minutes" min="0" max="59" step="1" inputmode="numeric" value="%s" data-required="true"></label></div></fieldset>`,
		manualHidden, esc(prefill.Date), esc(prefill.ManualHours), esc(prefill.ManualMinutes))
	_, _ = fmt.Fprintf(w, `<fieldset class="entry-mode-panel wide" data-entry-mode-panel="range"%s><legend>Start and end</legend><p>%s</p><div class="entry-duration-grid"><label>Start<input type="datetime-local" name="start" value="%s" data-required="true"></label><label>End<input type="datetime-local" name="end" value="%s" data-required="true"></label><label>Break minutes <span class="field-hint">Deducted from the total duration.</span><input type="number" name="break_minutes" min="0" step="1" inputmode="numeric" value="%s"></label></div></fieldset>`,
		rangeHidden, esc(rangeIntro), esc(prefill.Start), esc(prefill.End), esc(prefill.BreakMinutes))
}

func timesheetEntryMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "range") {
		return "range"
	}
	return "manual"
}

func Reports(user *NavUser, filter domain.ReportFilter, rows []map[string]any, saved []domain.SavedReport, selectors *SelectorData) templ.Component {
	return Layout("Reports", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		group := defaultVal(filter.Group, "user")
		pageHeader(w, "Reports", "Billing and review", "Answer the questions engineering leads and billing admins ask most: who did the work, what was billable, and where projects are burning time.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Filters</h2><p>Build repeatable views for PM review, monthly billing, and discipline performance checks.</p></div></div><form method="get" action="/reports" class="toolbar-form selector-form"><label>Group by<select name="group">%s%s%s%s%s%s</select></label><label>Date from<input type="date" name="begin" value="%s"></label><label>Date to<input type="date" name="end" value="%s"></label>`,
			reportOption(group, "user", "Engineer"), reportOption(group, "customer", "Client"), reportOption(group, "project", "Project"), reportOption(group, "activity", "Deliverable"), reportOption(group, "task", "Task"), reportOption(group, "group", "Group"), dateInput(filter.Begin), dateInput(filter.End))
		renderSelect(w, "Client", "customer_id", optionList(selectors, "customer"), filter.CustomerID, false, "All clients", nil)
		renderSelect(w, "Project", "project_id", optionList(selectors, "project"), filter.ProjectID, false, "All projects", map[string]string{"data-filter-parent": "customer_id", "data-filter-attr": "customer-id"})
		renderSelect(w, "Deliverable", "activity_id", optionList(selectors, "activity"), filter.ActivityID, false, "All deliverables", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "Task", "task_id", optionList(selectors, "task"), filter.TaskID, false, "All tasks", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "Engineer", "user_id", optionList(selectors, "user"), filter.UserID, false, "All engineers", nil)
		renderSelect(w, "Group", "group_id", optionList(selectors, "group"), filter.GroupID, false, "All groups", nil)
		renderBillableSelect(w, filter.Billable)
		_, _ = fmt.Fprintf(w, `<button class="primary">Apply</button></form><form method="post" action="/reports/saved" class="toolbar-form"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Saved report name" required><input type="hidden" name="group" value="%s"><input type="hidden" name="begin" value="%s"><input type="hidden" name="end" value="%s"><input type="hidden" name="customer_id" value="%s"><input type="hidden" name="project_id" value="%s"><input type="hidden" name="activity_id" value="%s"><input type="hidden" name="task_id" value="%s"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="group_id" value="%s"><input type="hidden" name="billable" value="%s"><button class="table-action">Save report</button></form></section>`,
			esc(user.CSRF), esc(group), dateInput(filter.Begin), dateInput(filter.End), idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID), idValue(filter.TaskID), idValue(filter.UserID), idValue(filter.GroupID), esc(billableValue(filter.Billable)))
		renderSavedReportsDropdown(w, user, saved)
		_, _ = fmt.Fprint(w, `<div class="tabs" aria-label="Report groups">`)
		reportTab(w, group, "user", "Engineer")
		reportTab(w, group, "customer", "Client")
		reportTab(w, group, "project", "Project")
		reportTab(w, group, "activity", "Deliverable")
		reportTab(w, group, "task", "Task")
		reportTab(w, group, "group", "Group")
		_, _ = fmt.Fprint(w, `</div>`)
		out := [][]string{}
		for _, row := range rows {
			out = append(out, []string{fmt.Sprint(row["name"]), fmt.Sprint(row["count"]), duration(row["seconds"].(int64)), money(row["cents"].(int64))})
		}
		dataTable(w, []string{reportHeading(group), "Entries", "Duration", "Revenue"}, out)
		_, _ = fmt.Fprintf(w, `<div class="export-row"><a class="ghost-button" href="/reports/export?group=%s&begin=%s&end=%s&customer_id=%s&project_id=%s&activity_id=%s&task_id=%s&user_id=%s&group_id=%s&billable=%s">Export CSV</a></div>`,
			esc(group), dateInput(filter.Begin), dateInput(filter.End),
			idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID),
			idValue(filter.TaskID), idValue(filter.UserID), idValue(filter.GroupID), esc(billableValue(filter.Billable)))
		return nil
	}))
}

func ProjectDashboard(user *NavUser, d domain.ProjectDashboard, selectors *SelectorData) templ.Component {
	return Layout("Project dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, d.Project.Name, "Project dashboard", "Monitor delivery burn, invoice readiness, and who is doing the work.")
		_, _ = fmt.Fprint(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/projects">Back to projects</a>`)
		if user.Permissions["manage_master_data"] {
			_, _ = fmt.Fprintf(w, `<a class="ghost-button" href="/projects/%d/edit">Edit project</a>`, d.Project.ID)
		}
		_, _ = fmt.Fprintf(w, `<a class="ghost-button" href="/projects/%d/members">Edit members</a>`, d.Project.ID)
		if user.Permissions["manage_projects"] {
			_, _ = fmt.Fprintf(w, `<a class="ghost-button" href="/projects/%d/workstreams">Edit workstreams</a>`, d.Project.ID)
		}
		_, _ = fmt.Fprint(w, `</div>`)
		renderProjectDashboardBody(w, d, selectors, fmt.Sprintf("/projects/%d/dashboard", d.Project.ID))
		return nil
	}))
}

func ProjectDashboards(user *NavUser, projects []SelectOption, selectedProjectID int64, dashboard *domain.ProjectDashboard, selectors *SelectorData) templ.Component {
	return Layout("Project dashboards", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Project dashboards", "Projects / Delivery", "Open a project dashboard directly and switch between project views without going back to the projects list.")
		_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Select project</h2><p>Choose a project to open its delivery dashboard.</p></div></div><form method="get" action="/project-dashboards" class="toolbar-form selector-form">`)
		renderSelect(w, "Project", "project_id", projects, selectedProjectID, true, "Select a project", nil)
		_, _ = fmt.Fprint(w, `<button class="primary">Open dashboard</button></form></section>`)
		if dashboard == nil {
			if len(projects) == 0 {
				_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No projects available</strong><span>Create a project first, then return here to inspect its dashboard.</span></div>`)
				return nil
			}
			rows := make([][]string, 0, len(projects))
			for _, project := range projects {
				rows = append(rows, []string{project.Label, `<a class="table-action" href="/project-dashboards?project_id=` + esc(fmt.Sprint(project.Value)) + `">Open dashboard</a>`})
			}
			dataTableRaw(w, []string{"Project", "Action"}, rows)
			return nil
		}
		renderProjectDashboardBody(w, *dashboard, selectors, "/project-dashboards")
		return nil
	}))
}

func renderProjectDashboardBody(w io.Writer, d domain.ProjectDashboard, selectors *SelectorData, filterAction string) {
	filter := d.Filter
	resetHref := fmt.Sprintf("/projects/%d/dashboard", d.Project.ID)
	if filterAction == "/project-dashboards" {
		resetHref = fmt.Sprintf("/project-dashboards?project_id=%d", d.Project.ID)
	}
	_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Project effort filters</h2><p>Keep chart and contribution views in sync while narrowing the project lens.</p></div></div><form method="get" action="%s" class="toolbar-form selector-form">`, esc(filterAction))
	_, _ = fmt.Fprintf(w, `<input type="hidden" name="project_id" value="%d">`, d.Project.ID)
	_, _ = fmt.Fprintf(w, `<label>Date from<input type="date" name="begin" value="%s"></label><label>Date to<input type="date" name="end" value="%s"></label>`, dateInput(filter.Begin), dateInput(filter.End))
	renderSelect(w, "Workstream", "workstream_id", optionList(selectors, "workstream"), filter.WorkstreamID, false, "All workstreams", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-ids"})
	renderSelect(w, "Deliverable", "activity_id", optionList(selectors, "activity"), filter.ActivityID, false, "All deliverables", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
	renderSelect(w, "Task", "task_id", optionList(selectors, "task"), filter.TaskID, false, "All tasks", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
	renderSelect(w, "Engineer", "user_id", optionList(selectors, "user"), filter.UserID, false, "All engineers", nil)
	renderSelect(w, "Group", "group_id", optionList(selectors, "group"), filter.GroupID, false, "All groups", nil)
	_, _ = fmt.Fprintf(w, `<button class="primary">Apply filters</button><a class="ghost-button" href="%s">Reset</a></form></section>`, esc(resetHref))

	_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
	metric(w, "Tracked", duration(d.TrackedSeconds), "Total project time")
	metric(w, "Estimate used", fmt.Sprintf("%d%%", d.EstimatePercent), "Against estimate")
	metric(w, "Billable value", money(d.BillableCents), "Tracked value")
	metric(w, "Budget used", fmt.Sprintf("%d%%", d.BudgetPercent), "Against fixed fee")
	metric(w, "Unbilled time", duration(d.UnbilledSeconds), "Billable hours not yet exported")
	metric(w, "Unbilled value", money(d.UnbilledCents), "Potential invoice value")
	metric(w, "Billable split", duration(d.BillableSeconds), "Direct billable effort")
	metric(w, "Internal split", duration(d.NonBillableSeconds), "Non-billable effort")
	_, _ = fmt.Fprint(w, `</section>`)
	if d.Alert {
		_, _ = fmt.Fprint(w, `<div class="alert">This project is near or over its estimate or budget threshold. Review burn before approving more work or sending the next invoice.</div>`)
	}
	renderProjectDashboardPieCharts(w, d)
	_, _ = fmt.Fprint(w, `<section class="section-spacer"><details class="panel collapsible-create-card" open><summary class="panel-head collapsible-create-summary"><div><h2>Active task mix</h2><p>Where the project effort is landing now.</p></div><span class="collapse-indicator collapse-indicator-section" aria-hidden="true"></span></summary><div class="collapsible-panel-content">`)
	taskRows := [][]string{}
	for _, task := range d.TaskSummaries {
		taskRows = append(taskRows, []string{esc(task.Name), esc(duration(task.TrackedSeconds)), esc(duration(task.UnbilledSeconds)), humanBillable(task.Billable)})
	}
	dataTableRaw(w, []string{"Task", "Tracked", "Unbilled", "Billable"}, taskRows)
	_, _ = fmt.Fprint(w, `</div></details><details class="panel collapsible-create-card section-spacer" open><summary class="panel-head collapsible-create-summary"><div><h2>Key contributors by category</h2><p>Who is carrying the billable load on this project.</p></div><span class="collapse-indicator collapse-indicator-section" aria-hidden="true"></span></summary><div class="collapsible-panel-content">`)
	contributorRows := [][]string{}
	for _, contributor := range d.Contributors {
		contributorRows = append(contributorRows, []string{contributor.DisplayName, duration(contributor.TrackedSeconds), duration(contributor.BillableSeconds)})
	}
	dataTable(w, []string{"Engineer", "Tracked", "Billable"}, contributorRows)
	_, _ = fmt.Fprint(w, `</div></details></section>`)
}

type dashboardPieSlice struct {
	Label string
	Value int64
	Color string
}

func renderProjectDashboardPieCharts(w io.Writer, d domain.ProjectDashboard) {
	_, _ = fmt.Fprint(w, `<section class="panel section-spacer"><div class="panel-head"><div><h2>Captured time distribution</h2><p>Fast pie summaries of where project effort is concentrated.</p></div></div><div class="project-dashboard-pies">`)
	renderProjectDashboardPieCard(
		w,
		"Time per workstream",
		"How total project time is distributed across assigned workstreams.",
		dashboardSlicesFromWorkstreams(d.WorkstreamSummaries),
		duration(d.TrackedSeconds),
	)
	renderProjectDashboardPieCard(
		w,
		"Time per user",
		"Share of tracked time by contributor on this project.",
		dashboardSlicesFromContributors(d.Contributors),
		duration(d.TrackedSeconds),
	)
	renderProjectDashboardPieCard(
		w,
		"Time per deliverable",
		"Distribution of tracked project time across deliverables.",
		dashboardSlicesFromActivities(d.ActivitySummaries),
		duration(d.TrackedSeconds),
	)
	_, _ = fmt.Fprint(w, `</div></section>`)
}

func dashboardSlicesFromWorkstreams(items []domain.ProjectWorkstreamSummary) []dashboardPieSlice {
	if len(items) == 0 {
		return []dashboardPieSlice{{Label: "No tracked time", Value: 0, Color: "#c8d7e6"}}
	}
	out := make([]dashboardPieSlice, 0, len(items))
	for i, item := range items {
		out = append(out, dashboardPieSlice{Label: item.Name, Value: item.TrackedSeconds, Color: dashboardPieColor(i)})
	}
	return out
}

func dashboardSlicesFromContributors(items []domain.ProjectContributorSummary) []dashboardPieSlice {
	if len(items) == 0 {
		return []dashboardPieSlice{{Label: "No tracked time", Value: 0, Color: "#c8d7e6"}}
	}
	out := make([]dashboardPieSlice, 0, len(items))
	for i, item := range items {
		out = append(out, dashboardPieSlice{Label: item.DisplayName, Value: item.TrackedSeconds, Color: dashboardPieColor(i)})
	}
	return out
}

func dashboardSlicesFromActivities(items []domain.ProjectActivitySummary) []dashboardPieSlice {
	if len(items) == 0 {
		return []dashboardPieSlice{{Label: "No tracked time", Value: 0, Color: "#c8d7e6"}}
	}
	out := make([]dashboardPieSlice, 0, len(items))
	for i, item := range items {
		out = append(out, dashboardPieSlice{Label: item.Name, Value: item.TrackedSeconds, Color: dashboardPieColor(i)})
	}
	return out
}

func dashboardPieColor(index int) string {
	palette := []string{"#2f80ed", "#14b8a6", "#d97706", "#16a34a", "#ef4444", "#2563eb", "#0f766e", "#9333ea"}
	if len(palette) == 0 {
		return "#2f80ed"
	}
	return palette[index%len(palette)]
}

func renderProjectDashboardPieCard(w io.Writer, title, description string, slices []dashboardPieSlice, totalText string) {
	total := int64(0)
	for _, slice := range slices {
		if slice.Value > 0 {
			total += slice.Value
		}
	}

	const radius = 42.0
	const circumference = 2 * 3.141592653589793 * radius

	_, _ = fmt.Fprintf(w, `<article class="panel project-dashboard-pie"><div class="panel-head"><div><h2>%s</h2><p>%s</p></div></div><div class="project-dashboard-pie-body">`, esc(title), esc(description))
	_, _ = fmt.Fprint(w, `<div class="project-dashboard-pie-visual" role="img" aria-label="Project dashboard pie chart">`)
	_, _ = fmt.Fprint(w, `<svg viewBox="0 0 120 120" aria-hidden="true">`)
	_, _ = fmt.Fprintf(w, `<circle cx="60" cy="60" r="%.2f" fill="none" stroke="var(--line)" stroke-width="16"></circle>`, radius)

	if total > 0 {
		offset := 0.0
		for _, slice := range slices {
			if slice.Value <= 0 {
				continue
			}
			portion := float64(slice.Value) / float64(total)
			dash := portion * circumference
			gap := circumference - dash
			_, _ = fmt.Fprintf(w, `<circle cx="60" cy="60" r="%.2f" fill="none" stroke="%s" stroke-width="16" stroke-linecap="butt" stroke-dasharray="%.4f %.4f" stroke-dashoffset="%.4f" transform="rotate(-90 60 60)"></circle>`, radius, slice.Color, dash, gap, -offset)
			offset += dash
		}
	}

	_, _ = fmt.Fprint(w, `</svg>`)
	_, _ = fmt.Fprintf(w, `<div class="project-dashboard-pie-center"><strong>%s</strong><small>Total</small></div>`, esc(totalText))
	_, _ = fmt.Fprint(w, `</div><ul class="project-dashboard-pie-legend">`)

	for _, slice := range slices {
		percent := "0%"
		if total > 0 && slice.Value > 0 {
			percent = fmt.Sprintf("%.0f%%", (float64(slice.Value)/float64(total))*100)
		}
		_, _ = fmt.Fprintf(w, `<li><span class="project-dashboard-pie-dot" style="background:%s"></span><span>%s</span><strong>%s</strong><small>%s</small></li>`, slice.Color, esc(slice.Label), esc(duration(slice.Value)), esc(percent))
	}

	_, _ = fmt.Fprint(w, `</ul></div></article>`)
}

func Calendar(user *NavUser, weekStart time.Time, entries []domain.Timesheet, selectors *SelectorData, editable map[int64]bool) templ.Component {
	return Layout("Calendar", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Calendar", "Weekly review", "Review where the week went, spot missing days, and jump straight into backfill.")
		prev := weekStart.AddDate(0, 0, -7).Format("2006-01-02")
		next := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
		_, _ = fmt.Fprintf(w, `<div class="tabs"><a class="tab-link" href="/calendar?date=%s">Previous week</a><a class="tab-link active" aria-current="page" href="/calendar?date=%s">%s</a><a class="tab-link" href="/calendar?date=%s">Next week</a></div><section class="calendar-stack">`,
			esc(prev), esc(weekStart.Format("2006-01-02")),
			esc(weekStart.Format("Jan 2")+" – "+weekStart.AddDate(0, 0, 6).Format("Jan 2 2006")),
			esc(next))
		today := time.Now().UTC()
		for day := 0; day < 7; day++ {
			current := weekStart.AddDate(0, 0, day)
			var dayEntries []domain.Timesheet
			total := int64(0)
			for _, entry := range entries {
				if sameDay(entry.StartedAt, current) {
					dayEntries = append(dayEntries, entry)
					total += entry.DurationSeconds
				}
			}
			isWeekend := current.Weekday() == time.Saturday || current.Weekday() == time.Sunday
			isMissing := total == 0 && !current.After(today) && !isWeekend
			dayClass := "calendar-day-row"
			if isMissing {
				dayClass += " calendar-day-missing"
			}
			if isWeekend {
				dayClass += " calendar-day-weekend"
			}
			_, _ = fmt.Fprintf(w, `<article class="%s"><div class="calendar-day-info"><strong class="calendar-day-name">%s</strong><span class="calendar-day-total">%s</span>`,
				esc(dayClass), esc(current.Format("Mon 02 Jan")), esc(duration(total)))
			if isMissing {
				_, _ = fmt.Fprintf(w, `<a class="calendar-log-link" href="/timesheets?date=%s">Log time</a>`, esc(current.Format("2006-01-02")))
			} else if !isWeekend {
				_, _ = fmt.Fprintf(w, `<a class="calendar-log-link" href="/timesheets?date=%s">Add more time</a>`, esc(current.Format("2006-01-02")))
			}
			_, _ = fmt.Fprint(w, `</div><div class="calendar-day-entries">`)
			if len(dayEntries) == 0 {
				if !isWeekend {
					_, _ = fmt.Fprint(w, `<span class="calendar-no-entries">No entries</span>`)
				}
			} else {
				for _, entry := range dayEntries {
					_, _ = fmt.Fprintf(w, `<div class="calendar-entry-row"><span class="entry-time">%s</span><span class="entry-label">%s</span><span class="entry-duration">%s</span>`,
						esc(timeRange(entry)),
						esc(calendarEntryLabel(entry, selectors)),
						esc(duration(entry.DurationSeconds)))
					if entry.Description != "" {
						_, _ = fmt.Fprintf(w, `<span class="entry-desc">%s</span>`, esc(entry.Description))
					}
					if editable[entry.ID] {
						_, _ = fmt.Fprintf(w, `<span class="entry-actions"><a class="table-action small" href="%s">Edit</a></span>`, esc(timesheetEditHref(entry.ID)))
					}
					_, _ = fmt.Fprint(w, `</div>`)
				}
			}
			_, _ = fmt.Fprint(w, `</div></article>`)
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func Account(user *NavUser, account domain.User, notice Notice) templ.Component {
	return Layout("Account", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Account", "Profile and security", "Keep your local profile, timezone, password, and workspace context up to date.")
		renderNotice(w, notice)
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Profile</h2><p>Name and local display preferences.</p></div></div><form method="post" action="/account" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Display name<input name="display_name" value="%s" required></label><label>Email<input value="%s" disabled></label>`, esc(user.CSRF), esc(account.DisplayName), esc(account.Email))
		renderTimezoneSelect(w, "Timezone", "timezone", account.Timezone, false)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save profile</button></div></form></div>`)
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Email address</h2><p>Send a one-time code to a new address before it becomes active.</p></div></div><form method="post" action="/account/email" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Current email<input value="%s" disabled></label><label>New email<input name="new_email" type="email" autocomplete="email" required></label><div class="form-actions"><button class="primary">Send verification code</button><a class="ghost-button" href="/account/email/verify">Enter code</a></div></form></div>`, esc(user.CSRF), esc(account.Email))
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Workspace</h2><p>Your current workspace and how to switch.</p></div></div><div class="form-grid">`)
		renderWorkspaceSwitcher(w, user)
		if len(user.Workspaces) <= 1 {
			_, _ = fmt.Fprintf(w, `<p class="workspace-account-name">%s</p>`, esc(user.CurrentWorkspaceName))
		}
		_, _ = fmt.Fprint(w, `</div></div>`)
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Password</h2><p>Change your password for local authentication.</p></div></div><form method="post" action="/account/password" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="username" autocomplete="username" value="%s"><label>Current password<input name="current_password" type="password" autocomplete="current-password" required></label><label>New password<input name="password" type="password" minlength="8" autocomplete="new-password" required></label><label>Confirm password<input name="confirm" type="password" minlength="8" autocomplete="new-password" required></label><div class="form-actions"><button class="primary">Update password</button></div></form></div></section>`, esc(user.CSRF), esc(account.Email))
		_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Two-factor authentication</h2><p>Two-factor settings are managed from the admin security page.</p></div></div><div class="form-actions"><a class="ghost-button" href="/admin/two-factor">Open two-factor settings</a></div></section>`)
		return nil
	}))
}

func AdminTwoFactor(user *NavUser, account domain.User, totpMode string, emailConfigured bool, setupSecret, setupURI, qrDataURI string, recoveryCodes []string, notice Notice) templ.Component {
	return Layout("Two-factor authentication", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Two-factor authentication", "Administration", "Manage second-factor sign-in methods for your account.")
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/account">Back to account settings</a></div>`)
		_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Two-factor authentication</h2><p>Add a second layer of security at login. Choose one method below.</p></div></div>`)
		if totpMode == "disabled" {
			_, _ = fmt.Fprint(w, `<p>Two-factor authentication is disabled for this deployment.</p>`)
		} else {
			_, _ = fmt.Fprint(w, `<div class="twofa-method"><div class="panel-head"><div><h3>Authenticator app</h3><p>Scan a QR code or enter the setup key in your authenticator app.</p></div></div>`)
			if account.TOTPEnabled {
				_, _ = fmt.Fprintf(w, `<p><span class="badge success">Enabled</span></p><form method="post" action="/account/totp/disable" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Disable authenticator app</button></form>`, esc(user.CSRF))
			} else {
				_, _ = fmt.Fprintf(w, `<p><span class="badge muted">Not enabled</span></p>`)
				if setupSecret != "" {
					_, _ = fmt.Fprint(w, `<div class="totp-setup">`)
					if qrDataURI != "" {
						_, _ = fmt.Fprintf(w, `<div class="totp-qr"><img src="%s" alt="Scan with your authenticator app" width="200" height="200"></div>`, esc(qrDataURI))
					}
					_, _ = fmt.Fprintf(w, `<div class="totp-key-block"><label>Setup key <span class="field-hint">Type this into your app if you cannot scan</span><input class="totp-key" readonly value="%s" onclick="this.select()"></label></div></div>`, esc(setupSecret))
				}
				_, _ = fmt.Fprintf(w, `<form method="post" action="/account/totp/enable" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="secret" value="%s"><label>Verification code <span class="field-hint">Enter the 6-digit code from your app to confirm</span><input name="code" inputmode="numeric" autocomplete="one-time-code" placeholder="000000" required></label><div class="form-actions"><button class="primary">Enable authenticator app</button></div></form>`, esc(user.CSRF), esc(setupSecret))
			}
			_, _ = fmt.Fprint(w, `</div>`)
			if emailConfigured {
				_, _ = fmt.Fprint(w, `<div class="twofa-method twofa-method-border"><div class="panel-head"><div><h3>Email sign-in code</h3><p>Receive a one-time code by email each time you sign in.</p></div></div>`)
				if account.EmailOTPEnabled {
					_, _ = fmt.Fprintf(w, `<p><span class="badge success">Enabled</span></p><form method="post" action="/account/email-otp/disable" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Disable email codes</button></form>`, esc(user.CSRF))
				} else {
					_, _ = fmt.Fprintf(w, `<p><span class="badge muted">Not enabled</span></p><form method="post" action="/account/email-otp/enable" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="primary">Enable email codes</button></form>`, esc(user.CSRF))
				}
				_, _ = fmt.Fprint(w, `</div>`)
			}
		}
		if len(recoveryCodes) > 0 {
			_, _ = fmt.Fprint(w, `<div class="recovery-box"><strong>Recovery codes</strong><p>Save these now. They are shown once.</p><code>`)
			for _, code := range recoveryCodes {
				_, _ = fmt.Fprintf(w, `%s<br>`, esc(code))
			}
			_, _ = fmt.Fprint(w, `</code></div>`)
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func VerifyEmail(user *NavUser, pendingEmail string, expires time.Time, notice Notice) templ.Component {
	return Layout("Verify email", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Verify Email", "Profile and security", "Enter the one-time code sent to your new email address.")
		renderNotice(w, notice)
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Email verification</h2><p>Codes expire after 10 minutes and stop working after too many attempts.</p></div></div>`)
		if pendingEmail == "" {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No pending email change</strong><span>Start a new email change from your account page.</span></div><div class="form-actions"><a class="ghost-button" href="/account">Back to account</a></div></section>`)
			return nil
		}
		_, _ = fmt.Fprintf(w, `<p class="field-hint">Pending email: <strong>%s</strong>. Expires %s.</p><form method="post" action="/account/email/verify" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Verification code<input name="code" inputmode="numeric" autocomplete="one-time-code" required></label><div class="form-actions"><button class="primary">Verify email</button><a class="ghost-button" href="/account">Back</a></div></form></section>`, esc(pendingEmail), esc(expires.Format("15:04 MST")), esc(user.CSRF))
		return nil
	}))
}

func AdminHome(user *NavUser) templ.Component {
	return Layout("Admin", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Admin", "Administration", "Administration stays separate from day-to-day engineering time capture. Open the area you need without cluttering the main workflow.")
		links := adminLinksFor(user)
		if len(links) == 0 {
			_, _ = fmt.Fprint(w, `<section class="panel"><div class="empty-state"><strong>No admin access</strong><span>Your account does not currently have any admin capabilities.</span></div></section>`)
			return nil
		}
		_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>Area</th><th>Purpose</th><th></th></tr></thead><tbody>`)
		_, _ = fmt.Fprint(w, `<tr><td class="muted-cell">Account settings</td><td>Update your personal profile, password, and account preferences.</td><td><a class="table-action" href="/account">Open</a></td></tr>`)
		for _, item := range links {
			_, _ = fmt.Fprintf(w, `<tr><td class="muted-cell">%s</td><td>%s</td><td><a class="table-action" href="%s">Open</a></td></tr>`,
				esc(item.Label), esc(adminDescription(item.Path)), esc(item.Path))
		}
		_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		return nil
	}))
}

func AdminDemoData(user *NavUser, workspaceName string, notice Notice) templ.Component {
	return Layout("Demo data", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Demo data", "Administration", "Show or hide the reusable demo dataset in your default workspace for evaluations and walkthroughs.")
		renderNotice(w, notice)
		targetWorkspace := workspaceName
		if strings.TrimSpace(targetWorkspace) == "" {
			targetWorkspace = "Not found"
		}
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Default workspace target</h2><p>These controls only affect the default workspace for your organization.</p></div></div><p class="field-hint">Current target: <strong>%s</strong></p><div class="actions-row"><form method="post" action="/admin/demo-data/add"><input type="hidden" name="csrf" value="%s"><button class="primary">Show demo data</button></form><form method="post" action="/admin/demo-data/remove" data-confirm="Hide demo data from the default workspace?"><input type="hidden" name="csrf" value="%s"><button class="danger">Hide demo data</button></form></div><p class="field-hint">Showing demo data is repeatable and will refresh the seeded evaluation dataset for that workspace.</p></section>`, esc(targetWorkspace), esc(user.CSRF), esc(user.CSRF))
		return nil
	}))
}

func ProjectMembers(user *NavUser, project domain.Project, members []domain.ProjectMember, users []domain.User, assignedGroups []domain.Group, groups []domain.Group) templ.Component {
	return Layout("Project members", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		userLabels := userLabelMap(users)
		groupLabels := groupLabelMap(groups)
		pageHeader(w, project.Name, "Project access", "Control who can log time to this project and who can manage project setup.")
		_, _ = fmt.Fprintf(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/projects/%d/dashboard">Back to project</a></div>`, project.ID)
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Add members</h2><p>Managers can administer this project; members can track time.</p></div></div><form method="post" action="/projects/%d/members" class="form-grid"><input type="hidden" name="csrf" value="%s"><label class="wide">Users<select name="user_id" multiple size="8">`, project.ID, esc(user.CSRF))
		for _, u := range users {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, u.ID, esc(userLabel(u)))
		}
		_, _ = fmt.Fprintf(w, `</select></label><label>Role<select name="role"><option value="member">Member</option><option value="manager">Manager</option></select></label><div class="form-actions"><button class="primary">Add selected</button></div></form></div><div class="panel form-panel"><div class="panel-head"><div><h2>Add groups</h2><p>Groups make bulk access easier to maintain.</p></div></div><form method="post" action="/projects/%d/groups" class="form-grid"><input type="hidden" name="csrf" value="%s"><label class="wide">Groups<select name="group_id" multiple size="8">`, project.ID, esc(user.CSRF))
		for _, g := range groups {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, g.ID, esc(g.Name))
		}
		_, _ = fmt.Fprint(w, `</select></label><div class="form-actions"><button class="primary">Add selected groups</button></div></form></div></section>`)
		memberRows := [][]string{}
		for _, member := range members {
			memberRows = append(memberRows, []string{fmt.Sprintf(`<label class="check-inline"><input type="checkbox" name="user_id" value="%d"> %s</label>`, member.UserID, esc(label(userLabels, member.UserID))), string(member.Role)})
		}
		_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/%d/members/remove" data-confirm="Remove selected project members?"><input type="hidden" name="csrf" value="%s">`, project.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"User", "Role"}, memberRows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected members</button></div></form>`)
		groupRows := [][]string{}
		for _, group := range assignedGroups {
			groupRows = append(groupRows, []string{fmt.Sprintf(`<label class="check-inline"><input type="checkbox" name="group_id" value="%d"> %s</label>`, group.ID, esc(label(groupLabels, group.ID)))})
		}
		_, _ = fmt.Fprint(w, `<div class="section-spacer"></div>`)
		_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/%d/groups/remove" data-confirm="Remove selected project groups?"><input type="hidden" name="csrf" value="%s">`, project.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"Group"}, groupRows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected groups</button></div></form>`)
		return nil
	}))
}

func WorkspaceAdmin(user *NavUser, workspaces []domain.WorkspaceSummary) templ.Component {
	return Layout("Workspaces", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Workspaces", "Organization admin", "Review workspace health, member counts, and workspace-level setup without leaving the organization view.")
		renderCreateCardStart(w, "Create workspace", "Only organization admins can add workspaces.", true)
		_, _ = fmt.Fprintf(w, `<form method="post" action="/admin/workspaces" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>Slug <span class="field-hint">URL-safe ID, auto-generated if blank</span><input name="slug" placeholder="e.g. my-team"></label><label>Billing unit <span class="field-hint">ISO 4217 code — e.g. ZAR (rand·cent), USD (dollar·cent), EUR (euro·cent)</span><input name="default_currency" value="ZAR" placeholder="ZAR" maxlength="3"></label>`, esc(user.CSRF))
		renderTimezoneSelect(w, "Timezone", "timezone", "UTC", true)
		_, _ = fmt.Fprint(w, `<label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Create workspace</button></div></form>`)
		renderCreateCardEnd(w, true)
		rows := [][]string{}
		for _, workspace := range workspaces {
			status := "Active"
			if workspace.Archived {
				status = "Archived"
			}
			rows = append(rows, []string{workspace.Name, workspace.Slug, status, fmt.Sprint(workspace.MemberCount), fmt.Sprint(workspace.ProjectCount), fmt.Sprintf(`<a class="table-action" href="/admin/workspaces/%d">Manage</a>`, workspace.ID)})
		}
		dataTableRaw(w, []string{"Name", "Slug", "Status", "Members", "Projects", "Action"}, rows)
		return nil
	}))
}

func WorkspaceDetail(user *NavUser, workspace domain.Workspace, members []domain.WorkspaceMember, users []domain.User) templ.Component {
	return Layout("Workspace admin", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, workspace.Name, "Workspace admin", "Manage workspace settings, member roles, and reporting access cleanly.")
		checked := ""
		if workspace.Archived {
			checked = " checked"
		}
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Settings</h2><p>Changes apply only inside this organization.</p></div></div><form method="post" action="/admin/workspaces/%d" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Slug <span class="field-hint">URL-safe ID</span><input name="slug" value="%s" required></label><label>Billing unit <span class="field-hint">ISO 4217 code — e.g. ZAR (rand·cent), USD (dollar·cent), EUR (euro·cent)</span><input name="default_currency" value="%s" placeholder="ZAR" maxlength="3"></label>`, workspace.ID, esc(user.CSRF), esc(workspace.Name), esc(workspace.Slug), esc(workspace.DefaultCurrency))
		renderTimezoneSelect(w, "Timezone", "timezone", workspace.Timezone, true)
		_, _ = fmt.Fprintf(w, `<label class="wide">Description<textarea name="description">%s</textarea></label><label class="check"><input type="checkbox" name="archived"%s> Archived</label><div class="form-actions"><button class="primary">Save workspace</button></div></form></div>`, esc(workspace.Description), checked)
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Add or update member</h2><p>Member: track time · Analyst: track time + view reports · Admin: full workspace management</p></div></div><form method="post" action="/admin/workspaces/%d/members" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>User<select name="user_id">`, workspace.ID, esc(user.CSRF))
		for _, user := range users {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, user.ID, esc(userLabel(user)))
		}
		_, _ = fmt.Fprint(w, `</select></label><label>Role<select name="role"><option value="member">Member</option><option value="analyst">Analyst</option><option value="admin">Admin</option></select></label><div class="form-actions"><button class="primary">Save member</button></div></form></div></section>`)
		rows := [][]string{}
		for _, member := range members {
			status := "Active"
			if !member.Enabled {
				status = "Disabled"
			}
			rows = append(rows, []string{member.DisplayName, member.Email, string(member.Role), status, fmt.Sprint(member.GroupCount), fmt.Sprint(member.ProjectMemberCount), fmt.Sprintf(`<form method="post" action="/admin/workspaces/%d/members/remove" data-confirm="Remove this workspace member?"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="user_id" value="%d"><button class="table-action">Remove</button></form>`, workspace.ID, esc(user.CSRF), member.UserID)})
		}
		dataTableRaw(w, []string{"Name", "Email", "Role", "Status", "Groups", "Projects", "Action"}, rows)
		return nil
	}))
}

func GroupMembers(user *NavUser, group domain.Group, members []domain.User, users []domain.User) templ.Component {
	return Layout("Group members", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		memberLabels := userLabelMap(members)
		pageHeader(w, group.Name, "Group membership", "Use groups for team-level project assignment and bulk access changes.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add members</h2><p>Select one or more workspace users.</p></div></div><form method="post" action="/groups/%d/members" class="form-grid"><input type="hidden" name="csrf" value="%s"><label class="wide">Users<select name="user_id" multiple size="10">`, group.ID, esc(user.CSRF))
		for _, item := range users {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, item.ID, esc(userLabel(item)))
		}
		_, _ = fmt.Fprint(w, `</select></label><div class="form-actions"><button class="primary">Add selected</button></div></form></section>`)
		rows := [][]string{}
		for _, member := range members {
			rows = append(rows, []string{fmt.Sprintf(`<label class="check-inline"><input type="checkbox" name="user_id" value="%d"> %s</label>`, member.ID, esc(label(memberLabels, member.ID))), member.Email})
		}
		_, _ = fmt.Fprintf(w, `<form method="post" action="/groups/%d/members/remove" data-confirm="Remove selected group members?"><input type="hidden" name="csrf" value="%s">`, group.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"Member", "Email"}, rows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected</button></div></form>`)
		return nil
	}))
}

func ProjectTemplates(user *NavUser, templates []domain.ProjectTemplate, selectors *SelectorData) templ.Component {
	return Layout("Project templates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Project templates", "Project setup", "Create repeatable engineering project blueprints with standard phases, tasks, and default controls.")
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How templates work:</strong> Capture your standard project setup once, including typical deliverables and task bundles, then reuse it whenever a similar engineering job starts.</div>`)
		renderCreateCardStart(w, "Create template", "Define defaults including tasks and activities (one per line).", true)
		renderProjectTemplateForm(w, user, domain.ProjectTemplate{Visible: true, Billable: true, BudgetAlertPercent: 80}, "/project-templates")
		renderCreateCardEnd(w, true)
		rows := [][]string{}
		for _, template := range templates {
			status := "Active"
			if template.Archived {
				status = "Archived"
			}
			var editForm strings.Builder
			renderProjectTemplateForm(&editForm, user, template, fmt.Sprintf("/project-templates/%d", template.ID))
			actions := `<details class="inline-edit"><summary class="table-action">Edit</summary><div class="inline-edit-form inline-edit-generic">` + editForm.String() + `<div class="inline-edit-actions"><button class="ghost-button small" type="button" data-close-details>Cancel</button></div></div></details>`
			rows = append(rows, []string{
				template.Name,
				template.ProjectName,
				status,
				fmt.Sprint(len(template.Tasks)),
				fmt.Sprint(len(template.Activities)),
				actions,
			})
		}
		dataTableRaw(w, []string{"Template", "Project name", "Status", "Tasks", "Activities", "Action"}, rows)
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel section-spacer"><div class="panel-head"><div><h2>Use template</h2><p>Creates a new project in this workspace with all tasks and activities from the template.</p></div></div><form method="post" action="/project-templates/use" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Template<select name="template_id">`, esc(user.CSRF))
		for _, template := range templates {
			if !template.Archived {
				_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, template.ID, esc(template.Name))
			}
		}
		_, _ = fmt.Fprint(w, `</select></label>`)
		renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), 0, true, "Select a customer", nil)
		_, _ = fmt.Fprint(w, `<label>Project name<input name="project_name" placeholder="Leave blank to use template default"></label><div class="form-actions"><button class="primary">Create project</button></div></form></section>`)
		return nil
	}))
}

func ProjectTemplateDetail(user *NavUser, template domain.ProjectTemplate, selectors *SelectorData) templ.Component {
	return Layout("Edit template", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, template.Name, "Project template", "Refine a repeatable project setup or create a live project from this template.")
		_, _ = fmt.Fprint(w, `<section class="panel form-panel">`)
		renderProjectTemplateForm(w, user, template, fmt.Sprintf("/project-templates/%d", template.ID))
		_, _ = fmt.Fprintf(w, `</section><section class="panel form-panel section-spacer"><div class="panel-head"><div><h2>Use template</h2><p>Create a project from this template.</p></div></div><form method="post" action="/project-templates/%d/use" class="form-grid"><input type="hidden" name="csrf" value="%s">`, template.ID, esc(user.CSRF))
		renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), 0, true, "Select a customer", nil)
		_, _ = fmt.Fprintf(w, `<label>Project name<input name="project_name" value="%s"></label><div class="form-actions"><button class="primary">Create project</button></div></form></section>`, esc(template.ProjectName))
		return nil
	}))
}

func renderProjectTemplateForm(w io.Writer, user *NavUser, template domain.ProjectTemplate, action string) {
	archiveChecked := ""
	if template.Archived {
		archiveChecked = " checked"
	}
	visibleChecked := ""
	if template.Visible || template.ID == 0 {
		visibleChecked = " checked"
	}
	privateChecked := ""
	if template.Private {
		privateChecked = " checked"
	}
	billableChecked := ""
	if template.Billable || template.ID == 0 {
		billableChecked = " checked"
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="%s" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Template name<input name="name" value="%s" required></label><label>Project name<input name="project_name" value="%s" required></label><label>Project ID <span class="field-hint">leave blank to auto-generate</span><input name="project_number" value="%s"></label><label>Order number<input name="order_number" value="%s"></label><label>Estimate hours<input name="estimate_hours" value="%d"></label><label>Budget <span class="field-hint">in primary currency units</span><input name="budget" value="%d" placeholder="e.g. 10000"></label><label>Budget alert (%%) <span class="field-hint">warn when this %% is consumed</span><input name="budget_alert_percent" value="%d"></label><label class="wide">Description<textarea name="description">%s</textarea></label><label class="wide">Default tasks <span class="field-hint">one task name per line</span><textarea name="tasks" placeholder="One task per line">%s</textarea></label><label class="wide">Default deliverables <span class="field-hint">one deliverable name per line</span><textarea name="activities" placeholder="One deliverable per line">%s</textarea></label><label class="check"><input type="checkbox" name="visible"%s> Visible</label><label class="check"><input type="checkbox" name="private"%s> Private</label><label class="check"><input type="checkbox" name="billable"%s> Billable</label><label class="check"><input type="checkbox" name="archived"%s> Archived</label><div class="form-actions"><button class="primary">Save template</button></div></form>`,
		esc(action), esc(user.CSRF), esc(template.Name), esc(template.ProjectName), esc(template.ProjectNumber), esc(template.OrderNo), template.EstimateSeconds/3600, template.BudgetCents, defaultInt(template.BudgetAlertPercent, 80), esc(template.Description), esc(templateTaskLines(template.Tasks)), esc(templateActivityLines(template.Activities)), visibleChecked, privateChecked, billableChecked, archiveChecked)
}

func Invoices(user *NavUser, invoices []domain.Invoice, selectors *SelectorData) templ.Component {
	return Layout("Invoices", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Invoices", "Billing", "Turn classified project time into invoice records with fewer surprises at billing time.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create invoice</h2><p>Pulls all unexported billable entries for the customer within the date range. Entries are marked as exported once included.</p></div></div><form method="post" action="/invoices" class="toolbar-form"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), 0, true, "Select a customer", nil)
		_, _ = fmt.Fprint(w, `<label>Date from<input type="date" name="begin" required></label><label>Date to<input type="date" name="end" required></label><label>Tax (%)<input name="tax" value="0" placeholder="e.g. 20"></label><button class="primary">Create invoice</button></form></section>`)
		rows := [][]string{}
		for _, inv := range invoices {
			rows = append(rows, []string{inv.Number, label(selectors.CustomerLabels, inv.CustomerID), status(inv.Status), money(inv.TotalCents), fmt.Sprintf(`<a class="table-action" href="/api/invoices/%d/download">Download</a>`, inv.ID)})
		}
		dataTableRaw(w, []string{"Number", "Customer", "Status", "Total", "File"}, rows)
		return nil
	}))
}

func Webhooks(user *NavUser, hooks []domain.WebhookEndpoint) templ.Component {
	return Layout("Webhooks", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Webhooks", "Integrations", "Keep outbound integrations readable and operational for admins only.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add endpoint</h2><p>Use '*' for all events or comma-separated event names.</p></div></div><form method="post" action="/webhooks" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>URL<input name="url" required></label><label>Secret <span class="field-hint">signs each payload — store it securely</span><input name="secret" required></label><label>Events <span class="field-hint">use * for all, or comma-separate names</span><input name="events" value="*"></label><div class="form-actions"><button class="primary">Add webhook</button></div></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, hook := range hooks {
			rows = append(rows, []string{hook.Name, hook.URL, strings.Join(hook.Events, ","), yesNo(hook.Enabled)})
		}
		dataTableRaw(w, []string{"Name", "URL", "Events", "Enabled"}, rows)
		return nil
	}))
}

func UserForm(user *NavUser, target *domain.User) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/admin/users"
		email := ""
		username := ""
		displayName := ""
		timezone := "UTC"
		passwordRequired := ` required`
		passwordLabel := "Password"
		buttonLabel := "Create user"
		enabledChecked := ` checked`
		selectedRoleName := string(domain.RoleUser)
		if target != nil {
			action = fmt.Sprintf("/admin/users/%d", target.ID)
			email = target.Email
			username = target.Username
			displayName = target.DisplayName
			timezone = defaultVal(target.Timezone, "UTC")
			passwordRequired = ""
			passwordLabel = `Password <span class="field-hint">leave blank to keep the current password</span>`
			buttonLabel = "Save user"
			enabledChecked = checkedIf(target.Enabled)
			selectedRoleName = selectedRole(target.Roles)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Email<input name="email" type="email" value="%s" required></label><label>Username <span class="field-hint">used for login</span><input name="username" value="%s" required></label><label>Display name<input name="display_name" value="%s" required></label><label>%s<input name="password" type="password"%s></label>`, esc(action), esc(user.CSRF), esc(email), esc(username), esc(displayName), passwordLabel, passwordRequired)
		renderTimezoneSelect(w, "Timezone", "timezone", timezone, true)
		_, _ = fmt.Fprintf(w, `<label>Role<select name="role"><option value="user"%s>User</option><option value="teamlead"%s>Team lead</option><option value="admin"%s>Admin</option><option value="superadmin"%s>Super admin</option></select></label><label class="check"><input type="checkbox" name="enabled" value="1"%s> Enabled</label><div class="form-actions"><button class="primary">%s</button></div></form>`, selectedAttr(selectedRoleName == string(domain.RoleUser)), selectedAttr(selectedRoleName == string(domain.RoleTeamLead)), selectedAttr(selectedRoleName == string(domain.RoleAdmin)), selectedAttr(selectedRoleName == string(domain.RoleSuperAdmin)), enabledChecked, buttonLabel)
		if user.Permissions["manage_users"] {
			_, _ = fmt.Fprint(w,
				`<div class="role-guide">`+
					`<h3 class="role-guide-title">What each role can do</h3>`+
					`<div class="role-cards">`+

					`<div class="role-card role-card--user">`+
					`<div class="role-card-header"><span class="role-badge">User</span><span class="role-card-desc">The baseline for every team member.</span></div>`+
					`<ul class="role-features">`+
					`<li><a href="/timesheets">Log &amp; edit time entries</a></li>`+
					`<li><a href="/calendar">Calendar view</a></li>`+
					`<li><a href="/">Dashboard &amp; live timer</a></li>`+
					`<li><a href="/tags">Create and manage tags</a></li>`+
					`<li>REST API access</li>`+
					`<li><a href="/account">Manage own account &amp; 2FA</a></li>`+
					`</ul></div>`+

					`<div class="role-card role-card--teamlead">`+
					`<div class="role-card-header"><span class="role-badge">Team lead</span><span class="role-card-desc">Reporting visibility across assigned projects.</span></div>`+
					`<ul class="role-features">`+
					`<li class="role-inherit">Everything a User can do</li>`+
					`<li><a href="/reports">Reports, saved filters &amp; sharing</a></li>`+
					`<li><a href="/reports/utilization">Team utilization dashboard</a></li>`+
					`<li><a href="/reports/export">Export time data as CSV</a></li>`+
					`<li>Scoped to projects they are assigned to</li>`+
					`</ul></div>`+

					`<div class="role-card role-card--admin">`+
					`<div class="role-card-header"><span class="role-badge">Admin</span><span class="role-card-desc">Full workspace management.</span></div>`+
					`<ul class="role-features">`+
					`<li class="role-inherit">Everything a Team lead can do</li>`+
					`<li><a href="/customers">Customers</a></li>`+
					`<li><a href="/projects">Projects</a> — created via <a href="/projects/create">guided 4-step wizard</a></li>`+
					`<li><a href="/workstreams">Workstreams</a> &amp; per-project workstream assignments</li>`+
					`<li><a href="/activities">Deliverables</a> &amp; <a href="/tasks">tasks</a></li>`+
					`<li><a href="/rates">Billing rates</a> · <a href="/admin/exchange-rates">exchange rates</a> · <a href="/admin/recalculate">recalculation</a></li>`+
					`<li><a href="/invoices">Invoices &amp; exports</a></li>`+
					`<li><a href="/groups">Groups</a> &amp; <a href="/webhooks">webhooks</a></li>`+
					`<li><a href="/admin/users">Create &amp; manage users</a></li>`+
					`</ul></div>`+

					`<div class="role-card role-card--superadmin">`+
					`<div class="role-card-header"><span class="role-badge">Super admin</span><span class="role-card-desc">Organization-wide control.</span></div>`+
					`<ul class="role-features">`+
					`<li class="role-inherit">Everything an Admin can do</li>`+
					`<li><a href="/admin/workspaces">Create &amp; manage workspaces</a></li>`+
					`<li><a href="/admin/workspaces">Assign workspace members &amp; roles</a></li>`+
					`<li><a href="/admin/workspaces">Organization-level settings &amp; ownership</a></li>`+
					`</ul></div>`+

					`</div></div>`,
			)
		}
		return nil
	})
}

func ErrorPage(title, message string) templ.Component {
	return Layout(title, nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="login-shell"><div class="login-card"><h1>%s</h1><p>%s</p><a class="table-action" href="/">Return home</a></div></section>`, esc(title), esc(message))
		return nil
	}))
}

func renderWorkSelectors(w io.Writer, selectors *SelectorData, requireCore bool) {
	renderWorkSelectorsWithSelected(w, selectors, requireCore, TimesheetPrefill{})
}

func renderWorkSelectorsWithSelected(w io.Writer, selectors *SelectorData, requireCore bool, selected TimesheetPrefill) {
	renderSelect(w, "Client"+tipHTML("The client that will ultimately be billed for the work."), "customer_id", optionList(selectors, "customer"), selected.CustomerID, requireCore, "Select a client", nil)
	renderSelect(w, "Project"+tipHTML("The project or job number this time should land against."), "project_id", optionList(selectors, "project"), selected.ProjectID, requireCore, "Select a project", map[string]string{"data-filter-parent": "customer_id", "data-filter-attr": "customer-id"})
	renderSelect(w, "Workstream"+tipHTML("Workstream identifies the delivery stream or discipline this effort belongs to within the selected project."), "workstream_id", optionList(selectors, "workstream"), selected.WorkstreamID, requireCore, "Select a workstream", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-ids"})
	renderSelect(w, "Deliverable"+tipHTML("Deliverable explains the kind of effort performed, such as design review, QA/rework, site visit, or coordination."), "activity_id", optionList(selectors, "activity"), selected.ActivityID, requireCore, "Select a deliverable", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
	renderSelect(w, "Task"+tipHTML("Task is the deliverable or work package being billed, such as pump sizing, P&ID review, or tender support."), "task_id", optionList(selectors, "task"), selected.TaskID, false, "No task", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
}

// tipHTML returns an inline tooltip icon span. text is escaped internally.
func tipHTML(text string) string {
	return `<span class="tooltip-icon" data-tooltip="` + esc(text) + `">i</span>`
}

// renderSelect renders a label+select combo. labelHTML is emitted raw — callers
// must only pass trusted string literals or strings assembled with tipHTML/esc.
func renderSelect(w io.Writer, labelHTML, name string, options []SelectOption, selected int64, required bool, placeholder string, attrs map[string]string) {
	requiredAttr := ""
	if required {
		requiredAttr = " required"
	}
	_, _ = fmt.Fprintf(w, `<label>%s<select name="%s"%s`, labelHTML, esc(name), requiredAttr)
	for _, key := range sortedKeys(attrs) {
		value := attrs[key]
		_, _ = fmt.Fprintf(w, ` %s="%s"`, esc(key), esc(value))
	}
	_, _ = fmt.Fprint(w, `>`)
	selectedAttr := ""
	if selected == 0 {
		selectedAttr = " selected"
	}
	_, _ = fmt.Fprintf(w, `<option value=""%s>%s</option>`, selectedAttr, esc(placeholder))
	for _, option := range options {
		selectedAttr = ""
		if option.Value == selected {
			selectedAttr = " selected"
		}
		_, _ = fmt.Fprintf(w, `<option value="%d"%s`, option.Value, selectedAttr)
		for _, key := range sortedKeys(option.Attrs) {
			value := option.Attrs[key]
			if strings.TrimSpace(value) != "" {
				_, _ = fmt.Fprintf(w, ` data-%s="%s"`, esc(key), esc(value))
			}
		}
		_, _ = fmt.Fprintf(w, `>%s</option>`, esc(option.Label))
	}
	_, _ = fmt.Fprint(w, `</select></label>`)
}

func optionList(selectors *SelectorData, kind string) []SelectOption {
	if selectors == nil {
		return nil
	}
	switch kind {
	case "customer":
		return selectors.Customers
	case "project":
		return selectors.Projects
	case "workstream":
		return selectors.Workstreams
	case "activity":
		return selectors.Activities
	case "task":
		return selectors.Tasks
	case "user":
		return selectors.Users
	case "group":
		return selectors.Groups
	default:
		return nil
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func label(labels map[int64]string, id int64) string {
	if labels != nil {
		if value := labels[id]; value != "" {
			return value
		}
	}
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("Unavailable #%d", id)
}

func labelPtr(labels map[int64]string, id *int64) string {
	if id == nil {
		return "Any"
	}
	return label(labels, *id)
}

func userLabel(user domain.User) string {
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = user.Username
	}
	if user.Email != "" && user.Email != name {
		return name + " - " + user.Email
	}
	return name
}

func userLabelMap(users []domain.User) map[int64]string {
	labels := map[int64]string{}
	for _, user := range users {
		labels[user.ID] = userLabel(user)
	}
	return labels
}

func groupLabelMap(groups []domain.Group) map[int64]string {
	labels := map[int64]string{}
	for _, group := range groups {
		labels[group.ID] = group.Name
	}
	return labels
}

func renderNav(w io.Writer, user *NavUser, items []navItem) {
	currentGroup := ""
	groupOpen := false
	groupID := ""
	for _, item := range items {
		if !user.can(item.Permission) {
			continue
		}
		if item.Group != currentGroup {
			if groupOpen {
				_, _ = fmt.Fprint(w, `</div></section>`)
			}
			currentGroup = item.Group
			groupID = "nav-section-" + navGroupID(currentGroup)
			groupOpen = true
			_, _ = fmt.Fprintf(w, `<section class="nav-section" data-nav-section data-nav-group="%s"><button class="nav-group" type="button" data-nav-toggle aria-expanded="true" aria-controls="%s"><span>%s</span><span class="nav-section-icon" aria-hidden="true">▾</span></button><div class="nav-section-items" id="%s">`, esc(navGroupID(currentGroup)), esc(groupID), esc(currentGroup), esc(groupID))
		}
		class := ` class="nav-link"`
		if isActivePath(user.CurrentPath, item.Path) {
			class = ` class="nav-link active" aria-current="page"`
		}
		_, _ = fmt.Fprintf(w, `<a%s href="%s">%s</a>`, class, esc(item.Path), esc(item.Label))
	}
	if groupOpen {
		_, _ = fmt.Fprint(w, `</div></section>`)
	}
}

func navGroupID(group string) string {
	id := strings.ToLower(group)
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func renderAccountDropdown(w io.Writer, user *NavUser) {
	initial := "?"
	name := strings.TrimSpace(user.DisplayName)
	if name != "" {
		initial = strings.ToUpper(string([]rune(name)[0]))
	}
	_, _ = fmt.Fprintf(w, `<div class="dropdown account-menu" data-dropdown="account"><button class="account-trigger" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="account-menu"><span class="avatar" aria-hidden="true">%s</span><span class="account-name">%s</span><span class="chevron" aria-hidden="true">▾</span></button><div class="dropdown-menu dropdown-menu-right" id="account-menu" role="menu" hidden data-dropdown-menu><a role="menuitem" href="/">Dashboard</a><a role="menuitem" href="/timesheets">Timesheets</a><a role="menuitem" href="/account">Account settings</a>`, esc(initial), esc(user.DisplayName))
	if user != nil && len(adminLinksFor(user)) > 0 {
		_, _ = fmt.Fprint(w, `<a role="menuitem" href="/admin">Admin</a>`)
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="/logout" role="none"><input type="hidden" name="csrf" value="%s"><button role="menuitem" type="submit">Logout</button></form></div></div>`, esc(user.CSRF))
}

func renderWorkspaceSwitcher(w io.Writer, user *NavUser) {
	if user == nil || len(user.Workspaces) <= 1 {
		if user != nil && user.CurrentWorkspaceName != "" {
			_, _ = fmt.Fprintf(w, `<span class="workspace-pill">%s</span>`, esc(user.CurrentWorkspaceName))
		}
		return
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="/workspace" class="workspace-switcher"><input type="hidden" name="csrf" value="%s"><label><span class="sr-only">Workspace</span><select name="workspace_id" onchange="this.form.submit()">`, esc(user.CSRF))
	for _, workspace := range user.Workspaces {
		selected := ""
		if workspace.ID == user.CurrentWorkspaceID {
			selected = " selected"
		}
		_, _ = fmt.Fprintf(w, `<option value="%d"%s>%s</option>`, workspace.ID, selected, esc(workspace.Name))
	}
	_, _ = fmt.Fprint(w, `</select></label></form>`)
}

func renderTimezoneSelect(w io.Writer, label, name, selected string, required bool) {
	if selected == "" {
		selected = "UTC"
	}
	requiredAttr := ""
	if required {
		requiredAttr = " required"
	}
	_, _ = fmt.Fprintf(w, `<label>%s<select name="%s"%s>`, esc(label), esc(name), requiredAttr)
	for _, option := range timezoneOptions {
		sel := ""
		if option.Value == selected {
			sel = " selected"
		}
		_, _ = fmt.Fprintf(w, `<option value="%s"%s>%s</option>`, esc(option.Value), sel, esc(option.Label))
	}
	_, _ = fmt.Fprint(w, `</select></label>`)
}

func adminLinksFor(user *NavUser) []navItem {
	if user == nil {
		return nil
	}
	links := make([]navItem, 0, len(adminNav))
	for _, item := range adminNav {
		if item.Path == "/admin" {
			continue
		}
		if user.can(item.Permission) {
			links = append(links, item)
		}
	}
	if len(links) == 0 {
		return nil
	}
	return append([]navItem{{Label: "Admin Home", Path: "/admin", Group: "Administration"}}, append(links, navItem{Label: "Two-factor authentication", Path: "/admin/two-factor", Group: "Administration"})...)
}

func adminSidebarNav(user *NavUser) []navItem {
	return adminLinksFor(user)
}

func isAdminArea(user *NavUser) bool {
	if user == nil {
		return false
	}
	for _, item := range adminSidebarNav(user) {
		if isActivePath(user.CurrentPath, item.Path) {
			return true
		}
	}
	return false
}

func adminDescription(path string) string {
	switch path {
	case "/admin":
		return "Open the administration hub and choose the control area you need."
	case "/account":
		return "Open account settings for profile details, timezone, and password management."
	case "/admin/two-factor":
		return "Configure sign-in two-factor methods including authenticator app and email OTP."
	case "/admin/workspaces":
		return "Create workspaces, update workspace settings, and manage workspace membership."
	case "/admin/email":
		return "Review SMTP configuration, send test email, and set account email-change policy."
	case "/admin/demo-data":
		return "Show or hide the reusable demo dataset in the default workspace for product walkthroughs."
	case "/admin/schedule":
		return "Configure expected working days and hours for utilization and missing-time calculations."
	case "/rates":
		return "Maintain billable rates and user cost rates so invoices and profitability remain defensible."
	case "/admin/exchange-rates":
		return "Manage currency conversion rules used by billing and cross-currency reporting."
	case "/admin/recalculate":
		return "Preview the financial impact before recalculating historical time entries."
	case "/admin/users":
		return "Create users and make sure engineers and admins have the right access."
	case "/webhooks":
		return "Configure signed outbound webhook integrations."
	default:
		return "Open this admin area."
	}
}

type MenuAction struct {
	Label  string
	Href   string
	Method string
}

func RowActionMenu(id, label string, csrf string, actions []MenuAction) string {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, `<div class="dropdown row-menu" data-dropdown="%s"><button class="action-menu-button" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="%s-menu">%s <span class="chevron" aria-hidden="true">▾</span></button><div class="dropdown-menu dropdown-menu-right" id="%s-menu" role="menu" hidden data-dropdown-menu>`, esc(id), esc(id), esc(label), esc(id))
	for _, action := range actions {
		if strings.EqualFold(action.Method, "post") {
			_, _ = fmt.Fprintf(&b, `<form method="post" action="%s" role="none"><input type="hidden" name="csrf" value="%s"><button role="menuitem" type="submit">%s</button></form>`, esc(action.Href), esc(csrf), esc(action.Label))
		} else {
			_, _ = fmt.Fprintf(&b, `<a role="menuitem" href="%s">%s</a>`, esc(action.Href), esc(action.Label))
		}
	}
	_, _ = fmt.Fprint(&b, `</div></div>`)
	return b.String()
}

func renderSavedReportsDropdown(w io.Writer, user *NavUser, saved []domain.SavedReport) {
	if len(saved) == 0 {
		return
	}
	_, _ = fmt.Fprint(w, `<div class="report-actions"><div class="dropdown" data-dropdown="saved-reports"><button class="ghost-button dropdown-trigger" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="saved-reports-menu">Saved reports <span class="chevron" aria-hidden="true">▾</span></button><div class="dropdown-menu" id="saved-reports-menu" role="menu" hidden data-dropdown-menu>`)
	for _, report := range saved {
		shareInfo := ""
		if report.ShareToken != "" {
			shareURL := "/reports/share/" + report.ShareToken
			if report.ShareExpiresAt != nil {
				shareInfo = fmt.Sprintf(` <a class="table-action small" href="%s" target="_blank">Share link</a>`, esc(shareURL))
			}
		}
		_, _ = fmt.Fprintf(w, `<div role="menuitem" class="saved-report-item"><a href="%s" class="saved-report-link"><span>%s</span><small>%s</small></a><div class="saved-report-actions">%s<form method="post" action="/reports/saved/%d/delete" data-confirm="Delete this saved report?"><input type="hidden" name="csrf" value="%s"><button class="danger small">Delete</button></form><form method="post" action="/reports/saved/%d/share"><input type="hidden" name="csrf" value="%s"><input type="number" name="days" value="30" min="1" max="365" style="width:50px"><button class="table-action small">Share</button></form></div></div>`,
			esc(savedReportHref(report)), esc(report.Name), esc(report.GroupBy),
			shareInfo,
			report.ID, esc(user.CSRF),
			report.ID, esc(user.CSRF))
	}
	_, _ = fmt.Fprint(w, `</div></div></div>`)
}

func (u *NavUser) can(permission string) bool {
	if permission == "" {
		return true
	}
	return u != nil && u.Permissions[permission]
}

func isActivePath(currentPath, itemPath string) bool {
	if currentPath == "" {
		return false
	}
	cleanCurrent := path.Clean("/" + strings.TrimPrefix(currentPath, "/"))
	cleanItem := path.Clean("/" + strings.TrimPrefix(itemPath, "/"))
	if cleanItem == "/" {
		return cleanCurrent == "/"
	}
	return cleanCurrent == cleanItem || strings.HasPrefix(cleanCurrent, cleanItem+"/")
}

func reportTab(w io.Writer, current, value, label string) {
	class := ` class="tab-link"`
	currentAttr := ""
	if current == value {
		class = ` class="tab-link active"`
		currentAttr = ` aria-current="page"`
	}
	_, _ = fmt.Fprintf(w, `<a%s%s href="/reports?group=%s">%s</a>`, class, currentAttr, esc(value), esc(label))
}

func renderBillableSelect(w io.Writer, selected *bool) {
	current := billableValue(selected)
	_, _ = fmt.Fprintf(w, `<label>Billing class<select name="billable"><option value=""%s>All work</option><option value="true"%s>Billable only</option><option value="false"%s>Internal only</option></select></label>`,
		selectedAttr(current == ""),
		selectedAttr(current == "true"),
		selectedAttr(current == "false"))
}

func reportOption(current, value, label string) string {
	selected := ""
	if current == value {
		selected = ` selected`
	}
	return fmt.Sprintf(`<option value="%s"%s>%s</option>`, esc(value), selected, esc(label))
}

func renderNotice(w io.Writer, notice Notice) {
	message := strings.TrimSpace(notice.Message)
	if message == "" {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(notice.Kind))
	switch kind {
	case "success", "error", "warning", "info":
	default:
		kind = "info"
	}
	_, _ = fmt.Fprintf(w, `<div class="alert alert-%s">%s</div>`, esc(kind), esc(message))
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func timeRange(entry domain.Timesheet) string {
	if entry.EndedAt == nil {
		return entry.StartedAt.Format("15:04") + " - running"
	}
	return entry.StartedAt.Format("15:04") + " - " + entry.EndedAt.Format("15:04")
}

func savedReportHref(report domain.SavedReport) string {
	values := url.Values{}
	values.Set("group", defaultVal(report.GroupBy, "user"))
	var filters map[string]string
	if err := json.Unmarshal([]byte(report.FiltersJSON), &filters); err == nil {
		for key, value := range filters {
			value = strings.TrimSpace(value)
			if value != "" {
				values.Set(key, value)
			}
		}
	}
	return "/reports?" + values.Encode()
}

func pageHeader(w io.Writer, title, eyebrow, description string) {
	_, _ = fmt.Fprintf(w, `<header class="page-head"><div><span>%s</span><h1>%s</h1><p>%s</p></div></header>`, esc(eyebrow), esc(title), esc(description))
}

func metric(w io.Writer, label, value, hint string) {
	_, _ = fmt.Fprintf(w, `<article class="metric-card"><span>%s</span><strong>%s</strong><small>%s</small></article>`, esc(label), esc(value), esc(hint))
}

func dataTable(w io.Writer, headers []string, rows [][]string) {
	safe := [][]string{}
	for _, row := range rows {
		out := []string{}
		for _, cell := range row {
			out = append(out, esc(cell))
		}
		safe = append(safe, out)
	}
	dataTableRaw(w, headers, safe)
}

func dataTableRaw(w io.Writer, headers []string, rows [][]string) {
	actionIndexes := map[int]bool{}
	for index, header := range headers {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if normalized == "action" || normalized == "actions" {
			actionIndexes[index] = true
		}
	}

	_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr>`)
	for _, header := range headers {
		_, _ = fmt.Fprintf(w, `<th>%s</th>`, esc(header))
	}
	_, _ = fmt.Fprint(w, `</tr></thead><tbody>`)
	if len(rows) == 0 {
		_, _ = fmt.Fprintf(w, `<tr><td colspan="%d"><div class="empty-state"><strong>No records yet</strong><span>Create the first record above.</span></div></td></tr>`, len(headers))
	}
	for _, row := range rows {
		_, _ = fmt.Fprint(w, `<tr>`)
		for index, cell := range row {
			class := ""
			if actionIndexes[index] {
				class = ` class="actions-cell"`
			} else if index == 0 {
				class = ` class="muted-cell"`
			}
			_, _ = fmt.Fprintf(w, `<td%s>%s</td>`, class, cell)
		}
		_, _ = fmt.Fprint(w, `</tr>`)
	}
	_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
}

func esc(value string) string {
	return template.HTMLEscapeString(value)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func money(amount int64) string {
	return fmt.Sprintf("$%.2f", float64(amount))
}

func Money(amount int64) string {
	return money(amount)
}

func dateInput(value *time.Time) string {
	if value == nil {
		return ""
	}
	return esc(value.Format("2006-01-02"))
}

func idValue(value int64) string {
	if value == 0 {
		return ""
	}
	return esc(fmt.Sprint(value))
}

func ptrText(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func duration(seconds int64) string {
	h := seconds / 3600
	m := seconds % 3600 / 60
	return fmt.Sprintf("%dh %02dm", h, m)
}

func humanBillable(value bool) string {
	if value {
		return `<span class="badge success">Billable</span>`
	}
	return `<span class="badge muted">Internal</span>`
}

func humanExport(value bool) string {
	if value {
		return `<span class="badge">Exported</span>`
	}
	return `<span class="badge warning">Pending</span>`
}

func billableValue(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "true"
	}
	return "false"
}

func selectedAttr(value bool) string {
	if value {
		return ` selected`
	}
	return ""
}

func yesNo(value bool) string {
	if value {
		return `<span class="badge success">Yes</span>`
	}
	return `<span class="badge muted">No</span>`
}

func status(value string) string {
	return `<span class="badge">` + esc(value) + `</span>`
}

func val[T any](ptr *T, field string) string {
	if ptr == nil {
		return ""
	}
	switch v := any(ptr).(type) {
	case *domain.Customer:
		switch field {
		case "Name":
			return esc(v.Name)
		case "Company":
			return esc(v.Company)
		case "Email":
			return esc(v.Email)
		case "Currency":
			return esc(v.Currency)
		case "Timezone":
			return esc(v.Timezone)
		case "Number":
			return esc(v.Number)
		case "Comment":
			return esc(v.Comment)
		}
	}
	return ""
}

func defaultVal(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func configuredText(configured bool) string {
	if configured {
		return "configured"
	}
	return "not set"
}

func defaultInt(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}

func templateTaskLines(tasks []domain.ProjectTemplateTask) string {
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if strings.TrimSpace(task.Name) != "" {
			lines = append(lines, task.Name)
		}
	}
	return strings.Join(lines, "\n")
}

func templateActivityLines(activities []domain.ProjectTemplateActivity) string {
	lines := make([]string, 0, len(activities))
	for _, activity := range activities {
		if strings.TrimSpace(activity.Name) != "" {
			lines = append(lines, activity.Name)
		}
	}
	return strings.Join(lines, "\n")
}

func singularTitle(title string) string {
	switch title {
	case "Deliverables":
		return "Deliverable"
	case "Clients":
		return "Client"
	case "Customers":
		return "Customer"
	case "Projects":
		return "Project"
	case "Tags":
		return "Tag"
	case "Rates":
		return "Rate"
	case "Users":
		return "User"
	case "Groups":
		return "Group"
	default:
		return strings.TrimSuffix(title, "s")
	}
}

func bodyClass(title string) string {
	clean := strings.ToLower(strings.TrimSpace(title))
	clean = strings.ReplaceAll(clean, "&", "and")
	clean = strings.ReplaceAll(clean, " ", "-")
	clean = strings.ReplaceAll(clean, "/", "-")
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}
	return "page-" + strings.Trim(clean, "-")
}

func selectedRole(roles []domain.Role) string {
	for _, role := range []domain.Role{domain.RoleSuperAdmin, domain.RoleAdmin, domain.RoleTeamLead, domain.RoleUser} {
		for _, assigned := range roles {
			if assigned == role {
				return string(role)
			}
		}
	}
	return string(domain.RoleUser)
}

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

func reportHeading(group string) string {
	switch group {
	case "user":
		return "Engineer"
	case "customer":
		return "Client"
	case "project":
		return "Project"
	case "activity":
		return "Deliverable"
	case "task":
		return "Task"
	case "group":
		return "Group"
	default:
		return strings.Title(group)
	}
}

func calendarEntryLabel(entry domain.Timesheet, selectors *SelectorData) string {
	parts := []string{label(selectors.ProjectLabels, entry.ProjectID)}
	if entry.WorkstreamID != nil {
		parts = append(parts, label(selectors.WorkstreamLabels, *entry.WorkstreamID))
	}
	parts = append(parts, label(selectors.ActivityLabels, entry.ActivityID))
	if entry.TaskID != nil {
		parts = append(parts, label(selectors.TaskLabels, *entry.TaskID))
	}
	out := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func recentWorkLabel(item domain.DashboardRecentWork, selectors *SelectorData) string {
	parts := []string{
		label(selectors.CustomerLabels, item.CustomerID),
		label(selectors.ProjectLabels, item.ProjectID),
		labelPtr(selectors.WorkstreamLabels, item.WorkstreamID),
		label(selectors.ActivityLabels, item.ActivityID),
	}
	if item.TaskID != nil {
		parts = append(parts, label(selectors.TaskLabels, *item.TaskID))
	}
	out := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " / ")
}

func timesheetEditHref(id int64) string {
	return fmt.Sprintf("/timesheets/%d/edit", id)
}

func timesheetActions(entry domain.Timesheet, editable map[int64]bool) string {
	actions := `<a class="table-action small" href="` + esc(timesheetPrefillHref(domain.DashboardRecentWork{CustomerID: entry.CustomerID, ProjectID: entry.ProjectID, WorkstreamID: entry.WorkstreamID, ActivityID: entry.ActivityID, TaskID: entry.TaskID, Description: entry.Description, StartedAt: entry.StartedAt, DurationSeconds: entry.DurationSeconds, Billable: entry.Billable, Exported: entry.Exported})) + `">Duplicate</a>`
	if editable[entry.ID] {
		actions += ` <a class="table-action small" href="` + esc(timesheetEditHref(entry.ID)) + `">Edit</a>`
	}
	return actions
}

func timesheetPrefillHref(item domain.DashboardRecentWork) string {
	values := url.Values{}
	values.Set("customer_id", fmt.Sprint(item.CustomerID))
	values.Set("project_id", fmt.Sprint(item.ProjectID))
	if item.WorkstreamID != nil {
		values.Set("workstream_id", fmt.Sprint(*item.WorkstreamID))
	}
	values.Set("activity_id", fmt.Sprint(item.ActivityID))
	if item.TaskID != nil {
		values.Set("task_id", fmt.Sprint(*item.TaskID))
	}
	values.Set("date", time.Now().UTC().Format("2006-01-02"))
	if item.Description != "" {
		values.Set("description", item.Description)
	}
	return "/timesheets?" + values.Encode()
}

func favoritePrefillHref(item domain.Favorite) string {
	values := url.Values{}
	values.Set("customer_id", fmt.Sprint(item.CustomerID))
	values.Set("project_id", fmt.Sprint(item.ProjectID))
	values.Set("activity_id", fmt.Sprint(item.ActivityID))
	if item.TaskID != nil {
		values.Set("task_id", fmt.Sprint(*item.TaskID))
	}
	values.Set("date", time.Now().UTC().Format("2006-01-02"))
	if item.Description != "" {
		values.Set("description", item.Description)
	}
	return "/timesheets?" + values.Encode()
}

func projectWatchSummary(item domain.DashboardProjectWatch) string {
	flags := []string{}
	if item.EstimatePercent > 0 {
		flags = append(flags, fmt.Sprintf("%d%% of estimate", item.EstimatePercent))
	}
	if item.BudgetPercent > 0 {
		flags = append(flags, fmt.Sprintf("%d%% of budget", item.BudgetPercent))
	}
	if len(flags) == 0 {
		return "Tracked work is underway"
	}
	return strings.Join(flags, " · ")
}

func projectWatchMeta(item domain.DashboardProjectWatch) string {
	meta := []string{duration(item.TrackedSeconds) + " tracked"}
	if item.UnbilledSeconds > 0 {
		meta = append(meta, duration(item.UnbilledSeconds)+" unbilled")
	}
	if item.UnbilledCents > 0 {
		meta = append(meta, money(item.UnbilledCents)+" pending")
	}
	return strings.Join(meta, " · ")
}

// ─── New feature templates ────────────────────────────────────────────────────

func Tasks(user *NavUser, tasks []domain.Task, selectors *SelectorData, canManage bool) templ.Component {
	return Layout("Tasks", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Tasks", "Work breakdown", "Treat tasks as billable work packages such as calculations, reviews, inspections, coordination, and rework.")
		if canManage {
			_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create task</h2><p>Attach tasks to projects so entries can be reported at work-package level.</p></div></div><form class="form-grid" method="post" action="/tasks"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
			renderSelect(w, "Project", "project_id", optionList(selectors, "project"), 0, true, "Select a project", nil)
			_, _ = fmt.Fprint(w, `<label>Name<input name="name" required></label><label>Task number<input name="number"></label><label>Estimate (hours)<input name="estimate_hours" type="number" min="0" value="0"></label><label class="check"><input type="checkbox" name="visible" value="1" checked> Visible</label><label class="check"><input type="checkbox" name="billable" value="1" checked> Billable</label><div class="form-actions"><button class="primary">Create task</button></div></form></section>`)
		}
		if len(tasks) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No tasks yet</strong><span>Create tasks above or use a project template to add standard work packages.</span></div>`)
		} else {
			_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>Name</th><th>Project</th><th>Number</th><th>Visible</th><th>Billable</th><th>Estimate</th>`)
			if canManage {
				_, _ = fmt.Fprint(w, `<th>Actions</th>`)
			}
			_, _ = fmt.Fprint(w, `</tr></thead><tbody>`)
			for _, task := range tasks {
				if task.Archived {
					continue
				}
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%dh</td>`,
					esc(task.Name),
					esc(label(selectors.ProjectLabels, task.ProjectID)),
					esc(task.Number),
					yesNo(task.Visible), yesNo(task.Billable),
					task.EstimateSeconds/3600)
				if canManage {
					_, _ = fmt.Fprintf(w, `<td class="actions-cell"><details class="inline-edit"><summary class="table-action">Edit</summary><form class="compact-form inline-edit-form" method="post" action="/tasks/%d"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="project_id" value="%d"><div class="inline-edit-col"><label>Number<input name="number" value="%s"></label><label>Estimate hours<input name="estimate_hours" type="number" value="%d"></label><label class="check"><input type="checkbox" name="visible" value="1"%s> Visible</label><label class="check"><input type="checkbox" name="billable" value="1"%s> Billable</label></div><div class="inline-edit-col"><label>Name<input name="name" value="%s" required></label><div class="inline-edit-actions"><button class="primary small">Save</button><button class="ghost-button small" type="button" data-close-details>Cancel</button></div></div></form></details><form method="post" action="/tasks/%d/archive" data-confirm="Archive this task?"><input type="hidden" name="csrf" value="%s"><button class="danger small">Archive</button></form></td>`,
						task.ID, esc(user.CSRF),
						task.ProjectID,
						esc(task.Number),
						task.EstimateSeconds/3600,
						checkedIf(task.Visible), checkedIf(task.Billable),
						esc(task.Name),
						task.ID, esc(user.CSRF))
				}
				_, _ = fmt.Fprint(w, `</tr>`)
			}
			_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		}
		return nil
	}))
}

func checkedIf(v bool) string {
	if v {
		return " checked"
	}
	return ""
}

func SharedReport(name string, rows []map[string]any) templ.Component {
	return Layout(name+" — Shared Report", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="shared-report-shell"><div class="login-card shared-report-card"><h1>%s</h1><div class="table-scroll"><table><thead><tr><th>Name</th><th>Entries</th><th>Hours</th><th>Amount</th></tr></thead><tbody>`, esc(name))
		for _, row := range rows {
			sec, _ := row["seconds"].(int64)
			cents, _ := row["cents"].(int64)
			count, _ := row["count"].(int64)
			_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
				esc(fmt.Sprintf("%v", row["name"])), count, duration(sec), money(cents))
		}
		if len(rows) == 0 {
			_, _ = fmt.Fprint(w, `<tr><td colspan="4"><div class="empty-state"><strong>No shared report data</strong><span>This shared report has no rows for the selected filters.</span></div></td></tr>`)
		}
		_, _ = fmt.Fprint(w, `</tbody></table></div></div></section>`)
		return nil
	}))
}

func Utilization(user *NavUser, rows []domain.UtilizationRow, beginStr, endStr string) templ.Component {
	return Layout("Utilization", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Utilization", "Capacity review", "Compare expected effort to captured time. Spot missing hours early and understand billable vs non-billable split.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Period</h2><p>Filter utilization by the date range used for expected-hours comparison.</p></div></div><form class="toolbar-form" method="get" action="/reports/utilization"><label>From<input type="date" name="begin" value="%s"></label><label>To<input type="date" name="end" value="%s"></label><button class="primary">Apply</button>`, esc(beginStr), esc(endStr))
		_, _ = fmt.Fprintf(w, `<a class="ghost-button" href="/reports/export?begin=%s&end=%s">Export CSV</a></form></section>`, esc(beginStr), esc(endStr))
		if len(rows) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No utilization data</strong><span>No time entries match the selected period yet.</span></div>`)
		} else {
			// Summary totals
			var totalExpected, totalCaptured, totalBillable, totalMissing int64
			for _, r := range rows {
				totalExpected += r.ExpectedSeconds
				totalCaptured += r.TotalSeconds
				totalBillable += r.BillableSeconds
				if r.MissingSeconds > 0 {
					totalMissing += r.MissingSeconds
				}
			}
			overallPct := int64(0)
			if totalExpected > 0 {
				overallPct = 100 * totalCaptured / totalExpected
			}
			_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
			metric(w, "Expected", duration(totalExpected), "Total expected for period")
			metric(w, "Captured", duration(totalCaptured), "Total logged time")
			metric(w, "Missing", duration(totalMissing), "Hours not yet captured")
			metric(w, "Utilization", fmt.Sprintf("%d%%", overallPct), "Captured vs expected")
			_, _ = fmt.Fprint(w, `</section>`)
			_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table class="utilization-table"><thead><tr><th>User</th><th>Expected</th><th>Captured</th><th>Missing</th><th>Billable</th><th>Non-billable</th><th>Utilization</th><th>Amount</th></tr></thead><tbody>`)
			for _, r := range rows {
				pct := int64(0)
				if r.ExpectedSeconds > 0 {
					pct = 100 * r.TotalSeconds / r.ExpectedSeconds
				} else if r.TotalSeconds > 0 {
					pct = 100
				}
				missingClass := ""
				if r.MissingSeconds > 0 {
					missingClass = ` class="warn-cell"`
				}
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td%s>%s</td><td>%s</td><td>%s</td><td><div class="util-bar-wrap"><div class="util-bar" style="width:%d%%"></div><span>%d%%</span></div></td><td>%s</td></tr>`,
					esc(r.DisplayName),
					duration(r.ExpectedSeconds),
					duration(r.TotalSeconds),
					missingClass, duration(r.MissingSeconds),
					duration(r.BillableSeconds),
					duration(r.NonBillableSeconds),
					min64(pct, 100), pct,
					money(r.EntryCents))
			}
			_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		}
		return nil
	}))
}

func ExchangeRates(user *NavUser, rates []domain.ExchangeRate) templ.Component {
	return Layout("Exchange Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Exchange Rates", "Administration", "Convert billing amounts across billing units for multi-currency reporting.")
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>Rate format:</strong> Enter the multiplier as a decimal. For example, 0.085 means 1 ZAR converts to 0.085 USD. The most recently effective rate for each pair is applied automatically.</div>`)
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add exchange rate</h2><p>Use ISO 4217 currency codes and an effective date.</p></div></div><form class="form-grid" method="post" action="/admin/exchange-rates"><input type="hidden" name="csrf" value="%s"><label>From unit <span class="field-hint">ISO 4217, e.g. ZAR</span><input name="from_currency" maxlength="3" placeholder="ZAR" required></label><label>To unit <span class="field-hint">ISO 4217, e.g. USD</span><input name="to_currency" maxlength="3" placeholder="USD" required></label><label>Rate<input name="rate" type="number" step="0.000001" min="0.000001" placeholder="0.920" required></label><label>Effective from<input name="effective_from" type="date" required></label><div class="form-actions"><button class="primary">Add rate</button></div></form></section>`, esc(user.CSRF))
		if len(rates) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No exchange rates defined</strong><span>Add a rate above when projects need cross-currency reporting.</span></div>`)
		} else {
			_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>From</th><th>To</th><th>Rate</th><th>Effective</th><th>Action</th></tr></thead><tbody>`)
			for _, r := range rates {
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%.4f</td><td>%s</td><td><form method="post" action="/admin/exchange-rates/%d/delete"><input type="hidden" name="csrf" value="%s"><button class="danger small">Delete</button></form></td></tr>`,
					esc(r.FromCurrency), esc(r.ToCurrency), float64(r.RateThousandths)/1000.0, esc(r.EffectiveFrom.Format("2006-01-02")), r.ID, esc(user.CSRF))
			}
			_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		}
		return nil
	}))
}

func Workstreams(user *NavUser, items []domain.Workstream) templ.Component {
	return Layout("Workstreams", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Workstreams", "Projects / Delivery", "Define discipline or phase categories (e.g. Civil, Mechanical, Electrical) that can be assigned to projects.")
		renderCreateCardStart(w, "Create workstream", "Add discipline or phase categories to assign to projects.", true)
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/workstreams"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required placeholder="e.g. Civil Engineering"></label><label class="wide">Description<textarea name="description" placeholder="Optional description"></textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><div class="form-actions"><button class="primary">Save workstream</button></div></form>`, esc(user.CSRF))
		renderCreateCardEnd(w, true)
		if len(items) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No workstreams yet</strong><span>Create the first discipline or phase category above.</span></div>`)
			return nil
		}
		_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>Name</th><th>ID</th><th>Description</th><th>Visible</th><th>Actions</th></tr></thead><tbody>`)
		for _, ws := range items {
			_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td class="muted-cell">%s</td><td>%s</td><td>%s</td><td class="actions-cell"><details class="inline-edit"><summary class="table-action">Edit</summary><form class="compact-form inline-edit-form" method="post" action="/workstreams/%d"><input type="hidden" name="csrf" value="%s"><div class="inline-edit-col"><label>ID<input name="code" value="%s"></label><label>Name<input name="name" value="%s" required></label><label class="check"><input type="checkbox" name="visible"%s> Visible</label></div><div class="inline-edit-col"><label>Description<textarea name="description">%s</textarea></label><div class="inline-edit-actions"><button class="primary small">Save</button><button class="ghost-button small" type="button" data-close-details>Cancel</button></div></div></form></details><form method="post" action="/workstreams/%d/delete" data-confirm="Delete workstream?"><input type="hidden" name="csrf" value="%s"><button class="danger small">Delete</button></form></td></tr>`,
				esc(ws.Name), esc(ws.Code), esc(ws.Description), yesNo(ws.Visible), ws.ID, esc(user.CSRF), esc(ws.Code), esc(ws.Name), checkedIf(ws.Visible), esc(ws.Description), ws.ID, esc(user.CSRF))
		}
		_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		return nil
	}))
}

func ProjectWorkstreams(user *NavUser, project *domain.Project, assigned []domain.ProjectWorkstream, all []domain.Workstream) templ.Component {
	return Layout("Project Workstreams", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Workstreams — "+project.Name, "Projects / Delivery", "Assign workstreams to this project and optionally allocate a budget per discipline.")
		_, _ = fmt.Fprintf(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/projects/%d/dashboard">Back to project</a></div>`, project.ID)
		assignedIDs := map[int64]bool{}
		for _, pw := range assigned {
			assignedIDs[pw.WorkstreamID] = true
		}
		var opts []SelectOption
		for _, ws := range all {
			if !assignedIDs[ws.ID] {
				opts = append(opts, SelectOption{Value: ws.ID, Label: ws.Name})
			}
		}
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Assign workstream</h2></div></div><form class="form-grid" method="post" action="/projects/%d/workstreams"><input type="hidden" name="csrf" value="%s">`, project.ID, esc(user.CSRF))
		renderSelect(w, "Workstream", "workstream_id", opts, 0, true, "Select workstream", nil)
		_, _ = fmt.Fprint(w, `<label>Budget <span class="field-hint">in primary currency units</span><input name="budget" value="0" placeholder="e.g. 5000"></label><div class="form-actions"><button class="primary">Assign</button></div></form></section>`)
		if len(assigned) == 0 {
			_, _ = fmt.Fprint(w, `<p class="empty-state">No workstreams assigned yet.</p>`)
			return nil
		}
		_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>Workstream</th><th>Budget</th><th>Active</th><th></th></tr></thead><tbody>`)
		for _, pw := range assigned {
			_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td><form method="post" action="/projects/%d/workstreams/%d/remove" data-confirm="Remove workstream?"><input type="hidden" name="csrf" value="%s"><button class="table-action danger-action">Remove</button></form></td></tr>`,
				esc(pw.WorkstreamName), money(pw.BudgetCents), yesNo(pw.Active), project.ID, pw.WorkstreamID, esc(user.CSRF))
		}
		_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		return nil
	}))
}

func WorkScheduleSettings(user *NavUser, schedule domain.WorkSchedule) templ.Component {
	return Layout("Work Schedule", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Work Schedule", "Administration", "Configure expected working days and hours per day for utilization and missing-time calculations.")
		workingDaySet := map[time.Weekday]bool{}
		for _, d := range schedule.WorkingDaysOfWeek {
			workingDaySet[d] = true
		}
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Configure schedule</h2><p>These settings determine how Tockr calculates expected hours and missing time per day, week, month and year.</p></div></div><form class="form-grid" method="post" action="/admin/schedule"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		_, _ = fmt.Fprint(w, `<div class="wide"><label>Working days</label><div class="day-checkboxes">`)
		type dayDef struct {
			name string
			abbr string
			wd   time.Weekday
		}
		days := []dayDef{
			{"Monday", "mon", time.Monday}, {"Tuesday", "tue", time.Tuesday}, {"Wednesday", "wed", time.Wednesday},
			{"Thursday", "thu", time.Thursday}, {"Friday", "fri", time.Friday}, {"Saturday", "sat", time.Saturday}, {"Sunday", "sun", time.Sunday},
		}
		for _, d := range days {
			checked := ""
			if workingDaySet[d.wd] {
				checked = " checked"
			}
			_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="day_%s" value="1"%s> %s</label>`, d.abbr, checked, d.name)
		}
		_, _ = fmt.Fprint(w, `</div></div>`)
		_, _ = fmt.Fprintf(w, `<label>Hours per day<input type="number" name="hours_per_day" value="%.1f" min="1" max="24" step="0.5" required></label>`, schedule.WorkingHoursPerDay)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save schedule</button></div></form></section>`)
		return nil
	}))
}

func EmailSettings(user *NavUser, smtp SMTPSettingsView, settings domain.EmailSettings, notice Notice) templ.Component {
	return Layout("Email Settings", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Email", "Administration", "Review SMTP delivery configuration and account email policy.")
		renderNotice(w, notice)
		status := `<span class="badge success">Ready</span>`
		if !smtp.Valid {
			status = `<span class="badge warning">Needs configuration</span>`
		}
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>SMTP transport</h2><p>Provider credentials are environment-backed and are not editable in the browser.</p></div>%s</div><div class="summary-list">`, status)
		rows := [][2]string{
			{"TOCKR_SMTP_HOST", smtp.Host},
			{"TOCKR_SMTP_PORT", fmt.Sprint(smtp.Port)},
			{"TOCKR_SMTP_FROM", smtp.From},
			{"TOCKR_SMTP_USERNAME", configuredText(smtp.UsernameConfigured)},
			{"TOCKR_SMTP_PASSWORD", configuredText(smtp.PasswordConfigured)},
			{"TOCKR_SMTP_STARTTLS", fmt.Sprint(smtp.StartTLS)},
			{"TOCKR_PUBLIC_URL", defaultVal(smtp.PublicURL, "derived from request")},
		}
		for _, row := range rows {
			_, _ = fmt.Fprintf(w, `<div><span>%s</span><strong>%s</strong></div>`, esc(row[0]), esc(row[1]))
		}
		if smtp.Error != "" {
			_, _ = fmt.Fprintf(w, `<div><span>Configuration check</span><strong>%s</strong></div>`, esc(smtp.Error))
		}
		_, _ = fmt.Fprint(w, `</div></div>`)
		checked := ""
		if settings.NotifyOldEmailOnChange {
			checked = " checked"
		}
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Email policy</h2><p>These app settings are stored in Tockr.</p></div></div><form method="post" action="/admin/email" class="form-grid"><input type="hidden" name="csrf" value="%s"><label class="check"><input type="checkbox" name="notify_old_email"%s> Notify old address after an email change</label><div class="form-actions"><button class="primary">Save email settings</button></div></form></div>`, esc(user.CSRF), checked)
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Test delivery</h2><p>Sends through the configured SMTP server.</p></div></div><form method="post" action="/admin/email/test" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Recipient<input name="to" type="email" required></label><div class="form-actions"><button class="primary">Send test email</button></div></form></div>`, esc(user.CSRF))
		_, _ = fmt.Fprint(w, `<div class="panel"><div class="panel-head"><div><h2>Local testing</h2><p>Use the compose mail catcher for development: SMTP on port 1025 and inbox at http://localhost:8025.</p></div></div></div></section>`)
		return nil
	}))
}

func Recalculate(user *NavUser, preview []domain.RecalcPreviewRow, selectors *SelectorData, projectID int64, sinceStr string, notice Notice) templ.Component {
	return Layout("Recalculate Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Retroactive Rate Recalculation", "Administration", "Preview the financial impact before updating historical timesheet rates.")
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How to use this screen:</strong> Choose a project and a start date, then click <strong>Preview</strong> to see which time entries have a different rate than the one currently configured. Entries already included in an exported invoice are flagged — recalculating them updates the stored value but will not change any invoice already sent. Click <strong>Apply</strong> only when you have reviewed all changes.</div>`)
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Preview scope</h2><p>Select one project and the earliest date to evaluate.</p></div></div><form class="toolbar-form selector-form" method="get" action="/admin/recalculate">`)
		renderSelect(w, "Project", "project_id", optionList(selectors, "project"), projectID, true, "Select a project", nil)
		_, _ = fmt.Fprintf(w, `<label>Since<input type="date" name="since" value="%s"></label><button class="primary">Preview</button></form></section>`, esc(sinceStr))
		if len(preview) == 0 {
			_, _ = fmt.Fprint(w, `<div class="empty-state"><strong>No recalculation needed</strong><span>No timesheets need recalculation for the selected filters.</span></div>`)
		} else {
			var deltaTotal int64
			_, _ = fmt.Fprint(w, `<div class="info-callout warn">The entries below have a different rate than the one currently configured. <strong>Exported entries are flagged</strong> — updating them will not change any invoice already sent, but will affect future exports.</div>`)
			_, _ = fmt.Fprint(w, `<section class="table-card"><div class="table-scroll"><table><thead><tr><th>Date</th><th>Description</th><th>Current ¢/h</th><th>New ¢/h</th><th>Delta ¢</th><th>Exported</th></tr></thead><tbody>`)
			for _, row := range preview {
				deltaTotal += row.DeltaCents
				exportedBadge := `<span class="badge muted">No</span>`
				if row.Exported {
					exportedBadge = `<span class="badge warning">Yes</span>`
				}
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
					esc(row.StartedAt.Format("2006-01-02")), esc(row.Description),
					money(row.CurrentRateCents), money(row.ResolvedRateCents), money(row.DeltaCents), exportedBadge)
			}
			_, _ = fmt.Fprintf(w, `</tbody><tfoot><tr><td colspan="4"><strong>Total delta</strong></td><td><strong>%s</strong></td><td></td></tr></tfoot></table></div></section>`, money(deltaTotal))
			_, _ = fmt.Fprintf(w, `<form method="post" action="/admin/recalculate"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="project_id" value="%d"><input type="hidden" name="since" value="%s"><div class="form-actions"><button class="primary danger">Apply Recalculation (%d timesheets)</button></div></form>`,
				esc(user.CSRF), projectID, esc(sinceStr), len(preview))
		}
		return nil
	}))
}
