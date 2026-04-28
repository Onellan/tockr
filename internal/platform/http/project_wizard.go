package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"tockr/internal/domain"
	templates "tockr/web/templates"
)

const projectCreateDraftCookieName = "tockr_project_create_draft"

const (
	projectCreateStepDetails     = "details"
	projectCreateStepWorkstreams = "workstreams"
	projectCreateStepActivities  = "activities"
	projectCreateStepUsers       = "users"
)

func normalizeProjectCreateStep(value string) string {
	switch strings.TrimSpace(value) {
	case projectCreateStepWorkstreams:
		return projectCreateStepWorkstreams
	case projectCreateStepActivities:
		return projectCreateStepActivities
	case projectCreateStepUsers:
		return projectCreateStepUsers
	default:
		return projectCreateStepDetails
	}
}

func nextProjectCreateStep(step string) string {
	switch step {
	case projectCreateStepDetails:
		return projectCreateStepWorkstreams
	case projectCreateStepWorkstreams:
		return projectCreateStepActivities
	case projectCreateStepActivities:
		return projectCreateStepUsers
	default:
		return projectCreateStepUsers
	}
}

func previousProjectCreateStep(step string) string {
	switch step {
	case projectCreateStepUsers:
		return projectCreateStepActivities
	case projectCreateStepActivities:
		return projectCreateStepWorkstreams
	case projectCreateStepWorkstreams:
		return projectCreateStepDetails
	default:
		return projectCreateStepDetails
	}
}

func (s *Server) projectCreateWizardPage(w http.ResponseWriter, r *http.Request) {
	selectors, err := s.selectorData(r, true, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	step := normalizeProjectCreateStep(r.URL.Query().Get("step"))
	draft := s.projectCreateDraft(r)
	if step != projectCreateStepDetails && !draftHasProjectDetails(draft) {
		http.Redirect(w, r, "/projects/create?step="+projectCreateStepDetails, http.StatusSeeOther)
		return
	}
	s.render(w, r, templates.ProjectCreateWizard(s.nav(r), selectors, draft, step, s.popFlash(w, r)))
}

func (s *Server) projectCreateWizardSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, true, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	step := normalizeProjectCreateStep(r.FormValue("step"))
	action := strings.TrimSpace(r.FormValue("action"))
	draft := s.projectCreateDraft(r)

	if step == projectCreateStepDetails {
		draft.Project = readProjectDraftDetails(r, draft.Project, s.access(r).WorkspaceID)
	}

	redirectCurrentStep := func() {
		s.persistProjectCreateDraft(w, draft)
		http.Redirect(w, r, "/projects/create?step="+step, http.StatusSeeOther)
	}
	renderCurrentStep := func(notice templates.Notice) {
		s.persistProjectCreateDraft(w, draft)
		s.render(w, r, templates.ProjectCreateWizard(s.nav(r), selectors, draft, step, notice))
	}

	switch step {
	case projectCreateStepDetails:
		switch action {
		case "next":
			if err := validateProjectDraftDetails(draft.Project, selectors); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepWorkstreams, http.StatusSeeOther)
			return
		default:
			renderCurrentStep(templates.Notice{Kind: "error", Message: "Choose a valid action."})
			return
		}
	case projectCreateStepWorkstreams:
		switch action {
		case "back":
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepDetails, http.StatusSeeOther)
			return
		case "add-existing":
			wsID := formInt(r, "workstream_id")
			if err := addExistingDraftWorkstream(&draft, selectors, wsID, formIntAny(r, "budget", "budget_cents")); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "add-new":
			if err := addNewDraftWorkstream(&draft, r); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "remove":
			if err := removeDraftWorkstream(&draft, formInt(r, "index")); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "next":
			if err := validateProjectDraftWorkstreams(draft, selectors); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepActivities, http.StatusSeeOther)
			return
		default:
			renderCurrentStep(templates.Notice{Kind: "error", Message: "Choose a valid action."})
			return
		}
	case projectCreateStepActivities:
		switch action {
		case "back":
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepWorkstreams, http.StatusSeeOther)
			return
		case "add-existing":
			activityID := formInt(r, "activity_id")
			if err := addExistingDraftActivity(&draft, selectors, activityID); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "add-new":
			if err := addNewDraftActivity(&draft, r); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "remove":
			if err := removeDraftActivity(&draft, formInt(r, "index")); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "next":
			if err := validateProjectDraftActivities(draft, selectors); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepUsers, http.StatusSeeOther)
			return
		default:
			renderCurrentStep(templates.Notice{Kind: "error", Message: "Choose a valid action."})
			return
		}
	case projectCreateStepUsers:
		switch action {
		case "back":
			s.persistProjectCreateDraft(w, draft)
			http.Redirect(w, r, "/projects/create?step="+projectCreateStepActivities, http.StatusSeeOther)
			return
		case "add-users":
			if err := addDraftUsers(&draft, selectors, formIntList(r, "user_id"), domain.ProjectRole(r.FormValue("role"))); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "remove":
			if err := removeDraftUser(&draft, formInt(r, "user_id")); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			redirectCurrentStep()
			return
		case "submit":
			if err := validateProjectDraftAll(draft, selectors); err != nil {
				renderCurrentStep(templates.Notice{Kind: "error", Message: err.Error()})
				return
			}
			project, err := s.store.CreateProjectFromDraft(r.Context(), s.access(r).WorkspaceID, draft)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			actor := s.state(r).User.ID
			s.store.Audit(r.Context(), &actor, "create", "project", &project.ID, project.Name)
			http.SetCookie(w, s.clearProjectCreateDraftCookie())
			s.redirectWithFlash(w, r, fmt.Sprintf("/projects/%d/dashboard", project.ID), "success", "Project created")
			return
		default:
			renderCurrentStep(templates.Notice{Kind: "error", Message: "Choose a valid action."})
			return
		}
	default:
		renderCurrentStep(templates.Notice{Kind: "error", Message: "Unknown workflow step."})
	}
}

func readProjectDraftDetails(r *http.Request, current domain.Project, workspaceID int64) domain.Project {
	current.WorkspaceID = workspaceID
	current.CustomerID = formInt(r, "customer_id")
	current.Name = strings.TrimSpace(r.FormValue("name"))
	current.Number = strings.TrimSpace(r.FormValue("number"))
	current.OrderNo = strings.TrimSpace(r.FormValue("order_number"))
	current.EstimateSeconds = formInt(r, "estimate_hours") * 3600
	current.BudgetCents = formIntAny(r, "budget", "budget_cents")
	current.BudgetAlertPercent = formInt(r, "budget_alert_percent")
	current.Comment = r.FormValue("comment")
	current.Visible = checkbox(r, "visible")
	current.Private = checkbox(r, "private")
	current.Billable = checkbox(r, "billable")
	return current
}

func validateProjectDraftDetails(project domain.Project, selectors *templates.SelectorData) error {
	if project.CustomerID == 0 {
		return errors.New("Select a client before continuing")
	}
	if selectors == nil || selectors.CustomerLabels[project.CustomerID] == "" {
		return errors.New("Selected client is not available in this workspace")
	}
	if strings.TrimSpace(project.Name) == "" {
		return errors.New("Enter a project name before continuing")
	}
	if project.BudgetAlertPercent < 0 || project.BudgetAlertPercent > 100 {
		return errors.New("Budget alert percentage must be between 0 and 100")
	}
	return nil
}

func validateProjectDraftWorkstreams(draft domain.ProjectCreateDraft, selectors *templates.SelectorData) error {
	if len(draft.Workstreams) == 0 {
		return errors.New("Add at least one workstream before continuing")
	}
	for _, item := range draft.Workstreams {
		if item.ExistingWorkstreamID > 0 {
			if selectors == nil || selectors.WorkstreamLabels[item.ExistingWorkstreamID] == "" {
				return errors.New("One or more selected workstreams are no longer available")
			}
			continue
		}
		if strings.TrimSpace(item.Name) == "" {
			return errors.New("New workstreams must have a name")
		}
	}
	return nil
}

func validateProjectDraftActivities(draft domain.ProjectCreateDraft, selectors *templates.SelectorData) error {
	globalActivities := map[int64]bool{}
	if selectors != nil {
		for _, item := range selectors.Activities {
			if item.Attrs["project-id"] == "" {
				globalActivities[item.Value] = true
			}
		}
	}
	for _, item := range draft.Activities {
		if item.ExistingActivityID > 0 {
			if !globalActivities[item.ExistingActivityID] {
				return errors.New("Only global deliverables can be reused during project setup")
			}
			continue
		}
		if strings.TrimSpace(item.Name) == "" {
			return errors.New("New deliverables must have a name")
		}
	}
	return nil
}

func validateProjectDraftMembers(draft domain.ProjectCreateDraft, selectors *templates.SelectorData) error {
	for _, item := range draft.Members {
		if selectors == nil || selectors.UserLabels[item.UserID] == "" {
			return errors.New("One or more selected users are no longer available")
		}
	}
	return nil
}

func validateProjectDraftAll(draft domain.ProjectCreateDraft, selectors *templates.SelectorData) error {
	if err := validateProjectDraftDetails(draft.Project, selectors); err != nil {
		return err
	}
	if err := validateProjectDraftWorkstreams(draft, selectors); err != nil {
		return err
	}
	if err := validateProjectDraftActivities(draft, selectors); err != nil {
		return err
	}
	if err := validateProjectDraftMembers(draft, selectors); err != nil {
		return err
	}
	return nil
}

func draftHasProjectDetails(draft domain.ProjectCreateDraft) bool {
	return draft.Project.CustomerID > 0 || strings.TrimSpace(draft.Project.Name) != ""
}

func addExistingDraftWorkstream(draft *domain.ProjectCreateDraft, selectors *templates.SelectorData, workstreamID, budgetCents int64) error {
	if workstreamID == 0 {
		return errors.New("Select an existing workstream to add")
	}
	if selectors == nil || selectors.WorkstreamLabels[workstreamID] == "" {
		return errors.New("Selected workstream is not available")
	}
	for index, item := range draft.Workstreams {
		if item.ExistingWorkstreamID == workstreamID {
			draft.Workstreams[index].BudgetCents = budgetCents
			return nil
		}
	}
	draft.Workstreams = append(draft.Workstreams, domain.ProjectCreateWorkstreamDraft{ExistingWorkstreamID: workstreamID, BudgetCents: budgetCents})
	return nil
}

func addNewDraftWorkstream(draft *domain.ProjectCreateDraft, r *http.Request) error {
	name := strings.TrimSpace(r.FormValue("new_name"))
	if name == "" {
		return errors.New("Enter a workstream name")
	}
	for _, item := range draft.Workstreams {
		if item.ExistingWorkstreamID == 0 && strings.EqualFold(strings.TrimSpace(item.Name), name) {
			return errors.New("That workstream is already in the draft")
		}
	}
	draft.Workstreams = append(draft.Workstreams, domain.ProjectCreateWorkstreamDraft{
		Name:        name,
		Code:        strings.TrimSpace(r.FormValue("new_code")),
		Description: strings.TrimSpace(r.FormValue("new_description")),
		BudgetCents: formIntAny(r, "new_budget", "budget_cents"),
	})
	return nil
}

func removeDraftWorkstream(draft *domain.ProjectCreateDraft, index int64) error {
	if index < 0 || int(index) >= len(draft.Workstreams) {
		return errors.New("Workstream selection was not found")
	}
	draft.Workstreams = append(draft.Workstreams[:index], draft.Workstreams[index+1:]...)
	return nil
}

func addExistingDraftActivity(draft *domain.ProjectCreateDraft, selectors *templates.SelectorData, activityID int64) error {
	if activityID == 0 {
		return errors.New("Select an existing deliverable to add")
	}
	valid := false
	for _, item := range selectors.Activities {
		if item.Value == activityID && item.Attrs["project-id"] == "" {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("Only global deliverables can be reused during project setup")
	}
	for _, item := range draft.Activities {
		if item.ExistingActivityID == activityID {
			return nil
		}
	}
	draft.Activities = append(draft.Activities, domain.ProjectCreateActivityDraft{ExistingActivityID: activityID})
	return nil
}

func addNewDraftActivity(draft *domain.ProjectCreateDraft, r *http.Request) error {
	name := strings.TrimSpace(r.FormValue("new_name"))
	if name == "" {
		return errors.New("Enter a deliverable name")
	}
	for _, item := range draft.Activities {
		if item.ExistingActivityID == 0 && strings.EqualFold(strings.TrimSpace(item.Name), name) {
			return errors.New("That deliverable is already in the draft")
		}
	}
	draft.Activities = append(draft.Activities, domain.ProjectCreateActivityDraft{
		Name:     name,
		Number:   strings.TrimSpace(r.FormValue("new_number")),
		Comment:  strings.TrimSpace(r.FormValue("new_comment")),
		Visible:  checkbox(r, "new_visible"),
		Billable: checkbox(r, "new_billable"),
	})
	return nil
}

func removeDraftActivity(draft *domain.ProjectCreateDraft, index int64) error {
	if index < 0 || int(index) >= len(draft.Activities) {
		return errors.New("Deliverable selection was not found")
	}
	draft.Activities = append(draft.Activities[:index], draft.Activities[index+1:]...)
	return nil
}

func addDraftUsers(draft *domain.ProjectCreateDraft, selectors *templates.SelectorData, userIDs []int64, role domain.ProjectRole) error {
	if len(userIDs) == 0 {
		return errors.New("Select at least one user to add")
	}
	if role != domain.ProjectRoleManager {
		role = domain.ProjectRoleMember
	}
	byUser := map[int64]domain.ProjectCreateMemberDraft{}
	for _, item := range draft.Members {
		byUser[item.UserID] = item
	}
	for _, userID := range userIDs {
		if selectors == nil || selectors.UserLabels[userID] == "" {
			return errors.New("One or more selected users are not available")
		}
		byUser[userID] = domain.ProjectCreateMemberDraft{UserID: userID, Role: role}
	}
	merged := make([]domain.ProjectCreateMemberDraft, 0, len(byUser))
	for _, item := range byUser {
		merged = append(merged, item)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].UserID < merged[j].UserID })
	draft.Members = merged
	return nil
}

func removeDraftUser(draft *domain.ProjectCreateDraft, userID int64) error {
	if userID == 0 {
		return errors.New("User selection was not found")
	}
	filtered := draft.Members[:0]
	removed := false
	for _, item := range draft.Members {
		if item.UserID == userID {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !removed {
		return errors.New("User selection was not found")
	}
	draft.Members = filtered
	return nil
}

func (s *Server) projectCreateDraft(r *http.Request) domain.ProjectCreateDraft {
	cookie, err := r.Cookie(projectCreateDraftCookieName)
	if err != nil {
		return defaultProjectCreateDraft()
	}
	value, ok := s.unsign(cookie.Value)
	if !ok {
		return defaultProjectCreateDraft()
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return defaultProjectCreateDraft()
	}
	var draft domain.ProjectCreateDraft
	if err := json.Unmarshal(body, &draft); err != nil {
		return defaultProjectCreateDraft()
	}
	if draft.Project.BudgetAlertPercent == 0 {
		draft.Project.BudgetAlertPercent = 80
	}
	return draft
}

func defaultProjectCreateDraft() domain.ProjectCreateDraft {
	return domain.ProjectCreateDraft{Project: domain.Project{Visible: true, Billable: true, BudgetAlertPercent: 80}}
}

func (s *Server) persistProjectCreateDraft(w http.ResponseWriter, draft domain.ProjectCreateDraft) {
	body, err := json.Marshal(draft)
	if err != nil {
		return
	}
	value := base64.RawURLEncoding.EncodeToString(body)
	// #nosec G124
	http.SetCookie(w, &http.Cookie{
		Name:     projectCreateDraftCookieName,
		Value:    s.sign(value),
		Path:     "/projects/create",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(2 * time.Hour),
	})
}

func (s *Server) clearProjectCreateDraftCookie() *http.Cookie {
	// #nosec G124
	return &http.Cookie{Name: projectCreateDraftCookieName, Value: "", Path: "/projects/create", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode}
}

func projectCreateSelectedActivityOptions(selectors *templates.SelectorData) []templates.SelectOption {
	if selectors == nil {
		return nil
	}
	items := make([]templates.SelectOption, 0, len(selectors.Activities))
	for _, item := range selectors.Activities {
		if item.Attrs["project-id"] == "" {
			items = append(items, item)
		}
	}
	return items
}

func draftWorkstreamLabel(item domain.ProjectCreateWorkstreamDraft, labels map[int64]string) string {
	if item.ExistingWorkstreamID > 0 {
		return labels[item.ExistingWorkstreamID]
	}
	return item.Name
}

func draftActivityLabel(item domain.ProjectCreateActivityDraft, labels map[int64]string) string {
	if item.ExistingActivityID > 0 {
		return labels[item.ExistingActivityID]
	}
	return item.Name
}

func draftMemberLabel(item domain.ProjectCreateMemberDraft, labels map[int64]string) string {
	return labels[item.UserID]
}

func parseDraftIndex(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
