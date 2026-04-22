package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path"
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
	{"Invoices", "/invoices", "Analyze", auth.PermManageInvoices},
}

var adminNav = []navItem{
	{"Rates", "/rates", "Admin", auth.PermManageRates},
	{"Users", "/admin/users", "Admin", auth.PermManageUsers},
	{"Webhooks", "/webhooks", "Admin", auth.PermManageWebhooks},
}

func Layout(title string, user *NavUser, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>%s</title><link rel=\"icon\" href=\"/favicon.ico?v=20260422\" sizes=\"any\"><link rel=\"icon\" type=\"image/png\" sizes=\"32x32\" href=\"/static/favicon-32x32.png?v=20260422\"><link rel=\"icon\" type=\"image/png\" sizes=\"16x16\" href=\"/static/favicon-16x16.png?v=20260422\"><link rel=\"apple-touch-icon\" sizes=\"180x180\" href=\"/static/apple-touch-icon.png?v=20260422\"><link rel=\"manifest\" href=\"/static/site.webmanifest?v=20260422\"><meta name=\"theme-color\" content=\"#0f766e\"><link rel=\"stylesheet\" href=\"/static/style.css\"><script src=\"/static/htmx-lite.js\" defer></script><script src=\"/static/menu.js\" defer></script></head><body>", esc(title))
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
		renderWorkspaceSwitcher(w, user)
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
		_, _ = fmt.Fprint(w, `<section class="login-shell"><div class="login-copy"><span class="brand-mark large">T</span><h1>Tockr</h1><p>Fast, focused time tracking for small teams and Raspberry Pi deployments.</p><ul><li>Server-rendered workflows</li><li>SQLite-first operations</li><li>Kimai-inspired business structure</li></ul></div><form method="post" action="/login" class="login-card"><div><h2>Sign in</h2><p>Use your workspace credentials.</p></div>`)
		if message != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		_, _ = fmt.Fprint(w, `<label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><label>Two-factor code <input name="totp" inputmode="numeric" autocomplete="one-time-code" placeholder="Only if enabled"></label><button class="primary full">Login</button></form></section>`)
		return nil
	}))
}

func Dashboard(user *NavUser, stats map[string]int64, active *domain.Timesheet, favorites []domain.Favorite) templ.Component {
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
			_, _ = fmt.Fprintf(w, `<form method="post" action="/timesheets/start" class="compact-form"><input type="hidden" name="csrf" value="%s"><input name="customer_id" placeholder="Customer ID" required><input name="project_id" placeholder="Project ID" required><input name="activity_id" placeholder="Activity ID" required><input name="task_id" placeholder="Task ID"><input name="description" placeholder="What are you working on?"><button class="primary">Start timer</button></form>`, esc(user.CSRF))
		}
		_, _ = fmt.Fprintf(w, `</div><div class="panel"><div class="panel-head"><div><h2>Favorites</h2><p>Start repeated work without retyping IDs.</p></div></div><form method="post" action="/favorites" class="compact-form"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Name" required><input name="customer_id" placeholder="Customer ID" required><input name="project_id" placeholder="Project ID" required><input name="activity_id" placeholder="Activity ID" required><input name="task_id" placeholder="Task ID"><input name="description" placeholder="Description"><input name="tags" placeholder="Tags"><button class="table-action">Save</button></form><div class="summary-list">`, esc(user.CSRF))
		if len(favorites) == 0 {
			_, _ = fmt.Fprint(w, `<div><span>No favorites yet</span><strong>Create one from repeated work</strong></div>`)
		}
		for _, fav := range favorites {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/favorites/%d/start" class="favorite-row"><input type="hidden" name="csrf" value="%s"><span>%s</span><button class="table-action">Start</button></form>`, fav.ID, esc(user.CSRF), esc(fav.Name))
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
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Company<input name="company" value="%s"></label><label>Email<input name="email" value="%s"></label><label>Currency<input name="currency" value="%s"></label><label>Timezone<input name="timezone" value="%s"></label><label>Number<input name="number" value="%s"></label><label class="wide">Comment<textarea name="comment">%s</textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save customer</button></div></form>`,
			action, esc(user.CSRF), val(c, "Name"), val(c, "Company"), val(c, "Email"), defaultVal(val(c, "Currency"), "USD"), defaultVal(val(c, "Timezone"), "UTC"), val(c, "Number"), val(c, "Comment"))
		return nil
	})
}

func ProjectForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/projects"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label>Order<input name="order_number"></label><label>Estimate hours<input name="estimate_hours" value="0"></label><label>Budget cents<input name="budget_cents" value="0"></label><label>Alert percent<input name="budget_alert_percent" value="80"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="private"> Private</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save project</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func ActivityForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/activities"><input type="hidden" name="csrf" value="%s"><label>Project ID<input name="project_id"></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save activity</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func TaskForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/tasks"><input type="hidden" name="csrf" value="%s"><label>Project ID<input name="project_id" required></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label>Estimate hours<input name="estimate_hours" value="0"></label><label class="check"><input type="checkbox" name="visible" checked> Visible</label><label class="check"><input type="checkbox" name="billable" checked> Billable</label><div class="form-actions"><button class="primary">Save task</button></div></form>`, esc(user.CSRF))
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
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/rates"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id"></label><label>Project ID<input name="project_id"></label><label>Activity ID<input name="activity_id"></label><label>Task ID<input name="task_id"></label><label>User ID<input name="user_id"></label><label>Hourly amount cents<input name="amount_cents" required></label><label>Internal cents<input name="internal_amount_cents"></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to<input type="date" name="effective_to"></label><label class="check"><input type="checkbox" name="fixed"> Fixed rate</label><div class="form-actions"><button class="primary">Save rate</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func UserCostForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form-grid" method="post" action="/rates/costs"><input type="hidden" name="csrf" value="%s"><label>User ID<input name="user_id" required></label><label>Hourly cost cents<input name="amount_cents" required></label><label>Effective from<input type="date" name="effective_from"></label><label>Effective to<input type="date" name="effective_to"></label><div class="form-actions"><button class="primary">Save user cost</button></div></form>`, esc(user.CSRF))
		return nil
	})
}

func Rates(user *NavUser, rates []domain.Rate, costs []domain.UserCostRate) templ.Component {
	return Layout("Rates", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Rates", "Financial controls", "Date-effective billable rates and user cost rates for auditable reporting.")
		_, _ = fmt.Fprint(w, `<section class="two-col">`)
		_, _ = fmt.Fprint(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Billable rate</h2><p>Scope by customer, project, activity, task, or user.</p></div></div>`)
		if err := RateForm(user).Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</div><div class="panel form-panel"><div class="panel-head"><div><h2>User cost</h2><p>Use effective dates before profitability reporting.</p></div></div>`)
		if err := UserCostForm(user).Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</div></section>`)
		rateRows := [][]string{}
		for _, rate := range rates {
			rateRows = append(rateRows, []string{fmt.Sprint(rate.ID), ptrText(rate.CustomerID), ptrText(rate.ProjectID), ptrText(rate.ActivityID), ptrText(rate.TaskID), ptrText(rate.UserID), money(rate.AmountCents), ptrText(rate.InternalAmountCents), dateInput(&rate.EffectiveFrom), dateInput(rate.EffectiveTo)})
		}
		dataTable(w, []string{"ID", "Customer", "Project", "Activity", "Task", "User", "Bill rate", "Internal", "From", "To"}, rateRows)
		costRows := [][]string{}
		for _, cost := range costs {
			costRows = append(costRows, []string{fmt.Sprint(cost.ID), fmt.Sprint(cost.UserID), money(cost.AmountCents), dateInput(&cost.EffectiveFrom), dateInput(cost.EffectiveTo)})
		}
		_, _ = fmt.Fprint(w, `<div class="section-spacer"></div>`)
		dataTable(w, []string{"ID", "User", "Cost", "From", "To"}, costRows)
		return nil
	}))
}

func Timesheets(user *NavUser, rows [][]string) templ.Component {
	return Layout("Timesheets", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Timesheets", "Time entries", "Record work manually or use the timer from the dashboard.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Create entry</h2><p>Use customer, project, activity, and optional task IDs from the management lists.</p></div></div><form method="post" action="/timesheets" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Project ID<input name="project_id" required></label><label>Activity ID<input name="activity_id" required></label><label>Task ID<input name="task_id"></label><label>Start<input type="datetime-local" name="start" required></label><label>End<input type="datetime-local" name="end" required></label><label>Break minutes<input name="break_minutes" value="0"></label><label>Tags<input name="tags" placeholder="comma,separated"></label><label class="wide">Description<textarea name="description"></textarea></label><div class="form-actions"><button class="primary">Add entry</button></div></form></section>`, esc(user.CSRF))
		dataTable(w, []string{"ID", "Task", "Start", "End", "Duration", "Rate", "Billable", "Exported", "Description"}, rows)
		return nil
	}))
}

func Reports(user *NavUser, filter domain.ReportFilter, rows []map[string]any, saved []domain.SavedReport) templ.Component {
	return Layout("Reports", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		group := defaultVal(filter.Group, "user")
		pageHeader(w, "Reports", "Analysis", "Branch-derived activity and customer reporting, simplified for fast reads.")
		_, _ = fmt.Fprintf(w, `<section class="panel form-panel"><div class="panel-head"><div><h2>Filters</h2><p>Save repeatable reporting views.</p></div></div><form method="get" action="/reports" class="toolbar-form"><select name="group">%s%s%s%s%s%s</select><input type="date" name="begin" value="%s"><input type="date" name="end" value="%s"><input name="customer_id" placeholder="Customer ID" value="%s"><input name="project_id" placeholder="Project ID" value="%s"><input name="activity_id" placeholder="Activity ID" value="%s"><input name="task_id" placeholder="Task ID" value="%s"><button class="primary">Apply</button></form><form method="post" action="/reports/saved" class="toolbar-form"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Saved report name" required><input type="hidden" name="group" value="%s"><input type="hidden" name="begin" value="%s"><input type="hidden" name="end" value="%s"><input type="hidden" name="customer_id" value="%s"><input type="hidden" name="project_id" value="%s"><input type="hidden" name="activity_id" value="%s"><input type="hidden" name="task_id" value="%s"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="group_id" value="%s"><button class="table-action">Save report</button></form></section>`,
			reportOption(group, "user", "Users"),
			reportOption(group, "customer", "Customers"),
			reportOption(group, "project", "Projects"),
			reportOption(group, "activity", "Activities"),
			reportOption(group, "task", "Tasks"),
			reportOption(group, "group", "Groups"),
			dateInput(filter.Begin), dateInput(filter.End), idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID), idValue(filter.TaskID),
			esc(user.CSRF), esc(group), dateInput(filter.Begin), dateInput(filter.End), idValue(filter.CustomerID), idValue(filter.ProjectID), idValue(filter.ActivityID), idValue(filter.TaskID), idValue(filter.UserID), idValue(filter.GroupID))
		renderSavedReportsDropdown(w, saved)
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
		pageHeader(w, project.Name, "Project access", "Manage direct project members and group access for private work.")
		_, _ = fmt.Fprintf(w, `<section class="two-col"><div class="panel form-panel"><div class="panel-head"><div><h2>Add member</h2><p>Managers can administer this project; members can track time.</p></div></div><form method="post" action="/projects/%d/members" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>User<select name="user_id">`, project.ID, esc(user.CSRF))
		for _, u := range users {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, u.ID, esc(u.DisplayName))
		}
		_, _ = fmt.Fprintf(w, `</select></label><label>Role<select name="role"><option value="member">Member</option><option value="manager">Manager</option></select></label><div class="form-actions"><button class="primary">Save member</button></div></form></div><div class="panel form-panel"><div class="panel-head"><div><h2>Add group</h2><p>Groups make bulk access easier to maintain.</p></div></div><form method="post" action="/projects/%d/groups" class="form-grid"><input type="hidden" name="csrf" value="%s"><label>Group<select name="group_id">`, project.ID, esc(user.CSRF))
		for _, g := range groups {
			_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, g.ID, esc(g.Name))
		}
		_, _ = fmt.Fprint(w, `</select></label><div class="form-actions"><button class="primary">Add group</button></div></form></div></section>`)
		memberRows := [][]string{}
		for _, member := range members {
			memberRows = append(memberRows, []string{fmt.Sprint(member.UserID), string(member.Role), fmt.Sprintf(`<form method="post" action="/projects/%d/members/remove"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="user_id" value="%d"><button class="table-action">Remove</button></form>`, project.ID, esc(user.CSRF), member.UserID)})
		}
		dataTableRaw(w, []string{"User", "Role", "Action"}, memberRows)
		groupRows := [][]string{}
		for _, group := range assignedGroups {
			groupRows = append(groupRows, []string{fmt.Sprint(group.ID), group.Name, fmt.Sprintf(`<form method="post" action="/projects/%d/groups/remove"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="group_id" value="%d"><button class="table-action">Remove</button></form>`, project.ID, esc(user.CSRF), group.ID)})
		}
		_, _ = fmt.Fprint(w, `<div class="section-spacer"></div>`)
		dataTableRaw(w, []string{"Group", "Name", "Action"}, groupRows)
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

func renderSavedReportsDropdown(w io.Writer, saved []domain.SavedReport) {
	if len(saved) == 0 {
		return
	}
	_, _ = fmt.Fprint(w, `<div class="report-actions"><div class="dropdown" data-dropdown="saved-reports"><button class="ghost-button dropdown-trigger" type="button" data-dropdown-trigger aria-haspopup="menu" aria-expanded="false" aria-controls="saved-reports-menu">Saved reports <span class="chevron" aria-hidden="true">▾</span></button><div class="dropdown-menu" id="saved-reports-menu" role="menu" hidden data-dropdown-menu>`)
	for _, report := range saved {
		_, _ = fmt.Fprintf(w, `<a role="menuitem" href="%s"><span>%s</span><small>%s</small></a>`, esc(savedReportHref(report)), esc(report.Name), esc(report.GroupBy))
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
