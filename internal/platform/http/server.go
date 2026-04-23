package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
	r.Get("/reports/share/{token}", s.viewSharedReport)
	r.Post("/logout", s.requireLogin(s.logout))
	r.Group(func(r chi.Router) {
		r.Use(s.requireLoginMiddleware)
		r.Get("/", s.dashboard)
		r.Get("/admin", s.adminHome)
		r.Get("/account", s.account)
		r.Post("/account", s.updateAccount)
		r.Post("/account/password", s.updatePassword)
		r.Post("/account/totp/enable", s.enableTOTP)
		r.Post("/account/totp/disable", s.disableTOTP)
		r.Post("/workspace", s.switchWorkspace)
		r.Get("/customers", s.customers)
		r.Post("/customers", s.requirePermission(auth.PermManageMaster, s.saveCustomer))
		r.Get("/projects", s.projects)
		r.Post("/projects", s.requirePermission(auth.PermManageMaster, s.saveProject))
		r.Get("/projects/{id}/dashboard", s.projectDashboard)
		r.Get("/projects/{id}/members", s.projectMembers)
		r.Post("/projects/{id}/members", s.projectMemberSave)
		r.Post("/projects/{id}/members/remove", s.projectMemberRemove)
		r.Post("/projects/{id}/groups", s.projectGroupSave)
		r.Post("/projects/{id}/groups/remove", s.projectGroupRemove)
		r.Get("/activities", s.activities)
		r.Post("/activities", s.requirePermission(auth.PermManageMaster, s.saveActivity))
		r.Get("/tasks", s.tasks)
		r.Post("/tasks", s.requirePermission(auth.PermManageMaster, s.saveTask))
		r.Post("/tasks/{id}", s.requirePermission(auth.PermManageMaster, s.saveTask))
		r.Post("/tasks/{id}/archive", s.requirePermission(auth.PermManageMaster, s.archiveTask))
		r.Get("/tags", s.tags)
		r.Post("/tags", s.requirePermission(auth.PermTrackTime, s.saveTag))
		r.Get("/groups", s.requirePermission(auth.PermManageGroups, s.groups))
		r.Post("/groups", s.requirePermission(auth.PermManageGroups, s.saveGroup))
		r.Get("/groups/{id}/members", s.requirePermission(auth.PermManageGroups, s.groupMembers))
		r.Post("/groups/{id}/members", s.requirePermission(auth.PermManageGroups, s.groupMemberSave))
		r.Post("/groups/{id}/members/remove", s.requirePermission(auth.PermManageGroups, s.groupMemberRemove))
		r.Get("/rates", s.requirePermission(auth.PermManageRates, s.rates))
		r.Post("/rates", s.requirePermission(auth.PermManageRates, s.saveRate))
		r.Post("/rates/costs", s.requirePermission(auth.PermManageRates, s.saveUserCostRate))
		r.Get("/timesheets", s.timesheets)
		r.Get("/calendar", s.calendar)
		r.Post("/timesheets", s.saveTimesheet)
		r.Post("/timesheets/start", s.startTimer)
		r.Post("/favorites", s.saveFavorite)
		r.Post("/favorites/{id}/start", s.startFavorite)
		r.Post("/favorites/{id}", s.editFavorite)
		r.Post("/favorites/{id}/delete", s.deleteFavorite)
		r.Post("/timesheets/stop", s.stopTimer)
		r.Get("/reports", s.requirePermission(auth.PermViewReports, s.reports))
		r.Post("/reports/saved", s.requirePermission(auth.PermViewReports, s.saveReport))
		r.Post("/reports/saved/{id}", s.requirePermission(auth.PermViewReports, s.editSavedReport))
		r.Post("/reports/saved/{id}/delete", s.requirePermission(auth.PermViewReports, s.deleteSavedReport))
		r.Post("/reports/saved/{id}/share", s.requirePermission(auth.PermViewReports, s.shareSavedReport))
		r.Get("/reports/utilization", s.requirePermission(auth.PermViewReports, s.utilization))
		r.Get("/reports/export", s.requirePermission(auth.PermViewReports, s.exportReports))
		r.Get("/timesheets/export", s.exportTimesheets)
		r.Get("/admin/exchange-rates", s.requirePermission(auth.PermManageRates, s.exchangeRates))
		r.Post("/admin/exchange-rates", s.requirePermission(auth.PermManageRates, s.saveExchangeRate))
		r.Post("/admin/exchange-rates/{id}/delete", s.requirePermission(auth.PermManageRates, s.deleteExchangeRate))
		r.Get("/admin/recalculate", s.requirePermission(auth.PermManageRates, s.recalculate))
		r.Post("/admin/recalculate", s.requirePermission(auth.PermManageRates, s.applyRecalculate))
		r.Get("/invoices", s.requirePermission(auth.PermManageInvoices, s.invoices))
		r.Post("/invoices", s.requirePermission(auth.PermManageInvoices, s.createInvoice))
		r.Get("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.webhooks))
		r.Post("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.createWebhook))
		r.Get("/project-templates", s.requirePermission(auth.PermManageProjects, s.projectTemplates))
		r.Post("/project-templates", s.requirePermission(auth.PermManageProjects, s.saveProjectTemplate))
		r.Post("/project-templates/use", s.requirePermission(auth.PermManageProjects, s.useProjectTemplate))
		r.Get("/project-templates/{id}", s.requirePermission(auth.PermManageProjects, s.projectTemplateDetail))
		r.Post("/project-templates/{id}", s.requirePermission(auth.PermManageProjects, s.saveProjectTemplate))
		r.Post("/project-templates/{id}/use", s.requirePermission(auth.PermManageProjects, s.useProjectTemplate))
		r.Get("/admin/workspaces", s.requirePermission(auth.PermManageOrg, s.workspacesAdmin))
		r.Post("/admin/workspaces", s.requirePermission(auth.PermManageOrg, s.saveWorkspace))
		r.Get("/admin/workspaces/{id}", s.requirePermission(auth.PermManageOrg, s.workspaceAdminDetail))
		r.Post("/admin/workspaces/{id}", s.requirePermission(auth.PermManageOrg, s.saveWorkspace))
		r.Post("/admin/workspaces/{id}/members", s.requirePermission(auth.PermManageOrg, s.workspaceMemberSave))
		r.Post("/admin/workspaces/{id}/members/remove", s.requirePermission(auth.PermManageOrg, s.workspaceMemberRemove))
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
	if s.totpAvailable() && user.TOTPEnabled {
		code := strings.TrimSpace(r.FormValue("totp"))
		valid := auth.VerifyTOTP(user.TOTPSecret, code, time.Now().UTC())
		if !valid {
			var err error
			valid, err = s.store.UseRecoveryCode(r.Context(), user.ID, code)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		if !valid {
			http.Redirect(w, r, "/login?message=Two-factor+code+required", http.StatusSeeOther)
			return
		}
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, 0, 14*24*time.Hour)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.TouchLogin(r.Context(), user.ID)
	s.store.Audit(r.Context(), &user.ID, "login", "user", &user.ID, "")
	http.SetCookie(w, s.cookie(session.ID))
	if s.totpRequired() && !user.TOTPEnabled {
		http.Redirect(w, r, "/account?message=Two-factor+setup+is+required", http.StatusSeeOther)
		return
	}
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

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	secret := ""
	uri := ""
	if s.totpAvailable() && !state.User.TOTPEnabled {
		secret = auth.NewTOTPSecret()
		uri = auth.TOTPURI("Tockr", state.User.Email, secret)
	}
	s.render(w, r, templates.Account(s.nav(r), *state.User, s.cfg.TOTPMode, secret, uri, nil, r.URL.Query().Get("message")))
}

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) {
	if !s.hasAnyAdminAccess(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.render(w, r, templates.AdminHome(s.nav(r)))
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := s.state(r).User.ID
	if err := s.store.UpdateProfile(r.Context(), userID, r.FormValue("display_name"), r.FormValue("timezone")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "update", "account", &userID, "profile")
	http.Redirect(w, r, "/account?message=Profile+updated", http.StatusSeeOther)
}

func (s *Server) updatePassword(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	password := r.FormValue("password")
	if len(password) < 8 || password != r.FormValue("confirm") {
		http.Redirect(w, r, "/account?message=Password+confirmation+does+not+match", http.StatusSeeOther)
		return
	}
	user := s.state(r).User
	if !auth.CheckPassword(user.PasswordHash, r.FormValue("current_password")) {
		http.Redirect(w, r, "/account?message=Current+password+is+incorrect", http.StatusSeeOther)
		return
	}
	userID := user.ID
	if err := s.store.UpdatePassword(r.Context(), userID, password); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "update", "account", &userID, "password")
	http.Redirect(w, r, "/account?message=Password+updated", http.StatusSeeOther)
}

func (s *Server) enableTOTP(w http.ResponseWriter, r *http.Request) {
	if !s.totpAvailable() {
		http.Error(w, "totp disabled", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	secret := strings.TrimSpace(r.FormValue("secret"))
	if !auth.VerifyTOTP(secret, r.FormValue("code"), time.Now().UTC()) {
		http.Redirect(w, r, "/account?message=Invalid+two-factor+code", http.StatusSeeOther)
		return
	}
	codes := auth.NewRecoveryCodes(8)
	userID := s.state(r).User.ID
	if err := s.store.EnableTOTP(r.Context(), userID, secret, codes); err != nil {
		s.serverError(w, r, err)
		return
	}
	user, _ := s.store.FindUserByID(r.Context(), userID)
	s.store.Audit(r.Context(), &userID, "enable", "totp", &userID, "")
	nav := s.nav(r)
	if user != nil {
		s.render(w, r, templates.Account(nav, *user, s.cfg.TOTPMode, "", "", codes, "Two-factor authentication enabled"))
		return
	}
	http.Redirect(w, r, "/account?message=Two-factor+authentication+enabled", http.StatusSeeOther)
}

func (s *Server) disableTOTP(w http.ResponseWriter, r *http.Request) {
	if s.totpRequired() {
		http.Redirect(w, r, "/account?message=Two-factor+is+required+for+this+deployment", http.StatusSeeOther)
		return
	}
	userID := s.state(r).User.ID
	if err := s.store.DisableTOTP(r.Context(), userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "disable", "totp", &userID, "")
	http.Redirect(w, r, "/account?message=Two-factor+authentication+disabled", http.StatusSeeOther)
}

func (s *Server) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	workspaceID := formInt(r, "workspace_id")
	state := s.state(r)
	ok, err := s.store.UserCanAccessWorkspace(r.Context(), state.User.ID, workspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.store.SwitchSessionWorkspace(r.Context(), state.Session.ID, workspaceID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, safeReturn(r, "/"), http.StatusSeeOther)
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
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Dashboard(s.nav(r), stats, active, favorites, selectors))
}

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListCustomers(r.Context(), s.access(r), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, c := range items {
		rows = append(rows, []string{c.Name, c.Company, c.Email, c.Currency, boolText(c.Visible), boolText(c.Billable)})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.CustomerForm(s.nav(r), nil)
	}
	s.render(w, r, templates.EntityList[domain.Customer]("Customers", s.nav(r), []string{"Name", "Company", "Email", "Currency", "Visible", "Billable"}, rows, form))
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
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, p := range items {
		actions := `<a class="table-action" href="/projects/` + fmt.Sprint(p.ID) + `/dashboard">Dashboard</a>`
		if s.access(r).ManagesProject(p.ID) {
			actions += templates.RowActionMenu(fmt.Sprintf("project-%d-actions", p.ID), "Project actions", s.nav(r).CSRF, []templates.MenuAction{{Label: "Members", Href: fmt.Sprintf("/projects/%d/members", p.ID)}})
		}
		rows = append(rows, []string{p.Name, labelValue(selectors.CustomerLabels, p.CustomerID), p.Number, boolText(p.Visible), boolText(p.Private), boolText(p.Billable), `<div class="row-actions">` + actions + `</div>`})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ProjectForm(s.nav(r), selectors)
	}
	s.render(w, r, templates.EntityListRaw("Projects", s.nav(r), []string{"Name", "Customer", "Number", "Visible", "Private", "Billable", "Insights"}, rows, form))
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	customerID := formInt(r, "customer_id")
	if !s.customerInScope(r, customerID) {
		http.Error(w, "invalid customer", http.StatusForbidden)
		return
	}
	p := &domain.Project{WorkspaceID: s.access(r).WorkspaceID, CustomerID: customerID, Name: r.FormValue("name"), Number: r.FormValue("number"), OrderNo: r.FormValue("order_number"), Visible: checkbox(r, "visible"), Private: checkbox(r, "private"), Billable: checkbox(r, "billable"), EstimateSeconds: formInt(r, "estimate_hours") * 3600, BudgetCents: formInt(r, "budget_cents"), BudgetAlertPercent: formInt(r, "budget_alert_percent"), Comment: r.FormValue("comment")}
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

func (s *Server) projectMembers(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	members, err := s.store.ListProjectMembers(r.Context(), project.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	assignedGroups, err := s.store.ListProjectGroups(r.Context(), project.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.store.ListWorkspaceUsers(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	groups, err := s.store.ListGroups(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectMembers(s.nav(r), *project, members, users, assignedGroups, groups))
}

func (s *Server) projectMemberSave(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	role := domain.ProjectRole(r.FormValue("role"))
	if role != domain.ProjectRoleManager {
		role = domain.ProjectRoleMember
	}
	userIDs := formIntList(r, "user_id")
	if len(userIDs) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
		return
	}
	for _, userID := range userIDs {
		if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
			s.serverError(w, r, err)
			return
		} else if !ok {
			http.Error(w, "user is not in workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddProjectMember(r.Context(), project.ID, userID, role); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_members", &project.ID, fmt.Sprintf("users=%v role=%s", userIDs, role))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectMemberRemove(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userIDs := formIntList(r, "user_id")
	for _, userID := range userIDs {
		if err := s.store.RemoveProjectMember(r.Context(), project.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_members", &project.ID, fmt.Sprintf("remove_users=%v", userIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectGroupSave(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	groupIDs := formIntList(r, "group_id")
	for _, groupID := range groupIDs {
		if group, err := s.store.Group(r.Context(), groupID); err != nil {
			s.serverError(w, r, err)
			return
		} else if group == nil || group.WorkspaceID != s.access(r).WorkspaceID {
			http.Error(w, "group is outside workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddGroupToProject(r.Context(), project.ID, groupID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_groups", &project.ID, fmt.Sprintf("groups=%v", groupIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectGroupRemove(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	groupIDs := formIntList(r, "group_id")
	for _, groupID := range groupIDs {
		if err := s.store.RemoveGroupFromProject(r.Context(), project.ID, groupID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_groups", &project.ID, fmt.Sprintf("remove_groups=%v", groupIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListTasks(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	canManage := s.hasPermission(r, auth.PermManageMaster)
	s.render(w, r, templates.Tasks(s.nav(r), items, selectors, canManage))
}

func (s *Server) saveTask(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	task := &domain.Task{
		ID:              pathID(r),
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
	action := "create"
	if task.ID != 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "task", &task.ID, task.Name)
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) archiveTask(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := s.store.ArchiveTask(r.Context(), s.access(r).WorkspaceID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "archive", "task", &id, "")
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListActivities(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, a := range items {
		project := "Global"
		if a.ProjectID != nil {
			project = labelValue(selectors.ProjectLabels, *a.ProjectID)
		}
		rows = append(rows, []string{a.Name, project, a.Number, boolText(a.Visible), boolText(a.Billable)})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ActivityForm(s.nav(r), selectors)
	}
	s.render(w, r, templates.EntityList[domain.Activity]("Activities", s.nav(r), []string{"Name", "Project", "Number", "Visible", "Billable"}, rows, form))
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var project *int64
	if value := formInt(r, "project_id"); value > 0 {
		if !s.canTrackProject(r, value) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
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
		rows = append(rows, []string{tag.Name, boolText(tag.Visible)})
	}
	s.render(w, r, templates.EntityList[domain.Tag]("Tags", s.nav(r), []string{"Name", "Visible"}, rows, templates.TagForm(s.nav(r))))
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
		rows = append(rows, []string{group.Name, group.Description, fmt.Sprintf(`<a class="table-action" href="/groups/%d/members">Members</a>`, group.ID)})
	}
	s.render(w, r, templates.EntityListRaw("Groups", s.nav(r), []string{"Name", "Description", "Action"}, rows, templates.GroupForm(s.nav(r))))
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

func (s *Server) groupMembers(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	members, err := s.store.ListGroupMembers(r.Context(), group.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.store.ListWorkspaceUsers(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.GroupMembers(s.nav(r), *group, members, users))
}

func (s *Server) groupMemberSave(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	for _, userID := range formIntList(r, "user_id") {
		if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
			s.serverError(w, r, err)
			return
		} else if !ok {
			http.Error(w, "user is not in workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddUserToGroup(r.Context(), group.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "group_members", &group.ID, "bulk_add")
	http.Redirect(w, r, fmt.Sprintf("/groups/%d/members", group.ID), http.StatusSeeOther)
}

func (s *Server) groupMemberRemove(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	for _, userID := range formIntList(r, "user_id") {
		if err := s.store.RemoveUserFromGroup(r.Context(), group.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "group_members", &group.ID, "bulk_remove")
	http.Redirect(w, r, fmt.Sprintf("/groups/%d/members", group.ID), http.StatusSeeOther)
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	costs, err := s.store.ListUserCostRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, true, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Rates(s.nav(r), rates, costs, selectors))
}

func (s *Server) saveRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if err := s.validateOptionalRateScope(r); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rate := &domain.Rate{
		WorkspaceID:         s.access(r).WorkspaceID,
		CustomerID:          formOptionalInt(r, "customer_id"),
		ProjectID:           formOptionalInt(r, "project_id"),
		ActivityID:          formOptionalInt(r, "activity_id"),
		TaskID:              formOptionalInt(r, "task_id"),
		UserID:              formOptionalInt(r, "user_id"),
		Kind:                "hourly",
		AmountCents:         formInt(r, "amount_cents"),
		InternalAmountCents: formOptionalInt(r, "internal_amount_cents"),
		Fixed:               checkbox(r, "fixed"),
		EffectiveFrom:       formDateOrEpoch(r, "effective_from"),
		EffectiveTo:         formOptionalDate(r, "effective_to"),
	}
	if err := s.store.UpsertRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/rates", http.StatusSeeOther)
}

func (s *Server) saveUserCostRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
		s.serverError(w, r, err)
		return
	} else if !ok {
		http.Error(w, "user is not in workspace", http.StatusForbidden)
		return
	}
	rate := &domain.UserCostRate{
		WorkspaceID:   s.access(r).WorkspaceID,
		UserID:        userID,
		AmountCents:   formInt(r, "amount_cents"),
		EffectiveFrom: formDateOrEpoch(r, "effective_from"),
		EffectiveTo:   formOptionalDate(r, "effective_to"),
	}
	if rate.UserID == 0 || rate.AmountCents < 0 {
		s.badRequest(w, r, errors.New("valid user and cost are required"))
		return
	}
	if err := s.store.UpsertUserCostRate(r.Context(), rate); err != nil {
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
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Timesheets(s.nav(r), timesheetRows(items, selectors), selectors))
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	start := weekStart(time.Now().UTC())
	if date := parseDateParam(r, "date"); date != nil {
		start = weekStart(date.UTC())
	}
	end := start.AddDate(0, 0, 7)
	filter := s.timesheetScope(r)
	filter.Begin = &start
	filter.End = &end
	filter.Page = 1
	filter.Size = 500
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Calendar(s.nav(r), start, items))
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
	if err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "task_id")); err != nil {
		s.badRequest(w, r, err)
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
	if err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "task_id")); err != nil {
		s.badRequest(w, r, err)
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
	if err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "task_id")); err != nil {
		s.badRequest(w, r, err)
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
	selectors, err := s.selectorData(r, true, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Reports(s.nav(r), filter, rows, saved, selectors))
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
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Invoices(s.nav(r), items, selectors))
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
	customerID := formInt(r, "customer_id")
	if !s.customerInScope(r, customerID) {
		http.Error(w, "invalid customer", http.StatusForbidden)
		return
	}
	inv, err := s.store.CreateInvoice(r.Context(), s.access(r), s.state(r).User.ID, customerID, begin, end.Add(24*time.Hour), formInt(r, "tax")*100)
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

func (s *Server) projectTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjectTemplates(r.Context(), s.access(r).WorkspaceID, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for i := range items {
		full, err := s.store.ProjectTemplate(r.Context(), s.access(r).WorkspaceID, items[i].ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if full != nil {
			items[i] = *full
		}
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectTemplates(s.nav(r), items, selectors))
}

func (s *Server) projectTemplateDetail(w http.ResponseWriter, r *http.Request) {
	template, err := s.store.ProjectTemplate(r.Context(), s.access(r).WorkspaceID, pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if template == nil {
		http.NotFound(w, r)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectTemplateDetail(s.nav(r), *template, selectors))
}

func (s *Server) saveProjectTemplate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	template := projectTemplateFromForm(r)
	template.ID = pathID(r)
	template.WorkspaceID = s.access(r).WorkspaceID
	if err := s.store.UpsertProjectTemplate(r.Context(), &template); err != nil {
		s.badRequest(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_template", &template.ID, template.Name)
	http.Redirect(w, r, fmt.Sprintf("/project-templates/%d", template.ID), http.StatusSeeOther)
}

func (s *Server) useProjectTemplate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	templateID := pathID(r)
	if templateID == 0 {
		templateID = formInt(r, "template_id")
	}
	projectID, err := s.store.CreateProjectFromTemplate(r.Context(), s.access(r).WorkspaceID, templateID, formInt(r, "customer_id"), r.FormValue("project_name"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "create", "project", &projectID, fmt.Sprintf("from_template=%d", templateID))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/dashboard", projectID), http.StatusSeeOther)
}

func (s *Server) workspacesAdmin(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.store.ListOrganizationWorkspaces(r.Context(), s.access(r).OrganizationID, r.URL.Query().Get("q"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.WorkspaceAdmin(s.nav(r), workspaces))
}

func (s *Server) workspaceAdminDetail(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	members, err := s.store.ListWorkspaceMembers(r.Context(), workspace.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.organizationUsers(r.Context(), s.access(r).OrganizationID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.WorkspaceDetail(s.nav(r), *workspace, members, users))
}

func (s *Server) saveWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	workspace := &domain.Workspace{
		ID:              pathID(r),
		OrganizationID:  s.access(r).OrganizationID,
		Name:            r.FormValue("name"),
		Slug:            defaultString(r.FormValue("slug"), slugify(r.FormValue("name"))),
		Description:     r.FormValue("description"),
		DefaultCurrency: defaultString(r.FormValue("default_currency"), "USD"),
		Timezone:        defaultString(r.FormValue("timezone"), "UTC"),
		Archived:        checkbox(r, "archived"),
	}
	if workspace.ID > 0 {
		if existing, ok := s.organizationWorkspace(w, r); !ok {
			return
		} else {
			workspace.OrganizationID = existing.OrganizationID
		}
	}
	if err := s.store.UpsertWorkspace(r.Context(), workspace); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if workspace.ID > 0 && !workspace.Archived {
		actor := s.state(r).User.ID
		_ = s.store.SetWorkspaceMember(r.Context(), workspace.ID, actor, domain.WorkspaceRoleAdmin)
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace", &workspace.ID, workspace.Name)
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
}

func (s *Server) workspaceMemberSave(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	user, err := s.store.FindUserByID(r.Context(), userID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user == nil || user.OrganizationID != workspace.OrganizationID {
		http.Error(w, "user is outside organization", http.StatusForbidden)
		return
	}
	role := domain.WorkspaceRole(r.FormValue("role"))
	if err := s.store.SetWorkspaceMember(r.Context(), workspace.ID, userID, role); err != nil {
		s.serverError(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace_members", &workspace.ID, fmt.Sprintf("user=%d role=%s", userID, role))
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
}

func (s *Server) workspaceMemberRemove(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	if err := s.store.RemoveWorkspaceMember(r.Context(), workspace.ID, userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace_members", &workspace.ID, fmt.Sprintf("remove_user=%d", userID))
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
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
		rows = append(rows, []string{u.Email, u.DisplayName, strings.Join(roles, ","), boolText(u.Enabled)})
	}
	s.render(w, r, templates.EntityList[domain.User]("Users", s.nav(r), []string{"Email", "Name", "Roles", "Enabled"}, rows, templates.UserForm(s.nav(r))))
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
						access, err := s.store.AccessForUserWorkspace(r.Context(), user.ID, session.WorkspaceID)
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
		if s.mustSetupTOTP(r) {
			http.Redirect(w, r, "/account?message=Two-factor+setup+is+required", http.StatusSeeOther)
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
		if s.mustSetupTOTP(r) {
			http.Redirect(w, r, "/account?message=Two-factor+setup+is+required", http.StatusSeeOther)
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

func (s *Server) authorizedProjectManagement(w http.ResponseWriter, r *http.Request) (*domain.Project, bool) {
	projectID := pathID(r)
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil {
		s.serverError(w, r, err)
		return nil, false
	}
	if project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return nil, false
	}
	if !s.access(r).ManagesProject(projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return project, true
}

func (s *Server) organizationWorkspace(w http.ResponseWriter, r *http.Request) (*domain.Workspace, bool) {
	workspace := s.workspaceByID(w, r, pathID(r))
	if workspace == nil {
		return nil, false
	}
	if workspace.OrganizationID != s.access(r).OrganizationID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return workspace, true
}

func (s *Server) workspaceByID(w http.ResponseWriter, r *http.Request, id int64) *domain.Workspace {
	workspace, err := s.store.Workspace(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return nil
	}
	if workspace == nil {
		http.NotFound(w, r)
		return nil
	}
	return workspace
}

func (s *Server) currentWorkspaceGroup(w http.ResponseWriter, r *http.Request) (*domain.Group, bool) {
	group, err := s.store.Group(r.Context(), pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return nil, false
	}
	if group == nil {
		http.NotFound(w, r)
		return nil, false
	}
	if group.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return group, true
}

func (s *Server) organizationUsers(ctx context.Context, organizationID int64) ([]domain.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.User, 0, len(users))
	for _, user := range users {
		if user.OrganizationID == organizationID {
			filtered = append(filtered, user)
		}
	}
	return filtered, nil
}

func (s *Server) userInWorkspace(ctx context.Context, userID, workspaceID int64) (bool, error) {
	users, err := s.store.ListWorkspaceUsers(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user.ID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) selectorData(r *http.Request, includeUsers, includeGroups bool) (*templates.SelectorData, error) {
	access := s.access(r)
	customers, _, err := s.store.ListCustomers(r.Context(), access, "", 1, 100)
	if err != nil {
		return nil, err
	}
	projects, _, err := s.store.ListProjects(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	activities, _, err := s.store.ListActivities(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	tasks, _, err := s.store.ListTasks(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	data := &templates.SelectorData{
		CustomerLabels: map[int64]string{},
		ProjectLabels:  map[int64]string{},
		ActivityLabels: map[int64]string{},
		TaskLabels:     map[int64]string{},
		UserLabels:     map[int64]string{},
		GroupLabels:    map[int64]string{},
	}
	for _, customer := range customers {
		label := customerLabel(customer)
		data.CustomerLabels[customer.ID] = label
		data.Customers = append(data.Customers, templates.SelectOption{Value: customer.ID, Label: label})
	}
	for _, project := range projects {
		label := projectLabel(project, data.CustomerLabels)
		data.ProjectLabels[project.ID] = label
		data.Projects = append(data.Projects, templates.SelectOption{Value: project.ID, Label: label, Attrs: map[string]string{"customer-id": fmt.Sprint(project.CustomerID)}})
	}
	for _, activity := range activities {
		label := activityLabel(activity, data.ProjectLabels)
		data.ActivityLabels[activity.ID] = label
		attrs := map[string]string{}
		if activity.ProjectID != nil {
			attrs["project-id"] = fmt.Sprint(*activity.ProjectID)
		}
		data.Activities = append(data.Activities, templates.SelectOption{Value: activity.ID, Label: label, Attrs: attrs})
	}
	for _, task := range tasks {
		label := taskLabel(task, data.ProjectLabels)
		data.TaskLabels[task.ID] = label
		data.Tasks = append(data.Tasks, templates.SelectOption{Value: task.ID, Label: label, Attrs: map[string]string{"project-id": fmt.Sprint(task.ProjectID)}})
	}
	if includeUsers {
		users, err := s.store.ListWorkspaceUsers(r.Context(), access.WorkspaceID)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			label := userLabel(user)
			data.UserLabels[user.ID] = label
			data.Users = append(data.Users, templates.SelectOption{Value: user.ID, Label: label})
		}
	}
	if includeGroups {
		groups, err := s.store.ListGroups(r.Context(), access.WorkspaceID)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			data.GroupLabels[group.ID] = group.Name
			data.Groups = append(data.Groups, templates.SelectOption{Value: group.ID, Label: group.Name})
		}
	}
	return data, nil
}

func customerLabel(customer domain.Customer) string {
	parts := []string{customer.Name}
	if customer.Number != "" {
		parts = append(parts, customer.Number)
	}
	if customer.Company != "" {
		parts = append(parts, customer.Company)
	}
	return strings.Join(parts, " - ")
}

func projectLabel(project domain.Project, customers map[int64]string) string {
	if customer := labelValue(customers, project.CustomerID); customer != "" {
		return project.Name + " - " + customer
	}
	return project.Name
}

func activityLabel(activity domain.Activity, projects map[int64]string) string {
	if activity.ProjectID != nil {
		return activity.Name + " - " + labelValue(projects, *activity.ProjectID)
	}
	return activity.Name + " - Global"
}

func taskLabel(task domain.Task, projects map[int64]string) string {
	return task.Name + " - " + labelValue(projects, task.ProjectID)
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

func labelValue(labels map[int64]string, id int64) string {
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

func (s *Server) customerInScope(r *http.Request, customerID int64) bool {
	if customerID == 0 {
		return false
	}
	customer, err := s.store.Customer(r.Context(), customerID)
	return err == nil && customer != nil && customer.WorkspaceID == s.access(r).WorkspaceID
}

func (s *Server) validateWorkSelection(r *http.Request, projectID, customerID, activityID int64, taskID *int64) error {
	if projectID == 0 || customerID == 0 || activityID == 0 {
		return errors.New("customer, project, and activity are required")
	}
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil {
		return err
	}
	if project == nil || project.WorkspaceID != s.access(r).WorkspaceID || project.CustomerID != customerID || !s.canTrackProject(r, projectID) {
		return errors.New("invalid project selection")
	}
	activity, err := s.store.Activity(r.Context(), activityID)
	if err != nil {
		return err
	}
	if activity == nil || activity.WorkspaceID != s.access(r).WorkspaceID {
		return errors.New("invalid activity selection")
	}
	if activity.ProjectID != nil && *activity.ProjectID != projectID {
		return errors.New("activity does not belong to the selected project")
	}
	if taskID != nil {
		task, err := s.store.Task(r.Context(), *taskID)
		if err != nil {
			return err
		}
		if task == nil || task.WorkspaceID != s.access(r).WorkspaceID || task.ProjectID != projectID {
			return errors.New("task does not belong to the selected project")
		}
	}
	return nil
}

func (s *Server) validateOptionalRateScope(r *http.Request) error {
	customerID := formInt(r, "customer_id")
	if customerID > 0 && !s.customerInScope(r, customerID) {
		return errors.New("invalid customer selection")
	}
	projectID := formInt(r, "project_id")
	if projectID > 0 && !s.canTrackProject(r, projectID) {
		return errors.New("invalid project selection")
	}
	if projectID > 0 && customerID > 0 {
		project, err := s.store.Project(r.Context(), projectID)
		if err != nil {
			return err
		}
		if project == nil || project.CustomerID != customerID {
			return errors.New("project does not belong to the selected customer")
		}
	}
	if activityID := formInt(r, "activity_id"); activityID > 0 {
		activity, err := s.store.Activity(r.Context(), activityID)
		if err != nil {
			return err
		}
		if activity == nil || activity.WorkspaceID != s.access(r).WorkspaceID || (activity.ProjectID != nil && projectID > 0 && *activity.ProjectID != projectID) {
			return errors.New("invalid activity selection")
		}
	}
	if taskID := formInt(r, "task_id"); taskID > 0 {
		task, err := s.store.Task(r.Context(), taskID)
		if err != nil {
			return err
		}
		if task == nil || task.WorkspaceID != s.access(r).WorkspaceID || (projectID > 0 && task.ProjectID != projectID) {
			return errors.New("invalid task selection")
		}
	}
	if userID := formInt(r, "user_id"); userID > 0 {
		ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("invalid user selection")
		}
	}
	return nil
}

func (s *Server) totpAvailable() bool {
	return s.cfg.TOTPMode == "optional" || s.cfg.TOTPMode == "required"
}

func (s *Server) totpRequired() bool {
	return s.cfg.TOTPMode == "required"
}

func (s *Server) mustSetupTOTP(r *http.Request) bool {
	state := s.state(r)
	if !s.totpRequired() || state.User == nil || state.User.TOTPEnabled {
		return false
	}
	path := r.URL.Path
	return path != "/account" && path != "/account/totp/enable" && path != "/logout" && !strings.HasPrefix(path, "/static/")
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
	workspaces, _ := s.store.ListUserWorkspaces(r.Context(), state.User.ID)
	currentName := ""
	for _, workspace := range workspaces {
		if workspace.ID == state.Access.WorkspaceID {
			currentName = workspace.Name
			break
		}
	}
	if currentName == "" {
		currentName = "Workspace"
	}
	return &templates.NavUser{DisplayName: state.User.DisplayName, Email: state.User.Email, CSRF: state.Session.CSRFToken, CurrentPath: r.URL.Path, Permissions: permissions, CurrentWorkspaceID: state.Access.WorkspaceID, CurrentWorkspaceName: currentName, Workspaces: workspaces}
}

func (s *Server) hasAnyAdminAccess(r *http.Request) bool {
	for _, permission := range []string{
		auth.PermManageOrg,
		auth.PermManageRates,
		auth.PermManageProjects,
		auth.PermManageUsers,
		auth.PermManageWebhooks,
	} {
		if s.hasPermission(r, permission) {
			return true
		}
	}
	return false
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

// ─── Favorite edit/delete ──────────────────────────────────────────────────────

func (s *Server) editFavorite(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	access := s.access(r)
	userID := s.state(r).User.ID
	var taskID *int64
	if v := r.FormValue("task_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			taskID = &n
		}
	}
	if err := s.store.UpdateFavorite(r.Context(), access.WorkspaceID, userID, id,
		r.FormValue("name"), formInt(r, "project_id"), formInt(r, "activity_id"),
		taskID, r.FormValue("description"), r.FormValue("tags")); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	access := s.access(r)
	userID := s.state(r).User.ID
	if err := s.store.DeleteFavorite(r.Context(), access.WorkspaceID, userID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ─── Saved report edit/delete/share ───────────────────────────────────────────

func (s *Server) editSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	access := s.access(r)
	userID := s.state(r).User.ID
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
	if err := s.store.UpdateSavedReport(r.Context(), access.WorkspaceID, userID, id,
		r.FormValue("name"), defaultString(r.FormValue("group"), "user"), string(body), checkbox(r, "shared")); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) deleteSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	access := s.access(r)
	userID := s.state(r).User.ID
	if err := s.store.DeleteSavedReport(r.Context(), access.WorkspaceID, userID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) shareSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	access := s.access(r)
	userID := s.state(r).User.ID
	if r.FormValue("action") == "revoke" {
		if err := s.store.RevokeReportShareToken(r.Context(), access.WorkspaceID, userID, id); err != nil {
			s.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/reports", http.StatusSeeOther)
		return
	}
	token := randomToken(24)
	days := 30
	if v := r.FormValue("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	expiresAt := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	if err := s.store.SetReportShareToken(r.Context(), access.WorkspaceID, userID, id, token, expiresAt); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) viewSharedReport(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.NotFound(w, r)
		return
	}
	report, err := s.store.FindSharedReport(r.Context(), token)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if report == nil {
		http.NotFound(w, r)
		return
	}
	if report.ShareExpiresAt != nil && report.ShareExpiresAt.Before(time.Now().UTC()) {
		http.Error(w, "This shared report link has expired.", http.StatusGone)
		return
	}
	filter := domain.ReportFilter{}
	if report.FiltersJSON != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(report.FiltersJSON), &m); err == nil {
			filter.Group = m["group"]
		}
	}
	access := domain.AccessContext{WorkspaceID: report.WorkspaceID, UserID: report.UserID, WorkspaceRole: domain.WorkspaceRoleAdmin}
	rows, err := s.store.ListReports(r.Context(), access, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.SharedReport(report.Name, rows))
}

// ─── Utilization dashboard ────────────────────────────────────────────────────

func (s *Server) utilization(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var begin, end *time.Time
	if v := q.Get("begin"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			begin = &t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end = &t
		}
	}
	rows, err := s.store.UtilizationReport(r.Context(), s.access(r), begin, end)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Utilization(s.nav(r), rows, q.Get("begin"), q.Get("end")))
}

// ─── CSV exports ──────────────────────────────────────────────────────────────

func (s *Server) exportReports(w http.ResponseWriter, r *http.Request) {
	access := s.access(r)
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
	rows, err := s.store.ListReports(r.Context(), access, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Name", "Count", "TrackedSeconds", "AmountCents"})
	for _, row := range rows {
		_ = cw.Write([]string{
			fmt.Sprintf("%v", row["name"]),
			fmt.Sprintf("%v", row["count"]),
			fmt.Sprintf("%v", row["seconds"]),
			fmt.Sprintf("%v", row["cents"]),
		})
	}
	cw.Flush()
}

func (s *Server) exportTimesheets(w http.ResponseWriter, r *http.Request) {
	filter := s.timesheetScope(r)
	filter.CustomerID = int64Param(r, "customer_id")
	filter.ProjectID = int64Param(r, "project_id")
	filter.ActivityID = int64Param(r, "activity_id")
	filter.TaskID = int64Param(r, "task_id")
	filter.Begin = parseDateParam(r, "begin")
	filter.End = parseDateParam(r, "end")
	filter.Page = 1
	filter.Size = 10000
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="timesheets.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "Date", "User", "Project", "Task", "Description", "Duration", "Billable", "Rate"})
	for _, ts := range items {
		date := ""
		if !ts.StartedAt.IsZero() {
			date = ts.StartedAt.Format("2006-01-02")
		}
		dur := ""
		if ts.DurationSeconds > 0 {
			dur = strconv.FormatInt(ts.DurationSeconds, 10)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(ts.ID, 10),
			date,
			strconv.FormatInt(ts.UserID, 10),
			strconv.FormatInt(ts.ProjectID, 10),
			func() string {
				if ts.TaskID != nil {
					return strconv.FormatInt(*ts.TaskID, 10)
				}
				return ""
			}(),
			ts.Description,
			dur,
			boolText(ts.Billable),
			formatCents(ts.RateCents),
		})
	}
	cw.Flush()
}

// ─── Exchange Rates ───────────────────────────────────────────────────────────

func (s *Server) exchangeRates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListExchangeRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ExchangeRates(s.nav(r), rates))
}

func (s *Server) saveExchangeRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	from := strings.TrimSpace(r.FormValue("from_currency"))
	to := strings.TrimSpace(r.FormValue("to_currency"))
	if len(from) != 3 || len(to) != 3 {
		s.badRequest(w, r, errors.New("currency codes must be 3 letters"))
		return
	}
	rateStr := strings.TrimSpace(r.FormValue("rate"))
	rateFloat, err := strconv.ParseFloat(rateStr, 64)
	if err != nil || rateFloat <= 0 {
		s.badRequest(w, r, errors.New("rate must be a positive number"))
		return
	}
	effStr := strings.TrimSpace(r.FormValue("effective_from"))
	effDate, err := time.Parse("2006-01-02", effStr)
	if err != nil {
		s.badRequest(w, r, errors.New("effective_from must be YYYY-MM-DD"))
		return
	}
	rate := &domain.ExchangeRate{
		WorkspaceID:     s.access(r).WorkspaceID,
		FromCurrency:    strings.ToUpper(from),
		ToCurrency:      strings.ToUpper(to),
		RateThousandths: int64(rateFloat * 1000),
		EffectiveFrom:   effDate.UTC(),
	}
	if err := s.store.UpsertExchangeRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/exchange-rates", http.StatusSeeOther)
}

func (s *Server) deleteExchangeRate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteExchangeRate(r.Context(), s.access(r).WorkspaceID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/exchange-rates", http.StatusSeeOther)
}

// ─── Rate Recalculation ───────────────────────────────────────────────────────

func (s *Server) recalculate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := int64Param(r, "project_id")
	var since *time.Time
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			since = &t
		}
	}
	var preview []domain.RecalcPreviewRow
	if projectID > 0 || since != nil {
		var err error
		preview, err = s.store.RecalcPreview(r.Context(), s.access(r), projectID, since)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Recalculate(s.nav(r), preview, selectors, projectID, q.Get("since")))
}

func (s *Server) applyRecalculate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	var since *time.Time
	if v := r.FormValue("since"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			since = &t
		}
	}
	if projectID == 0 {
		s.badRequest(w, r, errors.New("project_id is required"))
		return
	}
	count, err := s.store.ApplyRecalc(r.Context(), s.access(r), projectID, since)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "recalculate", "rates", nil, fmt.Sprintf("updated %d timesheets", count))
	http.Redirect(w, r, fmt.Sprintf("/admin/recalculate?project_id=%d&since=%s&applied=%d",
		projectID, r.FormValue("since"), count), http.StatusSeeOther)
}

func (s *Server) writeInvoiceFile(inv *domain.Invoice) error {
	ctx := context.Background()
	detail, err := s.store.InvoiceDetails(ctx, inv.ID)
	if err != nil || detail == nil {
		detail = &domain.InvoiceDetail{Invoice: *inv}
	}
	dir := filepath.Join(s.cfg.DataDir, "invoices")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Invoice `)
	sb.WriteString(htmlEscape(inv.Number))
	sb.WriteString(`</title><style>
body{font-family:sans-serif;max-width:800px;margin:40px auto;padding:0 20px;color:#333}
table{width:100%;border-collapse:collapse;margin:20px 0}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid #ddd}
th{background:#f5f5f5;font-weight:600}
.total-row td{font-weight:bold;border-top:2px solid #333}
.header{display:flex;justify-content:space-between;margin-bottom:40px}
.from,.to{min-width:220px}
h1{margin:0 0 4px}
.meta{color:#666;font-size:.9em}
</style></head><body>`)
	sb.WriteString(`<div class="header"><div class="from">`)
	if detail.WorkspaceName != "" {
		sb.WriteString(`<h1>`)
		sb.WriteString(htmlEscape(detail.WorkspaceName))
		sb.WriteString(`</h1>`)
	} else {
		sb.WriteString(`<h1>Invoice</h1>`)
	}
	sb.WriteString(`<div class="meta">Invoice #: `)
	sb.WriteString(htmlEscape(inv.Number))
	sb.WriteString(`<br>Date: `)
	sb.WriteString(inv.CreatedAt.Format("2006-01-02"))
	sb.WriteString(`</div></div>`)
	sb.WriteString(`<div class="to"><strong>Bill To:</strong><br>`)
	sb.WriteString(htmlEscape(detail.Customer.Name))
	if detail.Customer.Email != "" {
		sb.WriteString(`<br>`)
		sb.WriteString(htmlEscape(detail.Customer.Email))
	}
	sb.WriteString(`</div></div>`)
	sb.WriteString(`<table><thead><tr><th>Description</th><th>Qty</th><th>Unit</th><th>Total</th></tr></thead><tbody>`)
	if len(detail.Items) > 0 {
		for _, item := range detail.Items {
			sb.WriteString(`<tr><td>`)
			sb.WriteString(htmlEscape(item.Description))
			sb.WriteString(`</td><td>`)
			sb.WriteString(fmt.Sprintf("%d", item.Quantity))
			sb.WriteString(`</td><td>`)
			sb.WriteString(formatCents(item.UnitCents))
			sb.WriteString(`</td><td>`)
			sb.WriteString(formatCents(item.TotalCents))
			sb.WriteString(`</td></tr>`)
		}
	} else {
		sb.WriteString(`<tr><td colspan="4">(no line items)</td></tr>`)
	}
	sb.WriteString(`</tbody><tfoot>`)
	if inv.TaxCents > 0 {
		sb.WriteString(`<tr><td colspan="3">Subtotal</td><td>`)
		sb.WriteString(formatCents(inv.TotalCents - inv.TaxCents))
		sb.WriteString(`</td></tr><tr><td colspan="3">Tax</td><td>`)
		sb.WriteString(formatCents(inv.TaxCents))
		sb.WriteString(`</td></tr>`)
	}
	sb.WriteString(`<tr class="total-row"><td colspan="3">Total</td><td>`)
	sb.WriteString(formatCents(inv.TotalCents))
	sb.WriteString(`</td></tr></tfoot></table>`)
	sb.WriteString(`</body></html>`)
	return os.WriteFile(filepath.Join(dir, inv.Filename), []byte(sb.String()), 0o644)
}

func randomToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func formatCents(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
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

func safeReturn(r *http.Request, fallback string) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return fallback
	}
	if parsed, err := url.Parse(ref); err == nil && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/") {
		return parsed.RequestURI()
	}
	if parsed, err := url.Parse(ref); err == nil && parsed.Host == r.Host && strings.HasPrefix(parsed.Path, "/") {
		return parsed.RequestURI()
	}
	return fallback
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

func formIntList(r *http.Request, key string) []int64 {
	raw := r.Form[key]
	out := make([]int64, 0, len(raw))
	seen := map[int64]bool{}
	for _, value := range raw {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func formOptionalInt(r *http.Request, key string) *int64 {
	if strings.TrimSpace(r.FormValue(key)) == "" {
		return nil
	}
	value := formInt(r, key)
	return &value
}

func formDateOrEpoch(r *http.Request, key string) time.Time {
	if value := formOptionalDate(r, key); value != nil {
		return *value
	}
	return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
}

func formOptionalDate(r *http.Request, key string) *time.Time {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
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

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	previousDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			previousDash = false
			continue
		}
		if !previousDash && b.Len() > 0 {
			b.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func projectTemplateFromForm(r *http.Request) domain.ProjectTemplate {
	return domain.ProjectTemplate{
		Name:               r.FormValue("name"),
		Description:        r.FormValue("description"),
		ProjectName:        r.FormValue("project_name"),
		ProjectNumber:      r.FormValue("project_number"),
		OrderNo:            r.FormValue("order_number"),
		Visible:            checkbox(r, "visible"),
		Private:            checkbox(r, "private"),
		Billable:           checkbox(r, "billable"),
		EstimateSeconds:    formInt(r, "estimate_hours") * 3600,
		BudgetCents:        formInt(r, "budget_cents"),
		BudgetAlertPercent: defaultInt64(formInt(r, "budget_alert_percent"), 80),
		Archived:           checkbox(r, "archived"),
		Tasks:              projectTemplateTasksFromText(r.FormValue("tasks")),
		Activities:         projectTemplateActivitiesFromText(r.FormValue("activities")),
	}
}

func projectTemplateTasksFromText(value string) []domain.ProjectTemplateTask {
	lines := splitCSV(strings.ReplaceAll(value, "\n", ","))
	tasks := make([]domain.ProjectTemplateTask, 0, len(lines))
	for _, line := range lines {
		tasks = append(tasks, domain.ProjectTemplateTask{Name: line, Visible: true, Billable: true})
	}
	return tasks
}

func projectTemplateActivitiesFromText(value string) []domain.ProjectTemplateActivity {
	lines := splitCSV(strings.ReplaceAll(value, "\n", ","))
	activities := make([]domain.ProjectTemplateActivity, 0, len(lines))
	for _, line := range lines {
		activities = append(activities, domain.ProjectTemplateActivity{Name: line, Visible: true, Billable: true})
	}
	return activities
}

func defaultInt64(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
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

func weekStart(value time.Time) time.Time {
	value = value.UTC()
	y, m, d := value.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return day.AddDate(0, 0, 1-weekday)
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

func timesheetRows(items []domain.Timesheet, selectors *templates.SelectorData) [][]string {
	rows := [][]string{}
	for _, t := range items {
		end := ""
		if t.EndedAt != nil {
			end = templates.FormatTime(*t.EndedAt)
		}
		task := ""
		if t.TaskID != nil {
			task = labelValue(selectors.TaskLabels, *t.TaskID)
		}
		rows = append(rows, []string{
			task,
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
