package templates

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/a-h/templ"

	"tockr/internal/domain"
)

type NavUser struct {
	DisplayName string
	CSRF        string
	IsAdmin     bool
}

func Layout(title string, user *NavUser, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>%s</title><link rel=\"stylesheet\" href=\"/static/style.css\"><script src=\"/static/htmx-lite.js\" defer></script></head><body>", esc(title))
		if user != nil {
			_, _ = fmt.Fprintf(w, `<header class="top"><a class="brand" href="/">Tockr</a><nav><a href="/timesheets">Timesheets</a><a href="/customers">Customers</a><a href="/projects">Projects</a><a href="/activities">Activities</a><a href="/tags">Tags</a><a href="/reports">Reports</a><a href="/invoices">Invoices</a>`)
			if user.IsAdmin {
				_, _ = fmt.Fprintf(w, `<a href="/rates">Rates</a><a href="/admin/users">Users</a><a href="/webhooks">Webhooks</a>`)
			}
			_, _ = fmt.Fprintf(w, `</nav><form method="post" action="/logout"><input type="hidden" name="csrf" value="%s"><button>%s · Logout</button></form></header>`, esc(user.CSRF), esc(user.DisplayName))
		}
		_, _ = fmt.Fprint(w, `<main>`)
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, _ = fmt.Fprint(w, `</main></body></html>`)
		return nil
	})
}

func Login(message string) templ.Component {
	return Layout("Login", nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="auth"><h1>Tockr</h1><p>Lightweight time tracking for small teams.</p>`)
		if message != "" {
			_, _ = fmt.Fprintf(w, `<div class="alert">%s</div>`, esc(message))
		}
		_, _ = fmt.Fprint(w, `<form method="post" action="/login" class="card form"><label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><button class="primary">Login</button></form></section>`)
		return nil
	}))
}

func Dashboard(user *NavUser, stats map[string]int64, active *domain.Timesheet) templ.Component {
	return Layout("Dashboard", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<h1>Dashboard</h1><section class="grid cards">`)
		card(w, "Active timers", fmt.Sprint(stats["active_timers"]))
		card(w, "Today", duration(stats["today_seconds"]))
		card(w, "Unexported billable", fmt.Sprint(stats["unexported"]))
		card(w, "Invoices", fmt.Sprint(stats["invoices"]))
		_, _ = fmt.Fprint(w, `</section><section class="panel"><h2>Timer</h2>`)
		if active != nil {
			_, _ = fmt.Fprintf(w, `<p>Running since %s</p><form method="post" action="/timesheets/stop"><input type="hidden" name="csrf" value="%s"><button class="primary">Stop timer</button></form>`, esc(active.StartedAt.Format("15:04")), esc(user.CSRF))
		} else {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/timesheets/start" class="row-form"><input type="hidden" name="csrf" value="%s"><input name="customer_id" placeholder="Customer ID" required><input name="project_id" placeholder="Project ID" required><input name="activity_id" placeholder="Activity ID" required><input name="description" placeholder="What are you working on?"><button class="primary">Start</button></form>`, esc(user.CSRF))
		}
		_, _ = fmt.Fprint(w, `</section>`)
		return nil
	}))
}

func EntityList[T any](title string, user *NavUser, headers []string, rows [][]string, form templ.Component) templ.Component {
	return Layout(title, user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<div class="split-head"><h1>%s</h1></div>`, esc(title))
		if form != nil {
			_, _ = fmt.Fprint(w, `<section class="panel">`)
			if err := form.Render(ctx, w); err != nil {
				return err
			}
			_, _ = fmt.Fprint(w, `</section>`)
		}
		table(w, headers, rows)
		return nil
	}))
}

func CustomerForm(user *NavUser, c *domain.Customer) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		action := "/customers"
		if c != nil && c.ID > 0 {
			action = fmt.Sprintf("/customers/%d", c.ID)
		}
		_, _ = fmt.Fprintf(w, `<form class="form grid-form" method="post" action="%s"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" value="%s" required></label><label>Company<input name="company" value="%s"></label><label>Email<input name="email" value="%s"></label><label>Currency<input name="currency" value="%s"></label><label>Timezone<input name="timezone" value="%s"></label><label>Number<input name="number" value="%s"></label><label class="wide">Comment<textarea name="comment">%s</textarea></label><label><input type="checkbox" name="visible" checked> Visible</label><label><input type="checkbox" name="billable" checked> Billable</label><button class="primary">Save customer</button></form>`,
			action, esc(user.CSRF), val(c, "Name"), val(c, "Company"), val(c, "Email"), defaultVal(val(c, "Currency"), "USD"), defaultVal(val(c, "Timezone"), "UTC"), val(c, "Number"), val(c, "Comment"))
		return nil
	})
}

func ProjectForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form grid-form" method="post" action="/projects"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label>Order<input name="order_number"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label><input type="checkbox" name="visible" checked> Visible</label><label><input type="checkbox" name="billable" checked> Billable</label><button class="primary">Save project</button></form>`, esc(user.CSRF))
		return nil
	})
}

func ActivityForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form grid-form" method="post" action="/activities"><input type="hidden" name="csrf" value="%s"><label>Project ID<input name="project_id"></label><label>Name<input name="name" required></label><label>Number<input name="number"></label><label class="wide">Comment<textarea name="comment"></textarea></label><label><input type="checkbox" name="visible" checked> Visible</label><label><input type="checkbox" name="billable" checked> Billable</label><button class="primary">Save activity</button></form>`, esc(user.CSRF))
		return nil
	})
}

func TagForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="row-form" method="post" action="/tags"><input type="hidden" name="csrf" value="%s"><input name="name" placeholder="Tag name" required><button class="primary">Save tag</button></form>`, esc(user.CSRF))
		return nil
	})
}

func RateForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form grid-form" method="post" action="/rates"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id"></label><label>Project ID<input name="project_id"></label><label>Activity ID<input name="activity_id"></label><label>User ID<input name="user_id"></label><label>Hourly amount cents<input name="amount_cents" required></label><label>Internal cents<input name="internal_amount_cents"></label><label><input type="checkbox" name="fixed"> Fixed rate</label><button class="primary">Save rate</button></form>`, esc(user.CSRF))
		return nil
	})
}

func Timesheets(user *NavUser, rows [][]string) templ.Component {
	return Layout("Timesheets", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<h1>Timesheets</h1><section class="panel"><form method="post" action="/timesheets" class="form grid-form"><input type="hidden" name="csrf" value="%s"><label>Customer ID<input name="customer_id" required></label><label>Project ID<input name="project_id" required></label><label>Activity ID<input name="activity_id" required></label><label>Start<input type="datetime-local" name="start" required></label><label>End<input type="datetime-local" name="end" required></label><label>Break minutes<input name="break_minutes" value="0"></label><label>Tags<input name="tags" placeholder="comma,separated"></label><label class="wide">Description<textarea name="description"></textarea></label><button class="primary">Add entry</button></form></section>`, esc(user.CSRF))
		table(w, []string{"ID", "Start", "End", "Duration", "Rate", "Billable", "Exported", "Description"}, rows)
		return nil
	}))
}

func Reports(user *NavUser, group string, rows []map[string]any) templ.Component {
	return Layout("Reports", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<h1>Reports</h1><div class="tabs"><a href="/reports?group=user">Users</a><a href="/reports?group=customer">Customers</a><a href="/reports?group=project">Projects</a><a href="/reports?group=activity">Activities</a></div><h2>%s report</h2>`, esc(strings.Title(group)))
		out := [][]string{}
		for _, row := range rows {
			out = append(out, []string{fmt.Sprint(row["name"]), fmt.Sprint(row["count"]), duration(row["seconds"].(int64)), money(row["cents"].(int64))})
		}
		table(w, []string{"Name", "Entries", "Duration", "Revenue"}, out)
		return nil
	}))
}

func Invoices(user *NavUser, invoices []domain.Invoice) templ.Component {
	return Layout("Invoices", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<h1>Invoices</h1><section class="panel"><form method="post" action="/invoices" class="row-form"><input type="hidden" name="csrf" value="%s"><input name="customer_id" placeholder="Customer ID" required><input type="date" name="begin" required><input type="date" name="end" required><input name="tax" value="0" placeholder="Tax %%"><button class="primary">Create invoice</button></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, inv := range invoices {
			rows = append(rows, []string{fmt.Sprint(inv.ID), inv.Number, fmt.Sprint(inv.CustomerID), inv.Status, money(inv.TotalCents), fmt.Sprintf(`<a href="/api/invoices/%d/download">Download</a>`, inv.ID)})
		}
		tableRaw(w, []string{"ID", "Number", "Customer", "Status", "Total", "File"}, rows)
		return nil
	}))
}

func Webhooks(user *NavUser, hooks []domain.WebhookEndpoint) templ.Component {
	return Layout("Webhooks", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<h1>Webhooks</h1><section class="panel"><form method="post" action="/webhooks" class="form grid-form"><input type="hidden" name="csrf" value="%s"><label>Name<input name="name" required></label><label>URL<input name="url" required></label><label>Secret<input name="secret" required></label><label>Events<input name="events" value="*"></label><button class="primary">Add webhook</button></form></section>`, esc(user.CSRF))
		rows := [][]string{}
		for _, hook := range hooks {
			rows = append(rows, []string{fmt.Sprint(hook.ID), hook.Name, hook.URL, strings.Join(hook.Events, ","), fmt.Sprint(hook.Enabled)})
		}
		table(w, []string{"ID", "Name", "URL", "Events", "Enabled"}, rows)
		return nil
	}))
}

func UserForm(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<form class="form grid-form" method="post" action="/admin/users"><input type="hidden" name="csrf" value="%s"><label>Email<input name="email" type="email" required></label><label>Username<input name="username" required></label><label>Name<input name="display_name" required></label><label>Password<input name="password" type="password" required></label><label>Timezone<input name="timezone" value="UTC"></label><label>Role<select name="role"><option value="user">User</option><option value="teamlead">Team lead</option><option value="admin">Admin</option><option value="superadmin">Super admin</option></select></label><button class="primary">Create user</button></form>`, esc(user.CSRF))
		return nil
	})
}

func ErrorPage(title, message string) templ.Component {
	return Layout(title, nil, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w, `<section class="auth"><h1>%s</h1><p>%s</p><a href="/">Return home</a></section>`, esc(title), esc(message))
		return nil
	}))
}

func table(w io.Writer, headers []string, rows [][]string) {
	safe := [][]string{}
	for _, row := range rows {
		out := []string{}
		for _, cell := range row {
			out = append(out, esc(cell))
		}
		safe = append(safe, out)
	}
	tableRaw(w, headers, safe)
}

func tableRaw(w io.Writer, headers []string, rows [][]string) {
	_, _ = fmt.Fprint(w, `<div class="table-wrap"><table><thead><tr>`)
	for _, header := range headers {
		_, _ = fmt.Fprintf(w, `<th>%s</th>`, esc(header))
	}
	_, _ = fmt.Fprint(w, `</tr></thead><tbody>`)
	for _, row := range rows {
		_, _ = fmt.Fprint(w, `<tr>`)
		for _, cell := range row {
			_, _ = fmt.Fprintf(w, `<td>%s</td>`, cell)
		}
		_, _ = fmt.Fprint(w, `</tr>`)
	}
	_, _ = fmt.Fprint(w, `</tbody></table></div>`)
}

func card(w io.Writer, label, value string) {
	_, _ = fmt.Fprintf(w, `<article class="card"><span>%s</span><strong>%s</strong></article>`, esc(label), esc(value))
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

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
