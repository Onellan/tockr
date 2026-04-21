package templates

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"path"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tockr/internal/auth"
	"tockr/internal/domain"
)

type NavUser struct {
	DisplayName string
	CSRF        string
	CurrentPath string
	Permissions map[string]bool
}

type navItem struct {
	Label      string
	Path       string
	Group      string
	Permission string
}

var primaryNav = []navItem{
	{"Dashboard", "/", "Work", ""},
	{"Timesheets", "/timesheets", "Work", auth.PermTrackTime},
	{"Customers", "/customers", "Manage", ""},
	{"Projects", "/projects", "Manage", ""},
	{"Activities", "/activities", "Manage", ""},
	{"Tags", "/tags", "Manage", auth.PermTrackTime},
	{"Groups", "/groups", "Manage", auth.PermManageGroups},
	{"Reports", "/reports", "Analyze", auth.PermViewReports},
	{"Invoices", "/invoices", "Analyze", auth.PermManageInvoices},
}

var adminNav = []navItem{
	{"Rates", "/rates", "Admin", auth.PermManageRates},
	{"Users", "/admin/users", "Admin", auth.PermManageUsers},
	{"Webhooks", "/webhooks", "Admin", auth.PermManageWebhooks},
}

func Layout(title string, user *NavUser, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>%s</title><link rel=\"stylesheet\" href=\"/static/style.css\"><script src=\"/static/htmx-lite.js\" defer></script></head><body>", esc(title))
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
		_, _ = fmt.Fprintf(w, `</nav></aside><div class="workspace"><header class="topbar"><div><span class="topbar-kicker">Workspace</span><strong>%s</strong></div><div class="account-area" aria-label="Account"><span class="account-name">%s</span><form method="post" action="/logout"><input type="hidden" name="csrf" value="%s"><button class="ghost-button" type="submit">Logout</button></form></div></header><main class="content" id="main-content" tabindex="-1">`, esc(title), esc(user.DisplayName), esc(user.CSRF))
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</main></div></div></body></html>`)
		return nil
	})
}

func Login(message string) templ.Component {
	return Layout("Login", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Fast, focused time tracking for small teams and Raspberry Pi deployments.</p><ul><li>Server-rendered workflows</li><li>SQLite-first operations</li><li>Kimai-inspired business structure</li></ul></div><form method="post" action="/login" class="login-card"><div><h2>Sign in</h2><p>Use your workspace credentials.</p></div>`)
		if message != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		_, _ = fmt.Fprint(w, `<label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><button class="primary full">Login</button></form></section>`)
		return nil
	}))
}

func Dashboard(user *NavUser, stats map[string]int64, active *domain.Timesheet) templ.Component {
	return Layout("Dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Dashboard", "Live overview", "Track current work, unexported time, and billing movement.")
		_, _ = fmt.Fprint(w, `<section class="metric-grid">`)
		metric(w, "Active timers", fmt.Sprint(stats["active_timers"]), "Running entries")
		metric(w, "Today", duration(stats["today_seconds"]), "Tracked by you")
		metric(w, "Unexported", fmt.Sprint(stats["unexported"]), "Billable entries")
		metric(w, "Invoices", fmt.Sprint(stats["invoices"]), "Created documents")
		_, _ = fmt.Fprint(w, `</section><section class="two-col"><div class="panel"><div class="panel-head"><div><h2>Timer</h2><p>Start or stop the active work entry.</p></div></div>`)
		if active != nil {
			_, _ = fmt.Fprintf(w, `<div class="timer-running"><span class="status-dot"></span><div><strong>Running since %s</strong><p>Timer is active for this user.</p></div></div><form method="post" action="/timesheets/stop" class="actions-row"><input type="hidden" name="csrf" value="%s"><button class="danger">Stop timer</button></form>`, esc(active.StartedAt.Format("15:04")), esc(user.CSRF))
		} else {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/timesheets/start" class="compact-form"><input type="hidden" name="csrf" value="%s"><input name="customer_id" placeholder="Customer ID" required><input name="project_id" placeholder="Project ID" required><input name="activity_id" placeholder="Activity ID" required><input name="description" placeholder="What are you working on?"><button class="primary">Start timer</button></form>`, esc(user.CSRF))
		}
		_, _ = fmt.Fprint(w, `</div><div class="panel"><div class="panel-head"><div><h2>Operational focus</h2><p>Keep unexported billable time low and invoice regularly.</p></div></div><div class="summary-list"><div><span>Recommended next step</span><strong>Review timesheets</strong></div><div><span>Billing readiness</span><strong>Check invoice queue</strong></div></div></div></section>`)
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

func CustomerForm(user *NavUser, c *domain.Customer) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/customers"
		if c != nil && c.ID > 0 {
			action = fmt.Sprintf("/customers/%d", c.ID)
		}
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Company<input name="company" value="%s"></label><label>Email<input name="email" value="%s"></label><label>Currency<input name="currency" value="%s"></label><label>Timezone<input name="timezone" value="%s"></label><label>Number<input name="number" value="%s"></label><label class="wide">Comment<textarea name="comment">%s</textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save customer</button></div></form>`,
			action, esc(user.CSRF), val(c, "Name"), val(c, "Company"), val(c, "Email"), defaultVal(val(c, "Currency"), "USD"), defaultVal(val(c, "Timezone"), "UTC"), val(c, "Number"), val(c, "Comment"))
		return nil
	})
}

func ProjectForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/projects"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label>Order<input name="order_number"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="private"> Private</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save project</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func ActivityForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/activities"><input type="hidden" name="csrf" value="%s"><label>Project ID<input name="project_id"></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save activity</button></div></form>`, esc(user.CSRF))
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
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/groups"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Save group</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func RateForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/rates"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id"></label><label>Project ID<input name="project_id"></label><label>Activity ID<input name="activity_id"></label><label>User ID<input name="user_id"></label><label>Hourly amount cents<input name="amount_cents" required></label><label>Internal cents<input name="internal_amount_cents"></label><label class="check"><input type="checkbox" name="fixed"> Fixed rate</label><div class="form-actions"><button class="primary">Save rate</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func Timesheets(user *NavUser, rows [][]string) templ.Component {
	return Layout("Timesheets", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Timesheets", "Time entries", "Record work manually or use the timer from the dashboard.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create entry</h2><p>Use customer, project, and activity IDs from the management lists.</p></div></div><form method="post" action="/timesheets" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Project ID<input name="project_id" required></label><label>Activity ID<input name="activity_id" required></label><label>Start<input type="datetime-local" name="start" required></label><label>End<input type="datetime-local" name="end" required></label><label>Break minutes<input name="break_minutes" value="0"></label><label>Tags<input name="tags" placeholder="comma,separated"></label><label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Add entry</button></div></form></section>`, esc(user.CSRF))
		dataTable(w, []string{"ID", "Start", "End", "Duration", "Rate", "Billable", "Exported", "Description"}, rows)
		return nil
	}))
}

func Reports(user *NavUser, group string, rows []map[string]any) templ.Component {
	return Layout("Reports", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Reports", "Analysis", "Branch-derived activity and customer reporting, simplified for fast reads.")
		_, _ = fmt.Fprint(w, `<div class="tabs" aria-label="Report groups">`)
		reportTab(w, group, "user", "Users")
		reportTab(w, group, "customer", "Customers")
		reportTab(w, group, "project", "Projects")
		reportTab(w, group, "activity", "Activities")
		reportTab(w, group, "group", "Groups")
		_, _ = fmt.Fprint(w, `</div>`)
		out := [][]string{}
		for _, row := range rows {
			out = append(out, []string{fmt.Sprint(row["name"]), fmt.Sprint(row["count"]), duration(row["seconds"].(int64)), money(row["cents"].(int64))})
		}
		dataTable(w, []string{strings.Title(group), "Entries", "Duration", "Revenue"}, out)
		return nil
	}))
}

func Invoices(user *NavUser, invoices []domain.Invoice) templ.Component {
	return Layout("Invoices", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Invoices", "Billing", "Create lightweight invoice records from unexported billable time.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create invoice</h2><p>Select a customer and date range.</p></div></div><form method="post" action="/invoices" class="toolbar-form"><input type="hidden" name="csrf" value="%s"><input name="customer_id" placeholder="Customer ID" required><input type="date" name="begin" required><input type="date" name="end" required><input name="tax" value="0" placeholder="Tax percent"><button class="primary">Create invoice</button></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, inv := range invoices {
			rows = append(rows, []string{fmt.Sprint(inv.ID), inv.Number, fmt.Sprint(inv.CustomerID), status(inv.Status), money(inv.TotalCents), fmt.Sprintf(`<a class="table-action" href="/api/invoices/%d/download">Download</a>`, inv.ID)})
		}
		dataTableRaw(w, []string{"ID", "Number", "Customer", "Status", "Total", "File"}, rows)
		return nil
	}))
}

func Webhooks(user *NavUser, hooks []domain.WebhookEndpoint) templ.Component {
	return Layout("Webhooks", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Webhooks", "Integrations", "Send signed JSON events without plugin infrastructure.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Add endpoint</h2><p>Use '*' for all events or comma-separated event names.</p></div></div><form method="post" action="/webhooks" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>URL<input name="url" required></label><label>Secret<input name="secret" required></label><label>Events<input name="events" value="*"></label><div class="form-actions"><button class="primary">Add webhook</button></div></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, hook := range hooks {
			rows = append(rows, []string{fmt.Sprint(hook.ID), hook.Name, hook.URL, strings.Join(hook.Events, ","), yesNo(hook.Enabled)})
		}
		dataTableRaw(w, []string{"ID", "Name", "URL", "Events", "Enabled"}, rows)
		return nil
	}))
}

func UserForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/admin/users"><input type="hidden" name="csrf" value="%s"><label>Email<input name="email" type="email" required></label><label>Username<input name="username" required></label><label>Name<input name="display_name" required></label><label>Password<input name="password" type="password" required></label><label>Timezone<input name="timezone" value="UTC"></label><label>Role<select name="role"><option value="user">User</option><option value="teamlead">Team lead</option><option value="admin">Admin</option><option value="superadmin">Super admin</option></select></label><div class="form-actions"><button class="primary">Create user</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func ErrorPage(title, message string) templ.Component {
	return Layout(title, nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="login-shell"><div class="login-card"><h1>%s</h1><p>%s</p><a class="table-action" href="/">Return home</a></div></section>`, esc(title), esc(message))
		return nil
	}))
}

func renderNav(w io.Writer, user *NavUser, items []navItem) {
	currentGroup := ""
	for _, item := range items {
		if !user.can(item.Permission) {
			continue
		}
		if item.Group != currentGroup {
			currentGroup = item.Group
			_, _ = fmt.Fprintf(w, `<span class="nav-group">%s</span>`, esc(currentGroup))
		}
		class := ` class="nav-link"`
		if isActivePath(user.CurrentPath, item.Path) {
			class = ` class="nav-link active" aria-current="page"`
		}
		_, _ = fmt.Fprintf(w, `<a%s href="%s">%s</a>`, class, esc(item.Path), esc(item.Label))
	}
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
