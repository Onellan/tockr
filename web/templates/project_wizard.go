package templates

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"

	"tockr/internal/domain"
)

func ProjectCreateEntryCard(user *NavUser) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprint(w, `<div class="project-wizard-entry"><p class="field-hint">Create projects with a guided 4-step workflow so details, workstreams, deliverables, and users stay together.</p><div class="form-actions"><a class="primary button-link" href="/projects/create">Start project setup</a></div></div>`)
		return nil
	})
}

func ProjectCreateWizard(user *NavUser, selectors *SelectorData, draft domain.ProjectCreateDraft, step string, notice Notice) templ.Component {
	return Layout("Create project", user, templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		pageHeader(w, "Create project", "Projects / Delivery", "Move through a focused setup flow and create the project only when everything is ready.")
		renderNotice(w, notice)
		_, _ = fmt.Fprint(w, `<div class="form-actions section-spacer"><a class="ghost-button" href="/projects">Back to projects</a></div>`)
		renderProjectWizardSteps(w, step)
		renderProjectWizardSummary(w, selectors, draft)
		switch step {
		case "workstreams":
			renderProjectWizardWorkstreams(w, user, selectors, draft)
		case "activities":
			renderProjectWizardActivities(w, user, selectors, draft)
		case "users":
			renderProjectWizardUsers(w, user, selectors, draft)
		default:
			renderProjectWizardDetails(w, user, selectors, draft)
		}
		return nil
	}))
}

func renderProjectWizardSteps(w io.Writer, current string) {
	steps := []struct {
		ID    string
		Label string
	}{
		{ID: "details", Label: "Project details"},
		{ID: "workstreams", Label: "Workstreams"},
		{ID: "activities", Label: "Deliverables"},
		{ID: "users", Label: "Assign users"},
	}
	_, _ = fmt.Fprint(w, `<ol class="wizard-steps section-spacer">`)
	for index, step := range steps {
		class := "wizard-step"
		if step.ID == current {
			class += " active"
		} else if projectWizardStepRank(step.ID) < projectWizardStepRank(current) {
			class += " complete"
		}
		_, _ = fmt.Fprintf(w, `<li class="%s"><span class="wizard-step-index">%d</span><div><strong>%s</strong><span>Step %d</span></div></li>`, class, index+1, esc(step.Label), index+1)
	}
	_, _ = fmt.Fprint(w, `</ol>`)
}

func projectWizardStepRank(step string) int {
	switch step {
	case "workstreams":
		return 2
	case "activities":
		return 3
	case "users":
		return 4
	default:
		return 1
	}
}

func renderProjectWizardSummary(w io.Writer, selectors *SelectorData, draft domain.ProjectCreateDraft) {
	customer := label(selectors.CustomerLabels, draft.Project.CustomerID)
	if customer == "" {
		customer = "Not selected"
	}
	projectName := strings.TrimSpace(draft.Project.Name)
	if projectName == "" {
		projectName = "Untitled project"
	}
	_, _ = fmt.Fprintf(w, `<section class="panel wizard-summary"><div class="panel-head"><div><h2>Current draft</h2><p>Nothing is saved yet. Final submit creates the full project in one transaction.</p></div></div><div class="summary-list"><div><span class="field-hint">Project</span><strong>%s</strong></div><div><span class="field-hint">Customer</span><strong>%s</strong></div><div><span class="field-hint">Workstreams</span><strong>%d</strong></div><div><span class="field-hint">Work types</span><strong>%d</strong></div><div><span class="field-hint">Users</span><strong>%d</strong></div></div></section>`, esc(projectName), esc(customer), len(draft.Workstreams), len(draft.Activities), len(draft.Members))
}

func renderProjectWizardDetails(w io.Writer, user *NavUser, selectors *SelectorData, draft domain.ProjectCreateDraft) {
	project := draft.Project
	visibleChecked := checkedIf(project.Visible)
	privateChecked := checkedIf(project.Private)
	billableChecked := checkedIf(project.Billable)
	_, _ = fmt.Fprintf(w, `<section class="panel form-panel section-spacer"><div class="panel-head"><div><h2>Step 1 · Project details</h2><p>Start with the core commercial and delivery details.</p></div></div><form class="form-grid project-form" method="post" action="/projects/create"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="details">`, esc(user.CSRF))
	renderSelect(w, requiredLabel("Customer")+tipHTML("The client this project is billed to. Determines the billing unit and default billing contact."), "customer_id", optionList(selectors, "customer"), project.CustomerID, true, "Select a customer", nil)
	_, _ = fmt.Fprintf(w, `<label>%s %s<input name="name" value="%s" required></label>`, requiredLabel("Name"), tipHTML("Project display name shown in timesheets, reports and invoices."), esc(project.Name))
	_, _ = fmt.Fprintf(w, `<label>Project ID %s<input name="number" value="%s" placeholder="auto-generated if blank"></label>`, tipHTML("Internal reference code (e.g. PR-000001). Leave blank to auto-generate."), esc(project.Number))
	_, _ = fmt.Fprintf(w, `<label>Order number %s<input name="order_number" value="%s"></label>`, tipHTML("Purchase order or contract reference number for invoice line items."), esc(project.OrderNo))
	_, _ = fmt.Fprintf(w, `<label>Estimate hours %s<input name="estimate_hours" value="%d"></label>`, tipHTML("Total hours budgeted. Tockr shows burn against this in the project dashboard."), project.EstimateSeconds/3600)
	_, _ = fmt.Fprintf(w, `<label>Budget %s<input name="budget" value="%d" placeholder="e.g. 10000"></label>`, tipHTML("Monetary budget in your primary billing currency unit."), project.BudgetCents)
	_, _ = fmt.Fprintf(w, `<label>Budget alert (%%) %s<input name="budget_alert_percent" value="%d"></label>`, tipHTML("Send a budget warning when this percentage of the monetary budget is consumed."), defaultInt(project.BudgetAlertPercent, 80))
	_, _ = fmt.Fprintf(w, `<label class="wide">Comment<textarea name="comment">%s</textarea></label>`, esc(project.Comment))
	_, _ = fmt.Fprint(w, `<div class="project-form-flags">`)
	_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="visible"%s> Visible</label>`, visibleChecked)
	_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="private"%s> Private</label>`, privateChecked)
	_, _ = fmt.Fprintf(w, `<label class="check"><input type="checkbox" name="billable"%s> Billable</label>`, billableChecked)
	_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary" name="action" value="next">Next</button></div></div></form></section>`)
}

func renderProjectWizardWorkstreams(w io.Writer, user *NavUser, selectors *SelectorData, draft domain.ProjectCreateDraft) {
	_, _ = fmt.Fprint(w, `<section class="two-col section-spacer">`)
	_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Step 2 · Workstreams</h2><p>Add the workstreams this project needs before anyone starts logging time.</p></div></div><form method="post" action="/projects/create" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="workstreams">`, esc(user.CSRF))
	renderSelect(w, requiredLabel("Existing workstream"), "workstream_id", optionList(selectors, "workstream"), 0, false, "Select a workstream", nil)
	_, _ = fmt.Fprint(w, `<label>Budget split<input name="budget" value="0"></label><div class="form-actions"><button class="primary" name="action" value="add-existing">Add existing workstream</button></div></form>`)
	_, _ = fmt.Fprintf(w, `<div class="section-spacer"></div><form method="post" action="/projects/create" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="workstreams"><label>%s<input name="new_name" required></label><label>Workstream ID<input name="new_code" placeholder="auto-generated if blank"></label><label>Budget split<input name="new_budget" value="0"></label><label class="wide">Description<textarea name="new_description"></textarea></label><div class="form-actions"><button class="primary" name="action" value="add-new">Create and add workstream</button></div></form></div>`, esc(user.CSRF), requiredLabel("Name"))
	_, _ = fmt.Fprint(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Selected workstreams</h2><p>Remove or adjust by re-adding an existing workstream with a new budget.</p></div></div>`)
	if len(draft.Workstreams) == 0 {
		_, _ = fmt.Fprint(w, `<div class="empty-state compact"><strong>No workstreams yet</strong><span>Add at least one workstream to continue.</span></div>`)
	} else {
		_, _ = fmt.Fprint(w, `<div class="draft-chip-list">`)
		for index, item := range draft.Workstreams {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="draft-chip"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="workstreams"><input type="hidden" name="index" value="%d"><div><strong>%s</strong><span>Budget split: %s</span></div><button class="ghost-button small" name="action" value="remove">Remove</button></form>`, esc(user.CSRF), index, esc(workstreamDraftLabel(selectors, item)), esc(Money(item.BudgetCents)))
		}
		_, _ = fmt.Fprint(w, `</div>`)
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="form-actions section-spacer"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="workstreams"><button class="ghost-button" name="action" value="back">Back</button><button class="primary" name="action" value="next">Next</button></form></section>`, esc(user.CSRF))
}

func renderProjectWizardActivities(w io.Writer, user *NavUser, selectors *SelectorData, draft domain.ProjectCreateDraft) {
	_, _ = fmt.Fprint(w, `<section class="two-col section-spacer">`)
	_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Step 3 · Deliverables</h2><p>Select reusable global deliverables or define project-specific ones inline.</p></div></div><div class="info-callout"><strong>Model note:</strong> deliverables in Tockr are project-scoped or global. They are not currently linked to individual workstreams.</div><form method="post" action="/projects/create" class="form-grid section-spacer"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="activities">`, esc(user.CSRF))
	renderGlobalActivitySelect(w, selectors)
	_, _ = fmt.Fprint(w, `<div class="form-actions"><button class="primary" name="action" value="add-existing">Add existing deliverable</button></div></form>`)
	_, _ = fmt.Fprintf(w, `<div class="section-spacer"></div><form method="post" action="/projects/create" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="activities"><label>%s<input name="new_name" required></label><label>Deliverable ID<input name="new_number" placeholder="auto-generated if blank"></label><label class="wide">Comment<textarea name="new_comment"></textarea></label><label class="check"><input type="checkbox" name="new_visible" checked> Visible</label><label class="check"><input type="checkbox" name="new_billable" checked> Billable</label><div class="form-actions"><button class="primary" name="action" value="add-new">Create and add deliverable</button></div></form></div>`, esc(user.CSRF), requiredLabel("Deliverable"))
	_, _ = fmt.Fprint(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Selected deliverables</h2><p>Global selections are copied into the project during the final submit so the project has its own linked deliverables.</p></div></div>`)
	if len(draft.Activities) == 0 {
		_, _ = fmt.Fprint(w, `<div class="empty-state compact"><strong>No deliverables yet</strong><span>Add deliverables now or continue without any if the project does not need them yet.</span></div>`)
	} else {
		_, _ = fmt.Fprint(w, `<div class="draft-chip-list">`)
		for index, item := range draft.Activities {
			mode := "New project deliverable"
			if item.ExistingActivityID > 0 {
				mode = "Copy from global deliverable"
			}
			_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="draft-chip"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="activities"><input type="hidden" name="index" value="%d"><div><strong>%s</strong><span>%s</span></div><button class="ghost-button small" name="action" value="remove">Remove</button></form>`, esc(user.CSRF), index, esc(activityDraftLabel(selectors, item)), esc(mode))
		}
		_, _ = fmt.Fprint(w, `</div>`)
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="form-actions section-spacer"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="activities"><button class="ghost-button" name="action" value="back">Back</button><button class="primary" name="action" value="next">Next</button></form></section>`, esc(user.CSRF))
}

func renderProjectWizardUsers(w io.Writer, user *NavUser, selectors *SelectorData, draft domain.ProjectCreateDraft) {
	_, _ = fmt.Fprint(w, `<section class="two-col section-spacer">`)
	_, _ = fmt.Fprintf(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Step 4 · Assign users</h2><p>Choose who should be able to work on the project and optionally set their role.</p></div></div><form method="post" action="/projects/create" class="form-grid"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="users"><label class="wide">%s<select name="user_id" multiple size="10">`, esc(user.CSRF), requiredLabel("Users"))
	for _, option := range selectors.Users {
		_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, option.Value, esc(option.Label))
	}
	_, _ = fmt.Fprint(w, `</select></label><label>Role<select name="role"><option value="member">Member</option><option value="manager">Manager</option></select></label><div class="form-actions"><button class="primary" name="action" value="add-users">Add selected users</button></div></form></div>`)
	_, _ = fmt.Fprint(w, `<div class="panel form-panel"><div class="panel-head"><div><h2>Selected users</h2><p>These assignments are created when you finish the workflow.</p></div></div>`)
	if len(draft.Members) == 0 {
		_, _ = fmt.Fprint(w, `<div class="empty-state compact"><strong>No users assigned yet</strong><span>You can finish without assigning users, or add them now.</span></div>`)
	} else {
		_, _ = fmt.Fprint(w, `<div class="draft-chip-list">`)
		for _, item := range draft.Members {
			_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="draft-chip"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="users"><input type="hidden" name="user_id" value="%d"><div><strong>%s</strong><span>%s</span></div><button class="ghost-button small" name="action" value="remove">Remove</button></form>`, esc(user.CSRF), item.UserID, esc(label(selectors.UserLabels, item.UserID)), esc(strings.Title(string(item.Role))))
		}
		_, _ = fmt.Fprint(w, `</div>`)
	}
	_, _ = fmt.Fprintf(w, `<form method="post" action="/projects/create" class="form-actions section-spacer"><input type="hidden" name="csrf" value="%s"><input type="hidden" name="step" value="users"><button class="ghost-button" name="action" value="back">Back</button><button class="primary" name="action" value="submit">Create project</button></form></section>`, esc(user.CSRF))
}

func renderGlobalActivitySelect(w io.Writer, selectors *SelectorData) {
	_, _ = fmt.Fprintf(w, `<label>%s<select name="activity_id"><option value="">Select a global deliverable</option>`, requiredLabel("Existing deliverable"))
	for _, option := range selectors.Activities {
		if option.Attrs["project-id"] != "" {
			continue
		}
		_, _ = fmt.Fprintf(w, `<option value="%d">%s</option>`, option.Value, esc(option.Label))
	}
	_, _ = fmt.Fprint(w, `</select></label>`)
}

func workstreamDraftLabel(selectors *SelectorData, item domain.ProjectCreateWorkstreamDraft) string {
	if item.ExistingWorkstreamID > 0 {
		return label(selectors.WorkstreamLabels, item.ExistingWorkstreamID)
	}
	return item.Name
}

func activityDraftLabel(selectors *SelectorData, item domain.ProjectCreateActivityDraft) string {
	if item.ExistingActivityID > 0 {
		return label(selectors.ActivityLabels, item.ExistingActivityID)
	}
	return item.Name
}

func requiredLabel(text string) string {
	return `<span>` + esc(text) + ` <span class="required-mark" aria-hidden="true">*</span><span class="sr-only"> (required)</span></span>`
}
