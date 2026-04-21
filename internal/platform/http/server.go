package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"tockr/internal/auth"
	"tockr/internal/db/sqlite"
	"tockr/internal/domain"
	"tockr/internal/platform/config"
	templates "tockr/web/templates"
)

type Server struct {
	cfg    config.Config
	store  *sqlite.Store
	log    *slog.Logger
	router chi.Router
}

type requestState struct {
	User    *domain.User
	Session *sqlite.Session
}

type contextKey string

const stateKey contextKey = "state"

func New(cfg config.Config, store *sqlite.Store, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, store: store, log: log}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(s.sessionMiddleware)
	r.Use(s.csrfMiddleware)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r.Get("/static/*", s.static)
	r.Get("/healthz", s.health)
	r.Get("/login", s.loginPage)
	r.Post("/login", s.login)
	r.Post("/logout", s.requireLogin(s.logout))
	r.Group(func(r chi.Router) {
		r.Use(s.requireLoginMiddleware)
		r.Get("/", s.dashboard)
		r.Get("/customers", s.customers)
		r.Post("/customers", s.requirePermission(auth.PermManageMaster, s.saveCustomer))
		r.Get("/projects", s.projects)
		r.Post("/projects", s.requirePermission(auth.PermManageMaster, s.saveProject))
		r.Get("/activities", s.activities)
		r.Post("/activities", s.requirePermission(auth.PermManageMaster, s.saveActivity))
		r.Get("/tags", s.tags)
		r.Post("/tags", s.requirePermission(auth.PermTrackTime, s.saveTag))
		r.Get("/rates", s.requirePermission(auth.PermManageRates, s.rates))
		r.Post("/rates", s.requirePermission(auth.PermManageRates, s.saveRate))
		r.Get("/timesheets", s.timesheets)
		r.Post("/timesheets", s.saveTimesheet)
		r.Post("/timesheets/start", s.startTimer)
		r.Post("/timesheets/stop", s.stopTimer)
		r.Get("/reports", s.reports)
		r.Get("/invoices", s.requirePermission(auth.PermManageInvoices, s.invoices))
		r.Post("/invoices", s.requirePermission(auth.PermManageInvoices, s.createInvoice))
		r.Get("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.webhooks))
		r.Post("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.createWebhook))
		r.Get("/admin/users", s.requirePermission(auth.PermManageUsers, s.users))
		r.Post("/admin/users", s.requirePermission(auth.PermManageUsers, s.createUser))
	})
	r.Route("/api", func(r chi.Router) {
		r.Use(s.requireLoginMiddleware)
		r.Get("/status", s.apiStatus)
		r.Get("/customers", s.apiCustomers)
		r.Get("/projects", s.apiProjects)
		r.Get("/activities", s.apiActivities)
		r.Get("/timesheets", s.apiTimesheets)
		r.Post("/timer/start", s.startTimer)
		r.Post("/timer/stop", s.stopTimer)
		r.Get("/invoices/{id}/download", s.apiInvoiceDownload)
		r.Patch("/invoices/{id}/meta", s.apiInvoiceMeta)
		r.Get("/webhooks", s.apiWebhooks)
	})
	s.router = r
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	http.ServeFile(w, r, filepath.Join("web", "static", filepath.Clean(path)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.Login(r.URL.Query().Get("message")))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	user, err := s.store.FindUserByEmail(r.Context(), r.FormValue("email"))
	if err != nil || user == nil || !auth.CheckPassword(user.PasswordHash, r.FormValue("password")) || !user.Enabled {
		http.Redirect(w, r, "/login?message=Invalid+credentials", http.StatusSeeOther)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, 14*24*time.Hour)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.TouchLogin(r.Context(), user.ID)
	s.store.Audit(r.Context(), &user.ID, "login", "user", &user.ID, "")
	http.SetCookie(w, s.cookie(session.ID))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	if state.Session != nil {
		_ = s.store.DeleteSession(r.Context(), state.Session.ID)
	}
	http.SetCookie(w, &http.Cookie{Name: "tockr_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	stats, err := s.store.Dashboard(r.Context(), state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	active, err := s.store.ActiveTimer(r.Context(), state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Dashboard(s.nav(r), stats, active))
}

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListCustomers(r.Context(), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, c := range items {
		rows = append(rows, []string{fmt.Sprint(c.ID), c.Name, c.Company, c.Email, c.Currency, boolText(c.Visible), boolText(c.Billable)})
	}
	s.render(w, r, templates.EntityList[domain.Customer]("Customers", s.nav(r), []string{"ID", "Name", "Company", "Email", "Currency", "Visible", "Billable"}, rows, templates.CustomerForm(s.nav(r), nil)))
}

func (s *Server) saveCustomer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	c := &domain.Customer{Name: r.FormValue("name"), Number: r.FormValue("number"), Company: r.FormValue("company"), Contact: r.FormValue("contact"), Email: r.FormValue("email"), Currency: r.FormValue("currency"), Timezone: r.FormValue("timezone"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertCustomer(r.Context(), c); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "customer", &c.ID, c.Name)
	http.Redirect(w, r, "/customers", http.StatusSeeOther)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListProjects(r.Context(), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, p := range items {
		rows = append(rows, []string{fmt.Sprint(p.ID), fmt.Sprint(p.CustomerID), p.Name, p.Number, boolText(p.Visible), boolText(p.Billable)})
	}
	s.render(w, r, templates.EntityList[domain.Project]("Projects", s.nav(r), []string{"ID", "Customer", "Name", "Number", "Visible", "Billable"}, rows, templates.ProjectForm(s.nav(r))))
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p := &domain.Project{CustomerID: formInt(r, "customer_id"), Name: r.FormValue("name"), Number: r.FormValue("number"), OrderNo: r.FormValue("order_number"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertProject(r.Context(), p); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "project", &p.ID, p.Name)
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListActivities(r.Context(), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, a := range items {
		project := ""
		if a.ProjectID != nil {
			project = fmt.Sprint(*a.ProjectID)
		}
		rows = append(rows, []string{fmt.Sprint(a.ID), project, a.Name, a.Number, boolText(a.Visible), boolText(a.Billable)})
	}
	s.render(w, r, templates.EntityList[domain.Activity]("Activities", s.nav(r), []string{"ID", "Project", "Name", "Number", "Visible", "Billable"}, rows, templates.ActivityForm(s.nav(r))))
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var project *int64
	if value := formInt(r, "project_id"); value > 0 {
		project = &value
	}
	a := &domain.Activity{ProjectID: project, Name: r.FormValue("name"), Number: r.FormValue("number"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertActivity(r.Context(), a); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "activity", &a.ID, a.Name)
	http.Redirect(w, r, "/activities", http.StatusSeeOther)
}

func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, tag := range tags {
		rows = append(rows, []string{fmt.Sprint(tag.ID), tag.Name, boolText(tag.Visible)})
	}
	s.render(w, r, templates.EntityList[domain.Tag]("Tags", s.nav(r), []string{"ID", "Name", "Visible"}, rows, templates.TagForm(s.nav(r))))
}

func (s *Server) saveTag(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, err := s.store.UpsertTag(r.Context(), r.FormValue("name"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "tag", &id, r.FormValue("name"))
	http.Redirect(w, r, "/tags", http.StatusSeeOther)
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListRates(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, rate := range rates {
		rows = append(rows, []string{fmt.Sprint(rate.ID), ptrText(rate.CustomerID), ptrText(rate.ProjectID), ptrText(rate.ActivityID), ptrText(rate.UserID), rate.Kind, fmt.Sprint(rate.AmountCents), ptrText(rate.InternalAmountCents), boolText(rate.Fixed)})
	}
	s.render(w, r, templates.EntityList[domain.Rate]("Rates", s.nav(r), []string{"ID", "Customer", "Project", "Activity", "User", "Kind", "Amount", "Internal", "Fixed"}, rows, templates.RateForm(s.nav(r))))
}

func (s *Server) saveRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	rate := &domain.Rate{
		CustomerID:          formOptionalInt(r, "customer_id"),
		ProjectID:           formOptionalInt(r, "project_id"),
		ActivityID:          formOptionalInt(r, "activity_id"),
		UserID:              formOptionalInt(r, "user_id"),
		Kind:                "hourly",
		AmountCents:         formInt(r, "amount_cents"),
		InternalAmountCents: formOptionalInt(r, "internal_amount_cents"),
		Fixed:               checkbox(r, "fixed"),
	}
	if err := s.store.UpsertRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/rates", http.StatusSeeOther)
}

func (s *Server) timesheets(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListTimesheets(r.Context(), sqlite.TimesheetFilter{UserID: s.state(r).User.ID, Page: page(r), Size: size(r)})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Timesheets(s.nav(r), timesheetRows(items)))
}

func (s *Server) saveTimesheet(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	start, end, err := parseRange(r.FormValue("start"), r.FormValue("end"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, start, end); err != nil {
		s.badRequest(w, r, err)
		return
	}
	t := &domain.Timesheet{UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: formInt(r, "project_id"), ActivityID: formInt(r, "activity_id"), StartedAt: start, EndedAt: &end, Timezone: s.cfg.DefaultTimezone, BreakSeconds: formInt(r, "break_minutes") * 60, Billable: true, Description: r.FormValue("description")}
	if err := s.store.CreateTimesheet(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), "timesheet.created", t)
	http.Redirect(w, r, "/timesheets", http.StatusSeeOther)
}

func (s *Server) startTimer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	start := time.Now().UTC()
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, start, start); err != nil {
		s.badRequest(w, r, err)
		return
	}
	t := &domain.Timesheet{UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: formInt(r, "project_id"), ActivityID: formInt(r, "activity_id"), StartedAt: start, Timezone: s.cfg.DefaultTimezone, Billable: true, Description: r.FormValue("description")}
	if err := s.store.StartTimer(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.badRequest(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "start", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), "timesheet.started", t)
	redirectOrJSON(w, r, "/")
}

func (s *Server) stopTimer(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.StopTimer(r.Context(), s.state(r).User.ID, time.Now().UTC())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if t != nil {
		uid := s.state(r).User.ID
		s.store.Audit(r.Context(), &uid, "stop", "timesheet", &t.ID, "")
		s.queueEvent(r.Context(), "timesheet.stopped", t)
	}
	redirectOrJSON(w, r, "/")
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "user"
	}
	rows, err := s.store.ListReports(r.Context(), group)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Reports(s.nav(r), group, rows))
}

func (s *Server) invoices(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListInvoices(r.Context(), int64Param(r, "customer_id"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Invoices(s.nav(r), items))
}

func (s *Server) createInvoice(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	begin, err := time.Parse("2006-01-02", r.FormValue("begin"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	end, err := time.Parse("2006-01-02", r.FormValue("end"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	inv, err := s.store.CreateInvoice(r.Context(), s.state(r).User.ID, formInt(r, "customer_id"), begin, end.Add(24*time.Hour), formInt(r, "tax")*100)
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := s.writeInvoiceFile(inv); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "invoice", &inv.ID, inv.Number)
	s.queueEvent(r.Context(), "invoice.created", inv)
	http.Redirect(w, r, "/invoices", http.StatusSeeOther)
}

func (s *Server) webhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Webhooks(s.nav(r), hooks))
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	hook := &domain.WebhookEndpoint{Name: r.FormValue("name"), URL: r.FormValue("url"), Secret: r.FormValue("secret"), Events: splitCSV(r.FormValue("events")), Enabled: true}
	if err := s.store.CreateWebhookEndpoint(r.Context(), hook); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, u := range users {
		roles := []string{}
		for _, role := range u.Roles {
			roles = append(roles, string(role))
		}
		rows = append(rows, []string{fmt.Sprint(u.ID), u.Email, u.DisplayName, strings.Join(roles, ","), boolText(u.Enabled)})
	}
	s.render(w, r, templates.EntityList[domain.User]("Users", s.nav(r), []string{"ID", "Email", "Name", "Roles", "Enabled"}, rows, templates.UserForm(s.nav(r))))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	role := domain.Role(defaultString(r.FormValue("role"), string(domain.RoleUser)))
	user := domain.User{
		Email:       r.FormValue("email"),
		Username:    r.FormValue("username"),
		DisplayName: r.FormValue("display_name"),
		Timezone:    defaultString(r.FormValue("timezone"), "UTC"),
		Enabled:     true,
	}
	if err := s.store.CreateUser(r.Context(), user, r.FormValue("password"), []domain.Role{role}); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "app": "tockr"})
}

func (s *Server) apiCustomers(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListCustomers(r.Context(), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiProjects(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListProjects(r.Context(), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiActivities(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListActivities(r.Context(), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiTimesheets(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListTimesheets(r.Context(), sqlite.TimesheetFilter{UserID: s.state(r).User.ID, Page: page(r), Size: size(r)})
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (s *Server) apiInvoiceDownload(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	inv, err := s.store.Invoice(r.Context(), id)
	if err != nil || inv == nil {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.DataDir, "invoices", inv.Filename)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, inv.Filename))
	http.ServeFile(w, r, path)
}

func (s *Server) apiInvoiceMeta(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	name := r.FormValue("name")
	if name == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.badRequest(w, r, err)
			return
		}
		name = body.Name
		r.Form.Set("value", body.Value)
	}
	if name == "" {
		s.badRequest(w, r, errors.New("metadata name is required"))
		return
	}
	if err := s.store.SetInvoiceMeta(r.Context(), id, name, r.FormValue("value")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.queueEvent(r.Context(), "invoice.meta.updated", map[string]any{"invoice_id": id, "name": name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invoice_id": id, "name": name})
}

func (s *Server) writePage(w http.ResponseWriter, data any, pageInfo domain.Page, err error) {
	if err != nil {
		s.serverError(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "page": pageInfo})
}

func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &requestState{}
		if cookie, err := r.Cookie("tockr_session"); err == nil {
			if sid, ok := s.unsign(cookie.Value); ok {
				session, err := s.store.FindSession(r.Context(), sid)
				if err == nil && session != nil {
					user, err := s.store.FindUserByID(r.Context(), session.UserID)
					if err == nil && user != nil {
						state.User = user
						state.Session = session
					}
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), stateKey, state)))
	})
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || r.URL.Path == "/login" {
			next.ServeHTTP(w, r)
			return
		}
		state := s.state(r)
		if state.Session == nil {
			http.Error(w, "missing session", http.StatusForbidden)
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			_ = r.ParseForm()
			token = r.FormValue("csrf")
		}
		if token == "" || !hmac.Equal([]byte(token), []byte(state.Session.CSRFToken)) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.state(r).User == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireLoginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.state(r).User == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.HasPermission(s.state(r).User, permission) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) state(r *http.Request) *requestState {
	state, _ := r.Context().Value(stateKey).(*requestState)
	if state == nil {
		return &requestState{}
	}
	return state
}

func (s *Server) nav(r *http.Request) *templates.NavUser {
	state := s.state(r)
	if state.User == nil || state.Session == nil {
		return nil
	}
	return &templates.NavUser{DisplayName: state.User.DisplayName, CSRF: state.Session.CSRFToken, IsAdmin: auth.HasPermission(state.User, auth.PermAdmin)}
}

func (s *Server) cookie(sessionID string) *http.Cookie {
	return &http.Cookie{Name: "tockr_session", Value: s.sign(sessionID), Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(14 * 24 * time.Hour)}
}

func (s *Server) sign(value string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	_, _ = mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return value + "." + sig
}

func (s *Server) unsign(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	expected := s.sign(parts[0])
	return parts[0], hmac.Equal([]byte(value), []byte(expected))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		s.log.Error("render failed", "err", err)
	}
}

func (s *Server) badRequest(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.log.Error("request failed", "err", err)
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (s *Server) writeInvoiceFile(inv *domain.Invoice) error {
	dir := filepath.Join(s.cfg.DataDir, "invoices")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("<!doctype html><title>%s</title><h1>%s</h1><p>Total: %.2f</p>", inv.Number, inv.Number, float64(inv.TotalCents)/100)
	return os.WriteFile(filepath.Join(dir, inv.Filename), []byte(content), 0o644)
}

func (s *Server) queueEvent(ctx context.Context, event string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := s.store.QueueWebhook(ctx, event, body); err != nil {
		s.log.Warn("queue webhook failed", "err", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func redirectOrJSON(w http.ResponseWriter, r *http.Request, location string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func page(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if value == 0 {
		return 1
	}
	return value
}

func size(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if value == 0 {
		return 25
	}
	return value
}

func pathID(r *http.Request) int64 {
	value, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return value
}

func int64Param(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return value
}

func formInt(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.FormValue(key), 10, 64)
	return value
}

func formOptionalInt(r *http.Request, key string) *int64 {
	if strings.TrimSpace(r.FormValue(key)) == "" {
		return nil
	}
	value := formInt(r, key)
	return &value
}

func checkbox(r *http.Request, key string) bool {
	value := r.FormValue(key)
	return value == "on" || value == "true" || value == "1"
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseRange(startValue, endValue string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01-02T15:04", startValue, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.ParseInLocation("2006-01-02T15:04", endValue, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("end must be after start")
	}
	return start.UTC(), end.UTC(), nil
}

func checkFuturePolicy(policy string, start, end time.Time) error {
	now := time.Now().UTC()
	switch policy {
	case "allow":
		return nil
	case "deny":
		if start.After(now) || end.After(now) {
			return errors.New("future times are not allowed")
		}
	case "end_of_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		limit := now.AddDate(0, 0, 7-weekday).Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
		if start.After(limit) || end.After(limit) {
			return errors.New("future times after this week are not allowed")
		}
	default:
		limit := now.Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
		if start.After(limit) || end.After(limit) {
			return errors.New("future times after today are not allowed")
		}
	}
	return nil
}

func timesheetRows(items []domain.Timesheet) [][]string {
	rows := [][]string{}
	for _, t := range items {
		end := ""
		if t.EndedAt != nil {
			end = templates.FormatTime(*t.EndedAt)
		}
		rows = append(rows, []string{
			fmt.Sprint(t.ID),
			templates.FormatTime(t.StartedAt),
			end,
			fmt.Sprintf("%dh %02dm", t.DurationSeconds/3600, (t.DurationSeconds%3600)/60),
			fmt.Sprintf("%.2f", float64(t.RateCents)/100),
			boolText(t.Billable),
			boolText(t.Exported),
			t.Description,
		})
	}
	return rows
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func ptrText(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
