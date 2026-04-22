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
	Access  domain.AccessContext
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
	r.Get("/favicon.ico", s.iconAsset("favicon.ico"))
	r.Get("/favicon-16x16.png", s.iconAsset("favicon-16x16.png"))
	r.Get("/favicon-32x32.png", s.iconAsset("favicon-32x32.png"))
	r.Get("/apple-touch-icon.png", s.iconAsset("apple-touch-icon.png"))
	r.Get("/site.webmanifest", s.iconAsset("site.webmanifest"))
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
		r.Get("/projects/{id}/dashboard", s.projectDashboard)
		r.Get("/activities", s.activities)
		r.Post("/activities", s.requirePermission(auth.PermManageMaster, s.saveActivity))
		r.Get("/tasks", s.tasks)
		r.Post("/tasks", s.requirePermission(auth.PermManageMaster, s.saveTask))
		r.Get("/tags", s.tags)
		r.Post("/tags", s.requirePermission(auth.PermTrackTime, s.saveTag))
		r.Get("/groups", s.requirePermission(auth.PermManageGroups, s.groups))
		r.Post("/groups", s.requirePermission(auth.PermManageGroups, s.saveGroup))
		r.Get("/rates", s.requirePermission(auth.PermManageRates, s.rates))
		r.Post("/rates", s.requirePermission(auth.PermManageRates, s.saveRate))
		r.Get("/timesheets", s.timesheets)
		r.Post("/timesheets", s.saveTimesheet)
		r.Post("/timesheets/start", s.startTimer)
		r.Post("/favorites", s.saveFavorite)
		r.Post("/favorites/{id}/start", s.startFavorite)
		r.Post("/timesheets/stop", s.stopTimer)
		r.Get("/reports", s.requirePermission(auth.PermViewReports, s.reports))
		r.Post("/reports/saved", s.requirePermission(auth.PermViewReports, s.saveReport))
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
		r.Get("/tasks", s.apiTasks)
		r.Get("/timesheets", s.apiTimesheets)
		r.Post("/timer/start", s.startTimer)
		r.Post("/timer/stop", s.stopTimer)
		r.Get("/invoices/{id}/download", s.requirePermission(auth.PermManageInvoices, s.apiInvoiceDownload))
		r.Patch("/invoices/{id}/meta", s.requirePermission(auth.PermManageInvoices, s.apiInvoiceMeta))
		r.Get("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.apiWebhooks))
	})
	s.router = r
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	setStaticHeaders(w, path)
	http.ServeFile(w, r, staticFile(path))
}

func (s *Server) iconAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setStaticHeaders(w, name)
		http.ServeFile(w, r, staticFile(name))
	}
}

func staticFile(name string) string {
	clean := filepath.Clean(strings.TrimPrefix(name, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") {
		return filepath.Join("web", "static", "__missing__")
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join("web", "static", clean)
	}
	for {
		candidate := filepath.Join(wd, "web", "static", clean)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return filepath.Join("web", "static", clean)
}

func setStaticHeaders(w http.ResponseWriter, path string) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	switch {
	case strings.HasSuffix(path, ".webmanifest"):
		w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	case strings.HasSuffix(path, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	case strings.HasSuffix(path, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
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
	stats, err := s.store.Dashboard(r.Context(), state.Access)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	active, err := s.store.ActiveTimer(r.Context(), state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	favorites, err := s.store.ListFavorites(r.Context(), state.Access.WorkspaceID, state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Dashboard(s.nav(r), stats, active, favorites))
}

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListCustomers(r.Context(), s.access(r), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, c := range items {
		rows = append(rows, []string{fmt.Sprint(c.ID), c.Name, c.Company, c.Email, c.Currency, boolText(c.Visible), boolText(c.Billable)})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.CustomerForm(s.nav(r), nil)
	}
	s.render(w, r, templates.EntityList[domain.Customer]("Customers", s.nav(r), []string{"ID", "Name", "Company", "Email", "Currency", "Visible", "Billable"}, rows, form))
}

func (s *Server) saveCustomer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	c := &domain.Customer{WorkspaceID: s.access(r).WorkspaceID, Name: r.FormValue("name"), Number: r.FormValue("number"), Company: r.FormValue("company"), Contact: r.FormValue("contact"), Email: r.FormValue("email"), Currency: r.FormValue("currency"), Timezone: r.FormValue("timezone"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertCustomer(r.Context(), c); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "customer", &c.ID, c.Name)
	http.Redirect(w, r, "/customers", http.StatusSeeOther)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListProjects(r.Context(), s.access(r), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, p := range items {
		rows = append(rows, []string{fmt.Sprint(p.ID), fmt.Sprint(p.CustomerID), p.Name, p.Number, boolText(p.Visible), boolText(p.Private), boolText(p.Billable), fmt.Sprintf(`<a class="table-action" href="/projects/%d/dashboard">Dashboard</a>`, p.ID)})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ProjectForm(s.nav(r))
	}
	s.render(w, r, templates.EntityListRaw("Projects", s.nav(r), []string{"ID", "Customer", "Name", "Number", "Visible", "Private", "Billable", "Insights"}, rows, form))
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p := &domain.Project{WorkspaceID: s.access(r).WorkspaceID, CustomerID: formInt(r, "customer_id"), Name: r.FormValue("name"), Number: r.FormValue("number"), OrderNo: r.FormValue("order_number"), Visible: checkbox(r, "visible"), Private: checkbox(r, "private"), Billable: checkbox(r, "billable"), EstimateSeconds: formInt(r, "estimate_hours") * 3600, BudgetCents: formInt(r, "budget_cents"), BudgetAlertPercent: formInt(r, "budget_alert_percent"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertProject(r.Context(), p); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "project", &p.ID, p.Name)
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func (s *Server) projectDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := s.store.ProjectDashboard(r.Context(), s.access(r), pathID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, templates.ProjectDashboard(s.nav(r), dashboard))
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListTasks(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, task := range items {
		rows = append(rows, []string{
			fmt.Sprint(task.ID),
			fmt.Sprint(task.ProjectID),
			task.Name,
			task.Number,
			boolText(task.Visible),
			boolText(task.Billable),
			fmt.Sprintf("%dh", task.EstimateSeconds/3600),
		})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.TaskForm(s.nav(r))
	}
	s.render(w, r, templates.EntityList[domain.Task]("Tasks", s.nav(r), []string{"ID", "Project", "Name", "Number", "Visible", "Billable", "Estimate"}, rows, form))
}

func (s *Server) saveTask(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	task := &domain.Task{
		WorkspaceID:     s.access(r).WorkspaceID,
		ProjectID:       projectID,
		Name:            r.FormValue("name"),
		Number:          r.FormValue("number"),
		Visible:         checkbox(r, "visible"),
		Billable:        checkbox(r, "billable"),
		EstimateSeconds: formInt(r, "estimate_hours") * 3600,
	}
	if err := s.store.UpsertTask(r.Context(), task); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "task", &task.ID, task.Name)
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListActivities(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
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
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ActivityForm(s.nav(r))
	}
	s.render(w, r, templates.EntityList[domain.Activity]("Activities", s.nav(r), []string{"ID", "Project", "Name", "Number", "Visible", "Billable"}, rows, form))
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var project *int64
	if value := formInt(r, "project_id"); value > 0 {
		project = &value
	}
	a := &domain.Activity{WorkspaceID: s.access(r).WorkspaceID, ProjectID: project, Name: r.FormValue("name"), Number: r.FormValue("number"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertActivity(r.Context(), a); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "activity", &a.ID, a.Name)
	http.Redirect(w, r, "/activities", http.StatusSeeOther)
}

func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context(), s.access(r).WorkspaceID)
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
	id, err := s.store.UpsertTag(r.Context(), s.access(r).WorkspaceID, r.FormValue("name"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "tag", &id, r.FormValue("name"))
	http.Redirect(w, r, "/tags", http.StatusSeeOther)
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListGroups(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, group := range groups {
		rows = append(rows, []string{fmt.Sprint(group.ID), group.Name, group.Description})
	}
	s.render(w, r, templates.EntityList[domain.Group]("Groups", s.nav(r), []string{"ID", "Name", "Description"}, rows, templates.GroupForm(s.nav(r))))
}

func (s *Server) saveGroup(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, err := s.store.CreateGroup(r.Context(), s.access(r).WorkspaceID, r.FormValue("name"), r.FormValue("description"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "group", &id, r.FormValue("name"))
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListRates(r.Context(), s.access(r).WorkspaceID)
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
		WorkspaceID:         s.access(r).WorkspaceID,
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
	filter := s.timesheetScope(r)
	filter.Page = page(r)
	filter.Size = size(r)
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
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
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	t := &domain.Timesheet{WorkspaceID: s.access(r).WorkspaceID, UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: projectID, ActivityID: formInt(r, "activity_id"), TaskID: formOptionalInt(r, "task_id"), StartedAt: start, EndedAt: &end, Timezone: s.cfg.DefaultTimezone, BreakSeconds: formInt(r, "break_minutes") * 60, Billable: true, Description: r.FormValue("description")}
	if err := s.store.CreateTimesheet(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.created", t)
	http.Redirect(w, r, "/timesheets", http.StatusSeeOther)
}

func (s *Server) startTimer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	start := time.Now().UTC()
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, start, start); err != nil {
		s.badRequest(w, r, err)
		return
	}
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	t := &domain.Timesheet{WorkspaceID: s.access(r).WorkspaceID, UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: projectID, ActivityID: formInt(r, "activity_id"), TaskID: formOptionalInt(r, "task_id"), StartedAt: start, Timezone: s.cfg.DefaultTimezone, Billable: true, Description: r.FormValue("description")}
	if err := s.store.StartTimer(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.badRequest(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "start", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.started", t)
	redirectOrJSON(w, r, "/")
}

func (s *Server) saveFavorite(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	favorite := &domain.Favorite{
		WorkspaceID: s.access(r).WorkspaceID,
		UserID:      s.state(r).User.ID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		CustomerID:  formInt(r, "customer_id"),
		ProjectID:   projectID,
		ActivityID:  formInt(r, "activity_id"),
		TaskID:      formOptionalInt(r, "task_id"),
		Description: r.FormValue("description"),
		Tags:        r.FormValue("tags"),
	}
	if favorite.Name == "" {
		s.badRequest(w, r, errors.New("favorite name is required"))
		return
	}
	if err := s.store.CreateFavorite(r.Context(), favorite); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "favorite", &favorite.ID, favorite.Name)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) startFavorite(w http.ResponseWriter, r *http.Request) {
	favorite, err := s.store.Favorite(r.Context(), s.access(r).WorkspaceID, s.state(r).User.ID, pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if favorite == nil {
		http.NotFound(w, r)
		return
	}
	if !s.canTrackProject(r, favorite.ProjectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	start := time.Now().UTC()
	t := &domain.Timesheet{WorkspaceID: favorite.WorkspaceID, UserID: s.state(r).User.ID, CustomerID: favorite.CustomerID, ProjectID: favorite.ProjectID, ActivityID: favorite.ActivityID, TaskID: favorite.TaskID, StartedAt: start, Timezone: s.cfg.DefaultTimezone, Billable: true, Description: favorite.Description}
	if err := s.store.StartTimer(r.Context(), t, splitCSV(favorite.Tags)); err != nil {
		s.badRequest(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "start", "favorite", &favorite.ID, favorite.Name)
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.started", t)
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
		s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.stopped", t)
	}
	redirectOrJSON(w, r, "/")
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	filter := domain.ReportFilter{
		Group:      defaultString(r.URL.Query().Get("group"), "user"),
		CustomerID: int64Param(r, "customer_id"),
		ProjectID:  int64Param(r, "project_id"),
		ActivityID: int64Param(r, "activity_id"),
		TaskID:     int64Param(r, "task_id"),
		UserID:     int64Param(r, "user_id"),
		GroupID:    int64Param(r, "group_id"),
	}
	filter.Begin = parseDateParam(r, "begin")
	filter.End = parseDateParam(r, "end")
	rows, err := s.store.ListReports(r.Context(), s.access(r), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	saved, err := s.store.ListSavedReports(r.Context(), s.access(r).WorkspaceID, s.state(r).User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Reports(s.nav(r), filter, rows, saved))
}

func (s *Server) saveReport(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := map[string]string{
		"begin":       r.FormValue("begin"),
		"end":         r.FormValue("end"),
		"customer_id": r.FormValue("customer_id"),
		"project_id":  r.FormValue("project_id"),
		"activity_id": r.FormValue("activity_id"),
		"task_id":     r.FormValue("task_id"),
		"user_id":     r.FormValue("user_id"),
		"group_id":    r.FormValue("group_id"),
	}
	body, err := json.Marshal(filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	report := &domain.SavedReport{
		WorkspaceID: s.access(r).WorkspaceID,
		UserID:      s.state(r).User.ID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		GroupBy:     defaultString(r.FormValue("group"), "user"),
		FiltersJSON: string(body),
		Shared:      checkbox(r, "shared"),
	}
	if report.Name == "" {
		s.badRequest(w, r, errors.New("saved report name is required"))
		return
	}
	if err := s.store.CreateSavedReport(r.Context(), report); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "saved_report", &report.ID, report.Name)
	http.Redirect(w, r, "/reports?group="+report.GroupBy, http.StatusSeeOther)
}

func (s *Server) invoices(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListInvoices(r.Context(), s.access(r).WorkspaceID, int64Param(r, "customer_id"), page(r), size(r))
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
	inv, err := s.store.CreateInvoice(r.Context(), s.access(r), s.state(r).User.ID, formInt(r, "customer_id"), begin, end.Add(24*time.Hour), formInt(r, "tax")*100)
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
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "invoice.created", inv)
	http.Redirect(w, r, "/invoices", http.StatusSeeOther)
}

func (s *Server) webhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Webhooks(s.nav(r), hooks))
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	hook := &domain.WebhookEndpoint{WorkspaceID: s.access(r).WorkspaceID, Name: r.FormValue("name"), URL: r.FormValue("url"), Secret: r.FormValue("secret"), Events: splitCSV(r.FormValue("events")), Enabled: true}
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
	items, pageInfo, err := s.store.ListCustomers(r.Context(), s.access(r), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiProjects(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListProjects(r.Context(), s.access(r), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiActivities(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListActivities(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiTasks(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListTasks(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiTimesheets(w http.ResponseWriter, r *http.Request) {
	filter := s.timesheetScope(r)
	filter.Page = page(r)
	filter.Size = size(r)
	items, pageInfo, err := s.store.ListTimesheets(r.Context(), filter)
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context(), s.access(r).WorkspaceID)
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
	if inv.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
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
	inv, err := s.store.Invoice(r.Context(), id)
	if err != nil || inv == nil {
		http.NotFound(w, r)
		return
	}
	if inv.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
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
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "invoice.meta.updated", map[string]any{"invoice_id": id, "name": name})
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
						access, err := s.store.AccessForUser(r.Context(), user.ID)
						if err == nil {
							state.Access = access
						}
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
		if !s.hasPermission(r, permission) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) hasPermission(r *http.Request, permission string) bool {
	return auth.HasPermission(s.access(r), permission)
}

func (s *Server) access(r *http.Request) domain.AccessContext {
	return s.state(r).Access
}

func (s *Server) timesheetScope(r *http.Request) sqlite.TimesheetFilter {
	access := s.access(r)
	filter := sqlite.TimesheetFilter{WorkspaceID: access.WorkspaceID}
	if access.IsWorkspaceAdmin() || access.WorkspaceRole == domain.WorkspaceRoleAnalyst {
		return filter
	}
	projectIDs := accessibleProjectIDs(access)
	if len(projectIDs) > 0 {
		filter.ProjectIDs = projectIDs
		return filter
	}
	filter.UserID = access.UserID
	return filter
}

func (s *Server) canTrackProject(r *http.Request, projectID int64) bool {
	access := s.access(r)
	if projectID == 0 {
		return true
	}
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil || project == nil || project.WorkspaceID != access.WorkspaceID {
		return false
	}
	return access.IsWorkspaceAdmin() || !project.Private || access.CanAccessProject(projectID)
}

func accessibleProjectIDs(access domain.AccessContext) []int64 {
	seen := map[int64]bool{}
	ids := []int64{}
	for id := range access.ManagedProjectIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range access.MemberProjectIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
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
	permissions := map[string]bool{}
	for _, permission := range []string{
		auth.PermAdmin,
		auth.PermManageOrg,
		auth.PermManageUsers,
		auth.PermManageMaster,
		auth.PermManageRates,
		auth.PermTrackTime,
		auth.PermViewReports,
		auth.PermManageInvoices,
		auth.PermUseAPI,
		auth.PermManageWebhooks,
		auth.PermManageGroups,
		auth.PermManageProjects,
	} {
		permissions[permission] = auth.HasPermission(state.Access, permission)
	}
	return &templates.NavUser{DisplayName: state.User.DisplayName, CSRF: state.Session.CSRFToken, CurrentPath: r.URL.Path, Permissions: permissions}
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

func (s *Server) queueEvent(ctx context.Context, workspaceID int64, event string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := s.store.QueueWebhook(ctx, workspaceID, event, body); err != nil {
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

func parseDateParam(r *http.Request, key string) *time.Time {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
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
			ptrText(t.TaskID),
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
