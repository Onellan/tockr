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

type SelectOption struct {
	Value int64
	Label string
	Attrs map[string]string
}

type SelectorData struct {
	Customers  []SelectOption
	Projects   []SelectOption
	Activities []SelectOption
	Tasks      []SelectOption
	Users      []SelectOption
	Groups     []SelectOption

	CustomerLabels map[int64]string
	ProjectLabels  map[int64]string
	ActivityLabels map[int64]string
	TaskLabels     map[int64]string
	UserLabels     map[int64]string
	GroupLabels    map[int64]string
}

var primaryNav = []navItem{
	{"Dashboard", "/", "Work", ""},
	{"Timesheets", "/timesheets", "Work", auth.PermTrackTime},
	{"Calendar", "/calendar", "Work", auth.PermTrackTime},
	{"Customers", "/customers", "Manage", ""},
	{"Projects", "/projects", "Manage", ""},
	{"Tasks", "/tasks", "Manage", ""},
	{"Activities", "/activities", "Manage", ""},
	{"Tags", "/tags", "Manage", auth.PermTrackTime},
	{"Groups", "/groups", "Manage", auth.PermManageGroups},
	{"Reports", "/reports", "Analyze", auth.PermViewReports},
	{"Utilization", "/reports/utilization", "Analyze", auth.PermViewReports},
	{"Invoices", "/invoices", "Analyze", auth.PermManageInvoices},
}

var adminNav = []navItem{
	{"Workspaces", "/admin/workspaces", "Admin", auth.PermManageOrg},
	{"Rates", "/rates", "Admin", auth.PermManageRates},
	{"Exchange Rates", "/admin/exchange-rates", "Admin", auth.PermManageRates},
	{"Recalculate", "/admin/recalculate", "Admin", auth.PermManageRates},
	{"Templates", "/project-templates", "Admin", auth.PermManageProjects},
	{"Users", "/admin/users", "Admin", auth.PermManageUsers},
	{"Webhooks", "/webhooks", "Admin", auth.PermManageWebhooks},
}

func Layout(title string, user *NavUser, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>%s</title><link rel=\"icon\" href=\"/favicon.ico?v=20260422\" sizes=\"any\"><link rel=\"icon\" type=\"image/png\" sizes=\"32x32\" href=\"/static/favicon-32x32.png?v=20260422\"><link rel=\"icon\" type=\"image/png\" sizes=\"16x16\" href=\"/static/favicon-16x16.png?v=20260422\"><link rel=\"apple-touch-icon\" sizes=\"180x180\" href=\"/static/apple-touch-icon.png?v=20260422\"><link rel=\"manifest\" href=\"/static/site.webmanifest?v=20260422\"><meta name=\"theme-color\" content=\"#0f766e\"><link rel=\"stylesheet\" href=\"/static/style.css?v=20260422-selectors\"><script src=\"/static/menu.js?v=20260422-navfix\" defer></script></head><body>", esc(title))
		if user == nil {
			_, _ = fmt.Fprint(w, `<main class="auth-main">`)
			if err := body.Render(ctx, w); err != nil {
				return err
			}
			_, _ = fmt.Fprint(w, `</main></body></html>`)
			return nil
		}
		_, _ = fmt.Fprint(w, `<a class="skip-link" href="#main-content">Skip to content</a><div class="app-shell"><aside class="sidebar" aria-label="Application navigation"><a class="brand" href="/" aria-label="Tockr dashboard"><span class="brand-mark">T</span><span><strong>Tockr</strong><small>Time operations</small></span></a><nav class="side-nav" aria-label="Primary navigation">`)
		renderNav(w, user, primaryNav)
		renderNav(w, user, adminNav)
		_, _ = fmt.Fprintf(w, `</nav></aside><div class="workspace"><header class="topbar"><div><span class="topbar-kicker">Workspace</span><strong>%s</strong></div><div class="account-area">`, esc(title))
		renderAccountDropdown(w, user)
		_, _ = fmt.Fprint(w, `</div></header><main class="content" id="main-content" tabindex="-1">`)
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</main></div></div></body></html>`)
		return nil
	})
}

func Login(message string) templ.Component {
	return Layout("Login", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Know exactly where your team's time goes — and bill every hour with confidence.</p><ul><li>Capture time before it slips away</li><li>Turn hours into accurate, ready-to-send invoices</li><li>Keep projects on budget with real-time visibility</li></ul></div><form method="post" action="/login" class="login-card"><div><h2>Welcome back</h2><p>Sign in to your account.</p></div>`)
		if message != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		_, _ = fmt.Fprint(w, `<label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><label>Two-factor code <input name="totp" inputmode="numeric" autocomplete="one-time-code" placeholder="Only if enabled"></label><button class="primary full">Login</button></form></section>`)
		return nil
	}))
}

func Dashboard(user *NavUser, stats map[string]int64, active *domain.Timesheet, favorites []domain.Favorite, selectors *SelectorData) templ.Component {
	return Layout("Dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Dashboard", "Live overview", "Track current work, unexported time, and billing movement.")
		_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
		metric(w, "Active timers", fmt.Sprint(stats["active_timers"]), "Running entries")
		metric(w, "Today", duration(stats["today_seconds"]), "Tracked by you")
		metric(w, "Unexported", fmt.Sprint(stats["unexported"]), "Not yet invoiced")
		metric(w, "Invoices", fmt.Sprint(stats["invoices"]), "Created documents")
		_, _ = fmt.Fprint(w, `</section><section class="two-col"><div class="panel"><div class="panel-head"><div><h2>Timer <span class="tooltip-icon" data-tooltip="Starts a live timer against the selected project and activity. When you stop it, a timesheet entry is created automatically.">i</span></h2><p>Start or stop the active work entry.</p></div></div>`)
		if active != nil {
			_, _ = fmt.Fprintf(w, `<div class="timer-running"><span class="status-dot"></span><div><strong>Running since %s</strong><p>Timer is active for this user.</p></div></div><form method="post" action="/timesheets/stop" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Stop timer</button></form>`, esc(active.StartedAt.Format("15:04")), esc(user.CSRF))
		} else {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/timesheets/start" class="compact-form selector-form"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
			renderWorkSelectors(w, selectors, true)
			_, _ = fmt.Fprint(w, `<input name="description" placeholder="What are you working on?"><button class="primary">Start timer</button></form>`)
		}
		_, _ = fmt.Fprintf(w, `</div><div class="panel"><div class="panel-head"><div><h2>Favorites <span class="tooltip-icon" data-tooltip="Save a preset combination of project, activity and task. Click Start next to any favorite to launch a timer with those settings instantly.">i</span></h2><p>Start repeated work without retyping IDs.</p></div></div><form method="post" action="/favorites" class="compact-form selector-form"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Name" required>`, esc(user.CSRF))
		renderWorkSelectors(w, selectors, true)
		_, _ = fmt.Fprint(w, `<input name="description" placeholder="Description"><input name="tags" placeholder="Tags"><button class="table-action">Save</button></form><div class="summary-list">`)
		if len(favorites) == 0 {
			_, _ = fmt.Fprint(w, `<div><span>No favorites yet</span><strong>Create one from repeated work</strong></div>`)
		}
		for _, fav := range favorites {
			_, _ = fmt.Fprintf(w, `<div class="favorite-row"><span class="fav-name">%s</span><div class="fav-actions"><form method="post" action="/favorites/%d/start"><input type="hidden" name="csrf" value="%s"><button class="table-action">Start</button></form><details class="dropdown inline-dropdown"><summary class="table-action">Edit</summary><form method="post" action="/favorites/%d" class="compact-form"><input type="hidden" name="csrf" value="%s"><input name="name" value="%s" required><input name="description" value="%s" placeholder="Description"><input name="tags" value="%s" placeholder="Tags"><button class="primary small">Save</button></form></details><form method="post" action="/favorites/%d/delete" onsubmit="return confirm('Delete this favorite?')"><input type="hidden" name="csrf" value="%s"><button class="danger small">×</button></form></div></div>`,
				esc(fav.Name),
				fav.ID, esc(user.CSRF),
				fav.ID, esc(user.CSRF), esc(fav.Name), esc(fav.Description), esc(fav.Tags),
				fav.ID, esc(user.CSRF))
		}
		_, _ = fmt.Fprint(w, `</div></div></section>`)
		return nil
	}))
}

func EntityList[T any](title string, user *NavUser, headers []string, rows [][]string, form templ.Component) templ.Component {
	return Layout(title, user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, title, "Directory", "Create and maintain the records used by time entries and reporting.")
		if form != nil {
			_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create `+esc(singularTitle(title))+`</h2><p>Keep required fields tight and searchable.</p></div></div>`)
			if err := form.Render(ctx, w); err != nil {
				return err
			}
			_, _ = fmt.Fprint(w, `</section>`)
		}
		dataTable(w, headers, rows)
		return nil
	}))
}

func EntityListRaw(title string, user *NavUser, headers []string, rows [][]string, form templ.Component) templ.Component {
	return Layout(title, user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, title, "Directory", "Create and maintain the records used by time entries and reporting.")
		if form != nil {
			_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create `+esc(singularTitle(title))+`</h2><p>Keep required fields tight and searchable.</p></div></div>`)
			if err := form.Render(ctx, w); err != nil {
				return err
			}
			_, _ = fmt.Fprint(w, `</section>`)
		}
		dataTableRaw(w, headers, rows)
		return nil
	}))
}

func CustomerForm(user *NavUser, c *domain.Customer) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/customers"
		if c != nil && c.ID > 0 {
			action = fmt.Sprintf("/customers/%d", c.ID)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Company<input name="company" value="%s"></label><label>Email<input name="email" value="%s"></label><label>Billing unit <span class="field-hint">ISO 4217 code — e.g. USD (dollar·cent), EUR (euro·cent), GBP (pound·pence)</span><input name="currency" value="%s" placeholder="USD" maxlength="3"></label><label>Timezone<input name="timezone" value="%s"></label><label>Reference number<input name="number" value="%s"></label><label class="wide">Comment<textarea name="comment">%s</textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save customer</button></div></form>`,
			action, esc(user.CSRF), val(c, "Name"), val(c, "Company"), val(c, "Email"), defaultVal(val(c, "Currency"), "USD"), defaultVal(val(c, "Timezone"), "UTC"), val(c, "Number"), val(c, "Comment"))
		return nil
	})
}

func ProjectForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/projects"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Customer"+tipHTML("The client this project is billed to. Determines the billing unit and default billing contact."), "customer_id", optionList(selectors, "customer"), 0, true, "Select a customer", nil)
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" required></label>`, tipHTML("Project display name shown in timesheets, reports and invoices."))
		_, _ = fmt.Fprintf(w, `<label>Number %s<input name="number"></label>`, tipHTML("Internal reference code (e.g. P-001). Optional — used for export matching."))
		_, _ = fmt.Fprintf(w, `<label>Order number %s<input name="order_number"></label>`, tipHTML("Purchase order or contract reference number for invoice line items."))
		_, _ = fmt.Fprintf(w, `<label>Estimate hours %s<input name="estimate_hours" value="0"></label>`, tipHTML("Total hours budgeted. Tockr shows burn against this in the project dashboard."))
		_, _ = fmt.Fprintf(w, `<label>Budget (¢) %s<input name="budget_cents" value="0" placeholder="e.g. 1000000 = $10,000"></label>`, tipHTML("Monetary budget in cents (e.g. 1000000 = $10,000). Triggers an alert when spend reaches the alert threshold."))
		_, _ = fmt.Fprintf(w, `<label>Budget alert (%%) %s<input name="budget_alert_percent" value="80"></label>`, tipHTML("Send a budget warning when this percentage of the monetary budget is consumed (e.g. 80 = alert at 80%)."))
		_, _ = fmt.Fprint(w, `<label class="wide">Comment<textarea name="comment"></textarea></label>`)
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible" checked> Visible %s</label>`, tipHTML("Visible projects appear in timesheet entry selectors. Uncheck to archive a project."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="private"> Private %s</label>`, tipHTML("Private projects are only visible to explicitly assigned members."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable" checked> Billable %s</label>`, tipHTML("Billable projects are included in invoice and rate calculations."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save project</button></div></form>`)
		return nil
	})
}

func ActivityForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/activities"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Project"+tipHTML("Scope this activity to one project, or leave blank to make it available across all projects."), "project_id", optionList(selectors, "project"), 0, false, "Global activity", nil)
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" required></label>`, tipHTML("Activity type shown in timesheets (e.g. Development, Design, Testing)."))
		_, _ = fmt.Fprintf(w, `<label>Number %s<input name="number"></label>`, tipHTML("Optional reference code. Useful for matching against external project management tools."))
		_, _ = fmt.Fprint(w, `<label class="wide">Comment<textarea name="comment"></textarea></label>`)
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible" checked> Visible %s</label>`, tipHTML("Visible activities appear in timesheet selectors. Uncheck to retire an activity without deleting it."))
		_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable" checked> Billable %s</label>`, tipHTML("Billable activities are included in invoice and rate calculations."))
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary">Save activity</button></div></form>`)
		return nil
	})
}

func TaskForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/tasks"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "Project"+tipHTML("Tasks must belong to a project. Choose which project this task falls under."), "project_id", optionList(selectors, "project"), 0, true, "Select a project", nil)
		_, _ = fmt.Fprintf(w, `<label>Name %s<input name="name" required></label>`, tipHTML("Task name shown in timesheet selectors (e.g. a sprint ticket or deliverable)."))
		_, _ = fmt.Fprintf(w, `<label>Number %s<input name="number"></label>`, tipHTML("Optional reference code — great for ticket numbers (e.g. PROJ-42). Shown in reports and CSV exports."))
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
		_, _ = fmt.Fprint(w, `<label>Billable rate (¢/hr)<input name="amount_cents" required placeholder="e.g. 10000 = $100/hr"></label><label>Internal cost rate (¢/hr) <span class="field-hint">optional, for margin reporting</span><input name="internal_amount_cents" placeholder="e.g. 6000 = $60/hr"></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to <span class="field-hint">leave blank for open-ended</span><input type="date" name="effective_to"></label><label class="check"><input type="checkbox" name="fixed"> Fixed total <span class="field-hint">charges the rate as a flat fee, not per hour</span></label><div class="form-actions"><button class="primary">Save rate</button></div></form>`)
		return nil
	})
}

func UserCostForm(user *NavUser, selectors *SelectorData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/rates/costs"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderSelect(w, "User", "user_id", optionList(selectors, "user"), 0, true, "Select a user", nil)
		_, _ = fmt.Fprint(w, `<label>User cost (¢/hr)<input name="amount_cents" required placeholder="e.g. 7500 = $75/hr"></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to<input type="date" name="effective_to"></label><div class="form-actions"><button class="primary">Save user cost</button></div></form>`)
		return nil
	})
}

func Rates(user *NavUser, rates []domain.Rate, costs []domain.UserCostRate, selectors *SelectorData) templ.Component {
	return Layout("Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Rates", "Financial controls", "Date-effective billable rates and user cost rates for auditable reporting.")
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How rate matching works:</strong> Tockr picks the most specific matching rate for each time entry. Leave a selector blank to make a rate apply more broadly — a rate with all fields blank applies to everyone. More specific rates always win over broader ones.</div>`)
		_, _ = fmt.Fprint(w, `<section class="two-col">`)
		_, _ = fmt.Fprint(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Billable rate</h2><p>Scope by customer, project, activity, task, or user.</p></div></div>`)
		if err := RateForm(user, selectors).Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</div><div class="panel form-panel"><div class="panel-head"><div><h2>User cost</h2><p>Use effective dates before profitability reporting.</p></div></div>`)
		if err := UserCostForm(user, selectors).Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</div></section>`)
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

func Timesheets(user *NavUser, rows [][]string, selectors *SelectorData) templ.Component {
	return Layout("Timesheets", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Timesheets", "Time entries", "Record work manually or use the timer from the dashboard.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create entry</h2><p>Select the related customer, project, activity, and optional task.</p></div></div><form method="post" action="/timesheets" class="form-grid selector-form"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
		renderWorkSelectors(w, selectors, true)
		_, _ = fmt.Fprint(w, `<label>Start<input type="datetime-local" name="start" required></label><label>End<input type="datetime-local" name="end" required></label><label>Break minutes <span class="field-hint">deducted from the total duration</span><input name="break_minutes" value="0"></label><label>Tags<input name="tags" placeholder="comma,separated"></label><label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Add entry</button></div></form></section>`)
		dataTable(w, []string{"Task", "Start", "End", "Duration", "Rate", "Billable", "Exported", "Description"}, rows)
		_, _ = fmt.Fprint(w, `<div class="export-row"><a class="ghost-button" href="/timesheets/export">Export CSV</a></div>`)
		return nil
	}))
}

func Reports(user *NavUser, filter domain.ReportFilter, rows []map[string]any, saved []domain.SavedReport, selectors *SelectorData) templ.Component {
	return Layout("Reports", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		group := defaultVal(filter.Group, "user")
		pageHeader(w, "Reports", "Analysis", "Filter, group, and save time-entry reports. Use \"Apply\" to view results and \"Save report\" to keep the current filters as a named shortcut.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Filters</h2><p>Save repeatable reporting views.</p></div></div><form method="get" action="/reports" class="toolbar-form selector-form"><label>Group by<select name="group">%s%s%s%s%s%s</select></label><label>Date from<input type="date" name="begin" value="%s"></label><label>Date to<input type="date" name="end" value="%s"></label>`,
			reportOption(group, "user", "Users"), reportOption(group, "customer", "Customers"), reportOption(group, "project", "Projects"), reportOption(group, "activity", "Activities"), reportOption(group, "task", "Tasks"), reportOption(group, "group", "Groups"), dateInput(filter.Begin), dateInput(filter.End))
		renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), filter.CustomerID, false, "All customers", nil)
		renderSelect(w, "Project", "project_id", optionList(selectors, "project"), filter.ProjectID, false, "All projects", map[string]string{"data-filter-parent": "customer_id", "data-filter-attr": "customer-id"})
		renderSelect(w, "Activity", "activity_id", optionList(selectors, "activity"), filter.ActivityID, false, "All activities", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "Task", "task_id", optionList(selectors, "task"), filter.TaskID, false, "All tasks", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
		renderSelect(w, "User", "user_id", optionList(selectors, "user"), filter.UserID, false, "All users", nil)
		renderSelect(w, "Group", "group_id", optionList(selectors, "group"), filter.GroupID, false, "All groups", nil)
		_, _ = fmt.Fprintf(w, `<button class="primary">Apply</button></form><form method="post" action="/reports/saved" class="toolbar-form"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Saved report name" required><input type="hidden" name="group" value="%s"><input type="hidden" name="begin" value="%s"><input type="hidden" name="end" value="%s"><input type="hidden" name="customer_id" value="%s"><input type="hidden" name="project_id" value="%s"><input type="hidden" name="activity_id" value="%s"><input type="hidden" name="task_id" value="%s"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="group_id" value="%s"><button class="table-action">Save report</button></form></section>`,
			esc(user.CSRF), esc(group), dateInput(filter.Begin), dateInput(filter.End), idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID), idValue(filter.TaskID), idValue(filter.UserID), idValue(filter.GroupID))
		renderSavedReportsDropdown(w, user, saved)
		_, _ = fmt.Fprint(w, `<div class="tabs" aria-label="Report groups">`)
		reportTab(w, group, "user", "Users")
		reportTab(w, group, "customer", "Customers")
		reportTab(w, group, "project", "Projects")
		reportTab(w, group, "activity", "Activities")
		reportTab(w, group, "task", "Tasks")
		reportTab(w, group, "group", "Groups")
		_, _ = fmt.Fprint(w, `</div>`)
		out := [][]string{}
		for _, row := range rows {
			out = append(out, []string{fmt.Sprint(row["name"]), fmt.Sprint(row["count"]), duration(row["seconds"].(int64)), money(row["cents"].(int64))})
		}
		dataTable(w, []string{strings.Title(group), "Entries", "Duration", "Revenue"}, out)
		_, _ = fmt.Fprintf(w, `<div class="export-row"><a class="ghost-button" href="/reports/export?group=%s&begin=%s&end=%s&customer_id=%s&project_id=%s&activity_id=%s&task_id=%s&user_id=%s&group_id=%s">Export CSV</a></div>`,
			esc(group), dateInput(filter.Begin), dateInput(filter.End),
			idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID),
			idValue(filter.TaskID), idValue(filter.UserID), idValue(filter.GroupID))
		return nil
	}))
}

func ProjectDashboard(user *NavUser, d domain.ProjectDashboard) templ.Component {
	return Layout("Project dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, d.Project.Name, "Project dashboard", "Estimate, budget, and tracked work for this project.")
		_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
		metric(w, "Tracked", duration(d.TrackedSeconds), "Total project time")
		metric(w, "Estimate used", fmt.Sprintf("%d%%", d.EstimatePercent), "Against estimate")
		metric(w, "Billable value", money(d.BillableCents), "Tracked value")
		metric(w, "Budget used", fmt.Sprintf("%d%%", d.BudgetPercent), "Against fixed fee")
		_, _ = fmt.Fprint(w, `</section>`)
		if d.Alert {
			_, _ = fmt.Fprint(w, `<div class="alert">This project is near or over its estimate or budget threshold.</div>`)
		}
		return nil
	}))
}

func Calendar(user *NavUser, weekStart time.Time, entries []domain.Timesheet) templ.Component {
	return Layout("Calendar", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Calendar", "Time review", "A lightweight weekly view of time entries without drag-and-drop complexity.")
		prev := weekStart.AddDate(0, 0, -7).Format("2006-01-02")
		next := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
		_, _ = fmt.Fprintf(w, `<div class="tabs"><a class="tab-link" href="/calendar?date=%s">Previous week</a><a class="tab-link active" aria-current="page" href="/calendar?date=%s">%s</a><a class="tab-link" href="/calendar?date=%s">Next week</a></div><section class="calendar-grid">`, esc(prev), esc(weekStart.Format("2006-01-02")), esc(weekStart.Format("Jan 2")+" - "+weekStart.AddDate(0, 0, 6).Format("Jan 2")), esc(next))
		for day := 0; day < 7; day++ {
			current := weekStart.AddDate(0, 0, day)
			total := int64(0)
			for _, entry := range entries {
				if sameDay(entry.StartedAt, current) {
					total += entry.DurationSeconds
				}
			}
			_, _ = fmt.Fprintf(w, `<article class="calendar-day"><div class="calendar-day-head"><strong>%s</strong><span>%s</span></div>`, esc(current.Format("Mon 02")), esc(duration(total)))
			empty := true
			for _, entry := range entries {
				if !sameDay(entry.StartedAt, current) {
					continue
				}
				empty = false
				_, _ = fmt.Fprintf(w, `<div class="calendar-entry"><strong>%s</strong><span>%s · Project %d</span><small>%s</small></div>`, esc(timeRange(entry)), esc(duration(entry.DurationSeconds)), entry.ProjectID, esc(entry.Description))
			}
			if empty {
				_, _ = fmt.Fprint(w, `<div class="calendar-empty">No entries</div>`)
			}
			_, _ = fmt.Fprint(w, `</article>`)
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func Account(user *NavUser, account domain.User, totpMode string, setupSecret, setupURI string, recoveryCodes []string, message string) templ.Component {
	return Layout("Account", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Account", "Profile and security", "Manage personal settings without changing admin-only access.")
		if message != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Profile</h2><p>Name and local display preferences.</p></div></div><form method="post" action="/account" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Display name<input name="display_name" value="%s" required></label><label>Email<input value="%s" disabled></label><label>Timezone<input name="timezone" value="%s"></label><div class="form-actions"><button class="primary">Save profile</button></div></form></div>`, esc(user.CSRF), esc(account.DisplayName), esc(account.Email), esc(account.Timezone))
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Workspace</h2><p>Your current workspace and how to switch.</p></div></div><div class="form-grid">`)
		renderWorkspaceSwitcher(w, user)
		if len(user.Workspaces) <= 1 {
			_, _ = fmt.Fprintf(w, `<p class="workspace-account-name">%s</p>`, esc(user.CurrentWorkspaceName))
		}
		_, _ = fmt.Fprint(w, `</div></div>`)
		_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Password</h2><p>Change your password for local authentication.</p></div></div><form method="post" action="/account/password" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="username" autocomplete="username" value="%s"><label>Current password<input name="current_password" type="password" autocomplete="current-password" required></label><label>New password<input name="password" type="password" minlength="8" autocomplete="new-password" required></label><label>Confirm password<input name="confirm" type="password" minlength="8" autocomplete="new-password" required></label><div class="form-actions"><button class="primary">Update password</button></div></form></div></section>`, esc(user.CSRF), esc(account.Email))
		_, _ = fmt.Fprint(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Two-factor authentication</h2><p>Optional unless deployment policy requires it.</p></div></div>`)
		if totpMode == "disabled" {
			_, _ = fmt.Fprint(w, `<p>Two-factor authentication is disabled for this deployment.</p>`)
		} else if account.TOTPEnabled {
			_, _ = fmt.Fprintf(w, `<p><span class="badge success">Enabled</span></p><form method="post" action="/account/totp/disable" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Disable TOTP</button></form>`, esc(user.CSRF))
		} else {
			_, _ = fmt.Fprintf(w, `<p><span class="badge muted">Not enabled</span></p><form method="post" action="/account/totp/enable" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="secret" value="%s"><label class="wide">Authenticator URI<input readonly value="%s"></label><label>Verification code<input name="code" inputmode="numeric" autocomplete="one-time-code" required></label><div class="form-actions"><button class="primary">Enable TOTP</button></div></form>`, esc(user.CSRF), esc(setupSecret), esc(setupURI))
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

func ProjectMembers(user *NavUser, project domain.Project, members []domain.ProjectMember, users []domain.User, assignedGroups []domain.Group, groups []domain.Group) templ.Component {
	return Layout("Project members", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		userLabels := userLabelMap(users)
		groupLabels := groupLabelMap(groups)
		pageHeader(w, project.Name, "Project access", "Manage direct project members and group access for private work.")
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
		_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/%d/members/remove" onsubmit="return confirm('Remove selected project members?')"><input type="hidden" name="csrf" value="%s">`, project.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"User", "Role"}, memberRows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected members</button></div></form>`)
		groupRows := [][]string{}
		for _, group := range assignedGroups {
			groupRows = append(groupRows, []string{fmt.Sprintf(`<label class="check-inline"><input type="checkbox" name="group_id" value="%d"> %s</label>`, group.ID, esc(label(groupLabels, group.ID)))})
		}
		_, _ = fmt.Fprint(w, `<div class="section-spacer"></div>`)
		_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/%d/groups/remove" onsubmit="return confirm('Remove selected project groups?')"><input type="hidden" name="csrf" value="%s">`, project.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"Group"}, groupRows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected groups</button></div></form>`)
		return nil
	}))
}

func WorkspaceAdmin(user *NavUser, workspaces []domain.WorkspaceSummary) templ.Component {
	return Layout("Workspaces", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Workspaces", "Organization admin", "Create and manage organization workspaces.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create workspace</h2><p>Only organization admins can add workspaces.</p></div></div><form method="post" action="/admin/workspaces" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>Slug <span class="field-hint">URL-safe ID, auto-generated if blank</span><input name="slug" placeholder="e.g. my-team"></label><label>Billing unit <span class="field-hint">ISO 4217 code — e.g. USD (dollar·cent), EUR (euro·cent)</span><input name="default_currency" value="USD" placeholder="USD" maxlength="3"></label><label>Timezone<input name="timezone" value="UTC"></label><label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Create workspace</button></div></form></section>`, esc(user.CSRF))
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
		pageHeader(w, workspace.Name, "Workspace admin", "Edit settings and manage workspace members.")
		checked := ""
		if workspace.Archived {
			checked = " checked"
		}
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Settings</h2><p>Changes apply only inside this organization.</p></div></div><form method="post" action="/admin/workspaces/%d" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Slug <span class="field-hint">URL-safe ID</span><input name="slug" value="%s" required></label><label>Billing unit <span class="field-hint">ISO 4217 code — e.g. USD (dollar·cent), EUR (euro·cent)</span><input name="default_currency" value="%s" placeholder="USD" maxlength="3"></label><label>Timezone<input name="timezone" value="%s"></label><label class="wide">Description<textarea name="description">%s</textarea></label><label class="check"><input type="checkbox" name="archived"%s> Archived</label><div class="form-actions"><button class="primary">Save workspace</button></div></form></div>`, workspace.ID, esc(user.CSRF), esc(workspace.Name), esc(workspace.Slug), esc(workspace.DefaultCurrency), esc(workspace.Timezone), esc(workspace.Description), checked)
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
			rows = append(rows, []string{member.DisplayName, member.Email, string(member.Role), status, fmt.Sprint(member.GroupCount), fmt.Sprint(member.ProjectMemberCount), fmt.Sprintf(`<form method="post" action="/admin/workspaces/%d/members/remove" onsubmit="return confirm('Remove this workspace member?')"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="user_id" value="%d"><button class="table-action">Remove</button></form>`, workspace.ID, esc(user.CSRF), member.UserID)})
		}
		dataTableRaw(w, []string{"Name", "Email", "Role", "Status", "Groups", "Projects", "Action"}, rows)
		return nil
	}))
}

func GroupMembers(user *NavUser, group domain.Group, members []domain.User, users []domain.User) templ.Component {
	return Layout("Group members", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		memberLabels := userLabelMap(members)
		pageHeader(w, group.Name, "Group membership", "Bulk add or remove workspace users from this group.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add members</h2><p>Select one or more workspace users.</p></div></div><form method="post" action="/groups/%d/members" class="form-grid"><input type="hidden" name="csrf" value="%s"><label class="wide">Users<select name="user_id" multiple size="10">`, group.ID, esc(user.CSRF))
		for _, item := range users {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, item.ID, esc(userLabel(item)))
		}
		_, _ = fmt.Fprint(w, `</select></label><div class="form-actions"><button class="primary">Add selected</button></div></form></section>`)
		rows := [][]string{}
		for _, member := range members {
			rows = append(rows, []string{fmt.Sprintf(`<label class="check-inline"><input type="checkbox" name="user_id" value="%d"> %s</label>`, member.ID, esc(label(memberLabels, member.ID))), member.Email})
		}
		_, _ = fmt.Fprintf(w, `<form method="post" action="/groups/%d/members/remove" onsubmit="return confirm('Remove selected group members?')"><input type="hidden" name="csrf" value="%s">`, group.ID, esc(user.CSRF))
		dataTableRaw(w, []string{"Member", "Email"}, rows)
		_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="danger">Remove selected</button></div></form>`)
		return nil
	}))
}

func ProjectTemplates(user *NavUser, templates []domain.ProjectTemplate, selectors *SelectorData) templ.Component {
	return Layout("Project templates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Project templates", "Admin", "Create reusable project blueprints for common work.")
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How templates work:</strong> Create a template once with your standard project settings, tasks, and activities. Then use <em>Use template</em> below to spin up a new project with all those defaults pre-applied.</div>`)
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create template</h2><p>Define defaults including tasks and activities (one per line).</p></div></div>`)
		renderProjectTemplateForm(w, user, domain.ProjectTemplate{Visible: true, Billable: true, BudgetAlertPercent: 80}, "/project-templates")
		_, _ = fmt.Fprint(w, `</section>`)
		rows := [][]string{}
		for _, template := range templates {
			status := "Active"
			if template.Archived {
				status = "Archived"
			}
			rows = append(rows, []string{
				template.Name,
				template.ProjectName,
				status,
				fmt.Sprint(len(template.Tasks)),
				fmt.Sprint(len(template.Activities)),
				fmt.Sprintf(`<a class="table-action" href="/project-templates/%d">Edit</a>`, template.ID),
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
		pageHeader(w, template.Name, "Project template", "Edit template defaults or create a project from this template.")
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
	_, _ = fmt.Fprintf(w, `<form method="post" action="%s" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Project name<input name="project_name" value="%s" required></label><label>Project number<input name="project_number" value="%s"></label><label>Order number<input name="order_number" value="%s"></label><label>Estimate hours<input name="estimate_hours" value="%d"></label><label>Budget (¢)<input name="budget_cents" value="%d"></label><label>Budget alert (%%)<input name="budget_alert_percent" value="%d"></label><label class="wide">Description<textarea name="description">%s</textarea></label><label class="wide">Default tasks<textarea name="tasks" placeholder="One task per line">%s</textarea></label><label class="wide">Default activities<textarea name="activities" placeholder="One activity per line">%s</textarea></label><label class="check"><input type="checkbox" name="visible"%s> Visible</label><label class="check"><input type="checkbox" name="private"%s> Private</label><label class="check"><input type="checkbox" name="billable"%s> Billable</label><label class="check"><input type="checkbox" name="archived"%s> Archived</label><div class="form-actions"><button class="primary">Save template</button></div></form>`,
		esc(action), esc(user.CSRF), esc(template.Name), esc(template.ProjectName), esc(template.ProjectNumber), esc(template.OrderNo), template.EstimateSeconds/3600, template.BudgetCents, defaultInt(template.BudgetAlertPercent, 80), esc(template.Description), esc(templateTaskLines(template.Tasks)), esc(templateActivityLines(template.Activities)), visibleChecked, privateChecked, billableChecked, archiveChecked)
}

func Invoices(user *NavUser, invoices []domain.Invoice, selectors *SelectorData) templ.Component {
	return Layout("Invoices", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Invoices", "Billing", "Create lightweight invoice records from unexported billable time.")
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
		pageHeader(w, "Webhooks", "Integrations", "Send signed JSON events without plugin infrastructure.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add endpoint</h2><p>Use '*' for all events or comma-separated event names.</p></div></div><form method="post" action="/webhooks" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>URL<input name="url" required></label><label>Secret <span class="field-hint">signs each payload — store it securely</span><input name="secret" required></label><label>Events <span class="field-hint">use * for all, or comma-separate names</span><input name="events" value="*"></label><div class="form-actions"><button class="primary">Add webhook</button></div></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, hook := range hooks {
			rows = append(rows, []string{hook.Name, hook.URL, strings.Join(hook.Events, ","), yesNo(hook.Enabled)})
		}
		dataTableRaw(w, []string{"Name", "URL", "Events", "Enabled"}, rows)
		return nil
	}))
}

func UserForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/admin/users"><input type="hidden" name="csrf" value="%s"><label>Email<input name="email" type="email" required></label><label>Username <span class="field-hint">used for login</span><input name="username" required></label><label>Display name<input name="display_name" required></label><label>Password<input name="password" type="password" required></label><label>Timezone<input name="timezone" value="UTC"></label><label>Role<select name="role"><option value="user">User</option><option value="teamlead">Team lead</option><option value="admin">Admin</option><option value="superadmin">Super admin</option></select></label><div class="form-actions"><button class="primary">Create user</button></div></form>`, esc(user.CSRF))
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
					`<li><a href="/projects">Projects &amp; team members</a></li>`+
					`<li><a href="/activities">Activities</a> &amp; <a href="/tasks">tasks</a></li>`+
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
	renderSelect(w, "Customer", "customer_id", optionList(selectors, "customer"), 0, requireCore, "Select a customer", nil)
	renderSelect(w, "Project", "project_id", optionList(selectors, "project"), 0, requireCore, "Select a project", map[string]string{"data-filter-parent": "customer_id", "data-filter-attr": "customer-id"})
	renderSelect(w, "Activity", "activity_id", optionList(selectors, "activity"), 0, requireCore, "Select an activity", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
	renderSelect(w, "Task", "task_id", optionList(selectors, "task"), 0, false, "No task", map[string]string{"data-filter-parent": "project_id", "data-filter-attr": "project-id"})
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
	_, _ = fmt.Fprintf(w, `<div class="dropdown account-menu" data-dropdown="account"><button class="account-trigger" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="account-menu"><span class="avatar" aria-hidden="true">%s</span><span class="account-name">%s</span><span class="chevron" aria-hidden="true">▾</span></button><div class="dropdown-menu dropdown-menu-right" id="account-menu" role="menu" hidden data-dropdown-menu><a role="menuitem" href="/">Dashboard</a><a role="menuitem" href="/timesheets">Timesheets</a><a role="menuitem" href="/account">Account settings</a><form method="post" action="/logout" role="none"><input type="hidden" name="csrf" value="%s"><button role="menuitem" type="submit">Logout</button></form></div></div>`, esc(initial), esc(user.DisplayName), esc(user.CSRF))
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
	_, _ = fmt.Fprintf(&b, `<div class="dropdown row-menu" data-dropdown="%s"><button class="icon-button" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="%s-menu" aria-label="%s">•••</button><div class="dropdown-menu dropdown-menu-right" id="%s-menu" role="menu" hidden data-dropdown-menu>`, esc(id), esc(id), esc(label), esc(id))
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
		_, _ = fmt.Fprintf(w, `<div role="menuitem" class="saved-report-item"><a href="%s" class="saved-report-link"><span>%s</span><small>%s</small></a><div class="saved-report-actions">%s<form method="post" action="/reports/saved/%d/delete" onsubmit="return confirm('Delete this saved report?')"><input type="hidden" name="csrf" value="%s"><button class="danger small">Delete</button></form><form method="post" action="/reports/saved/%d/share"><input type="hidden" name="csrf" value="%s"><input type="number" name="days" value="30" min="1" max="365" style="width:50px"><button class="table-action small">Share</button></form></div></div>`,
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

func reportOption(current, value, label string) string {
	selected := ""
	if current == value {
		selected = ` selected`
	}
	return fmt.Sprintf(`<option value="%s"%s>%s</option>`, esc(value), selected, esc(label))
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
			if index == 0 {
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

func money(cents int64) string {
	return fmt.Sprintf("$%.2f", float64(cents)/100)
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
	case "Activities":
		return "Activity"
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

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// ─── New feature templates ────────────────────────────────────────────────────

func Tasks(user *NavUser, tasks []domain.Task, selectors *SelectorData, canManage bool) templ.Component {
	return Layout("Tasks", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Tasks", "Work breakdown", "")
		if canManage {
			_, _ = fmt.Fprintf(w, `<details class="panel collapsible"><summary>Create task</summary><form class="form-grid" method="post" action="/tasks"><input type="hidden" name="csrf" value="%s">`, esc(user.CSRF))
			renderSelect(w, "Project", "project_id", optionList(selectors, "project"), 0, true, "Select a project", nil)
			_, _ = fmt.Fprint(w, `<label>Name<input name="name" required></label><label>Task number<input name="number"></label><label>Estimate (hours)<input name="estimate_hours" type="number" min="0" value="0"></label><label class="checkbox-label"><input type="checkbox" name="visible" value="1" checked>Visible</label><label class="checkbox-label"><input type="checkbox" name="billable" value="1" checked>Billable</label><div class="form-actions"><button class="primary">Create</button></div></form></details>`)
		}
		if len(tasks) == 0 {
			_, _ = fmt.Fprint(w, `<p class="empty-state">No tasks yet.</p>`)
		} else {
			_, _ = fmt.Fprint(w, `<table><thead><tr><th>Name</th><th>Project</th><th>Number</th><th>Visible</th><th>Billable</th><th>Estimate</th>`)
			if canManage {
				_, _ = fmt.Fprint(w, `<th></th>`)
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
					_, _ = fmt.Fprintf(w, `<td class="actions-cell"><details class="dropdown inline-dropdown"><summary class="table-action">Edit</summary><form class="compact-form" method="post" action="/tasks/%d"><input type="hidden" name="csrf" value="%s"><input name="name" value="%s" required><input name="number" value="%s"><input name="estimate_hours" type="number" value="%d"><label class="checkbox-label"><input type="checkbox" name="visible" value="1"%s>Visible</label><label class="checkbox-label"><input type="checkbox" name="billable" value="1"%s>Billable</label><button class="primary small">Save</button></form></details><form method="post" action="/tasks/%d/archive" onsubmit="return confirm('Archive this task?')"><input type="hidden" name="csrf" value="%s"><button class="danger small">Archive</button></form></td>`,
						task.ID, esc(user.CSRF),
						esc(task.Name), esc(task.Number),
						task.EstimateSeconds/3600,
						checkedIf(task.Visible), checkedIf(task.Billable),
						task.ID, esc(user.CSRF))
				}
				_, _ = fmt.Fprint(w, `</tr>`)
			}
			_, _ = fmt.Fprint(w, `</tbody></table>`)
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
		_, _ = fmt.Fprintf(w, `<section class="login-shell"><div class="login-card"><h1>%s</h1>`, esc(name))
		_, _ = fmt.Fprint(w, `<table><thead><tr><th>Name</th><th>Entries</th><th>Hours</th><th>Amount</th></tr></thead><tbody>`)
		for _, row := range rows {
			sec, _ := row["seconds"].(int64)
			cents, _ := row["cents"].(int64)
			count, _ := row["count"].(int64)
			_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
				esc(fmt.Sprintf("%v", row["name"])), count, duration(sec), money(cents))
		}
		_, _ = fmt.Fprint(w, `</tbody></table></div></section>`)
		return nil
	}))
}

func Utilization(user *NavUser, rows []domain.UtilizationRow, beginStr, endStr string) templ.Component {
	return Layout("Utilization", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="content-shell"><header class="page-header"><h1>Utilization</h1><p class="page-desc">Shows how much time each team member tracked in the selected period and what percentage was billable. The bar chart is relative to the highest-tracked user.</p></header>`)
		_, _ = fmt.Fprintf(w, `<form class="filter-bar" method="get" action="/reports/utilization"><label>From<input type="date" name="begin" value="%s"></label><label>To<input type="date" name="end" value="%s"></label><button class="primary">Apply</button>`, esc(beginStr), esc(endStr))
		_, _ = fmt.Fprintf(w, ` <a class="button" href="/reports/export?begin=%s&end=%s">Export CSV</a></form>`, esc(beginStr), esc(endStr))
		if len(rows) == 0 {
			_, _ = fmt.Fprint(w, `<p class="empty-state">No data for selected period.</p>`)
		} else {
			var maxSec int64
			for _, r := range rows {
				if r.TotalSeconds > maxSec {
					maxSec = r.TotalSeconds
				}
			}
			_, _ = fmt.Fprint(w, `<table class="utilization-table"><thead><tr><th>User</th><th>Total</th><th>Billable</th><th>Utilization</th><th>Amount</th></tr></thead><tbody>`)
			for _, r := range rows {
				pct := int64(0)
				if r.TotalSeconds > 0 {
					pct = 100 * r.BillableSeconds / r.TotalSeconds
				}
				barPct := int64(0)
				if maxSec > 0 {
					barPct = 100 * r.TotalSeconds / maxSec
				}
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td><div class="util-bar-wrap"><div class="util-bar" style="width:%d%%"></div><span>%d%%</span></div></td><td>%s</td></tr>`,
					esc(r.DisplayName), duration(r.TotalSeconds), duration(r.BillableSeconds), barPct, pct, money(r.EntryCents))
			}
			_, _ = fmt.Fprint(w, `</tbody></table>`)
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func ExchangeRates(user *NavUser, rates []domain.ExchangeRate) templ.Component {
	return Layout("Exchange Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="content-shell"><header class="page-header"><h1>Exchange Rates</h1></header>`)
		_, _ = fmt.Fprint(w, `<p class="page-desc">Exchange rates convert billing amounts across billing units for multi-unit reporting. Enter the multiplier as a decimal — for example, 0.920 means 1 USD (dollar·cent) converts to 0.920 EUR (euro·cent). The most recently effective rate for each pair is applied automatically.</p>`)
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/admin/exchange-rates"><input type="hidden" name="csrf" value="%s"><label>From unit <span class="field-hint">ISO 4217, e.g. USD (dollar·cent)</span><input name="from_currency" maxlength="3" placeholder="USD" required></label><label>To unit <span class="field-hint">ISO 4217, e.g. EUR (euro·cent)</span><input name="to_currency" maxlength="3" placeholder="EUR" required></label><label>Rate<input name="rate" type="number" step="0.000001" min="0.000001" placeholder="0.920" required></label><label>Effective From<input name="effective_from" type="date" required></label><div class="form-actions"><button class="primary">Add Rate</button></div></form>`, esc(user.CSRF))
		if len(rates) == 0 {
			_, _ = fmt.Fprint(w, `<p class="empty-state">No exchange rates defined.</p>`)
		} else {
			_, _ = fmt.Fprint(w, `<table><thead><tr><th>From</th><th>To</th><th>Rate</th><th>Effective</th><th></th></tr></thead><tbody>`)
			for _, r := range rates {
				_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%.4f</td><td>%s</td><td><form method="post" action="/admin/exchange-rates/%d/delete"><input type="hidden" name="csrf" value="%s"><button class="danger small">Delete</button></form></td></tr>`,
					esc(r.FromCurrency), esc(r.ToCurrency), float64(r.RateThousandths)/1000.0, esc(r.EffectiveFrom.Format("2006-01-02")), r.ID, esc(user.CSRF))
			}
			_, _ = fmt.Fprint(w, `</tbody></table>`)
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func Recalculate(user *NavUser, preview []domain.RecalcPreviewRow, selectors *SelectorData, projectID int64, sinceStr string) templ.Component {
	return Layout("Recalculate Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="content-shell"><header class="page-header"><h1>Retroactive Rate Recalculation</h1></header>`)
		_, _ = fmt.Fprint(w, `<div class="info-callout"><strong>How to use this screen:</strong> Choose a project and a start date, then click <strong>Preview</strong> to see which time entries have a different rate than the one currently configured. Entries already included in an exported invoice are flagged — recalculating them updates the stored value but will not change any invoice already sent. Click <strong>Apply</strong> only when you have reviewed all changes.</div>`)
		_, _ = fmt.Fprintf(w, `<form class="filter-bar" method="get" action="/admin/recalculate">`)
		renderSelect(w, "Project", "project_id", optionList(selectors, "project"), projectID, true, "Select a project", nil)
		_, _ = fmt.Fprintf(w, `<label>Since<input type="date" name="since" value="%s"></label><button class="primary">Preview</button></form>`, esc(sinceStr))
		if len(preview) == 0 {
			_, _ = fmt.Fprint(w, `<p class="empty-state">No timesheets need recalculation for the selected filters.</p>`)
		} else {
			var deltaTotal int64
			_, _ = fmt.Fprint(w, `<div class="info-callout warn">The entries below have a different rate than the one currently configured. <strong>Exported entries are flagged</strong> — updating them will not change any invoice already sent, but will affect future exports.</div>`)
			_, _ = fmt.Fprint(w, `<table><thead><tr><th>Date</th><th>Description</th><th>Current ¢/h</th><th>New ¢/h</th><th>Delta ¢</th><th>Exported</th></tr></thead><tbody>`)
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
			_, _ = fmt.Fprintf(w, `</tbody><tfoot><tr><td colspan="4"><strong>Total delta</strong></td><td><strong>%s</strong></td><td></td></tr></tfoot></table>`, money(deltaTotal))
			_, _ = fmt.Fprintf(w, `<form method="post" action="/admin/recalculate"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="project_id" value="%d"><input type="hidden" name="since" value="%s"><div class="form-actions"><button class="primary danger">Apply Recalculation (%d timesheets)</button></div></form>`,
				esc(user.CSRF), projectID, esc(sinceStr), len(preview))
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}
