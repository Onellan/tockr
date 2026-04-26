package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"tockr/internal/domain"
)

func TestCoreTimesheetFlow(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatalf("missing seeded user: %v", err)
	}
	customer := &domain.Customer{Name: "Acme", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{CustomerID: customer.ID, Name: "Migration", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	activity := &domain.Activity{ProjectID: &project.ID, Name: "Build", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activity); err != nil {
		t.Fatal(err)
	}
	rate := &domain.Rate{AmountCents: 10000}
	if err := store.UpsertRate(ctx, rate); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := time.Now().UTC().Add(-time.Hour)
	entry := &domain.Timesheet{UserID: user.ID, CustomerID: customer.ID, ProjectID: project.ID, ActivityID: activity.ID, StartedAt: start, EndedAt: &end, Billable: true}
	if err := store.CreateTimesheet(ctx, entry, []string{"pi"}); err != nil {
		t.Fatal(err)
	}
	items, page, err := store.ListTimesheets(ctx, TimesheetFilter{UserID: user.ID, Page: 1, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(items) != 1 {
		t.Fatalf("expected one timesheet, got page=%#v len=%d", page, len(items))
	}
	if items[0].RateCents != 10000 {
		t.Fatalf("expected default rate, got %d", items[0].RateCents)
	}
}

func TestDefaultWorkstreamsSeededForAllWorkspaces(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}

	workspaceOne, err := store.ListWorkstreams(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceOne) != len(defaultWorkstreams) {
		t.Fatalf("workspace 1 workstream count = %d, want %d", len(workspaceOne), len(defaultWorkstreams))
	}

	workspace := &domain.Workspace{
		OrganizationID:  1,
		Name:            "Expansion Workspace",
		Slug:            "expansion-workspace",
		DefaultCurrency: "USD",
		Timezone:        "UTC",
	}
	if err := store.UpsertWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	workspaceTwo, err := store.ListWorkstreams(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceTwo) != len(defaultWorkstreams) {
		t.Fatalf("workspace %d workstream count = %d, want %d", workspace.ID, len(workspaceTwo), len(defaultWorkstreams))
	}

	got := map[string]string{}
	for _, workstream := range workspaceTwo {
		got[workstream.Name] = workstream.Description
	}
	for _, expected := range defaultWorkstreams {
		if got[expected.Name] != expected.Description {
			t.Fatalf("default workstream %q mismatch", expected.Name)
		}
	}
}

func TestHierarchyBackfillAndGroupProjectAccess(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	access, err := store.AccessForUser(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !access.IsWorkspaceAdmin() || access.OrganizationRole != domain.OrgRoleOwner || access.WorkspaceID != 1 {
		t.Fatalf("unexpected admin access: %#v", access)
	}
	if err := store.CreateUser(ctx, domain.User{
		Email:       "member@example.com",
		Username:    "member",
		DisplayName: "Member",
		Timezone:    "UTC",
		Enabled:     true,
	}, "member12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	member, err := store.FindUserByEmail(ctx, "member@example.com")
	if err != nil || member == nil {
		t.Fatal("missing member")
	}
	customer := &domain.Customer{WorkspaceID: 1, Name: "Customer", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Private", Private: true, Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	groupID, err := store.CreateGroup(ctx, 1, "Delivery", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddUserToGroup(ctx, groupID, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGroupToProject(ctx, project.ID, groupID); err != nil {
		t.Fatal(err)
	}
	access, err = store.AccessForUser(ctx, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !access.CanAccessProject(project.ID) {
		t.Fatalf("group membership should grant project access: %#v", access)
	}
}

func TestTogglInspiredPhaseOneModels(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatal("missing admin")
	}
	access, err := store.AccessForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	customer := &domain.Customer{WorkspaceID: access.WorkspaceID, Name: "Acme", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: access.WorkspaceID, CustomerID: customer.ID, Name: "Launch", Visible: true, Billable: true, EstimateSeconds: 3600, BudgetCents: 10000, BudgetAlertPercent: 50}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{WorkspaceID: access.WorkspaceID, ProjectID: project.ID, Name: "Design", Visible: true, Billable: true, EstimateSeconds: 1800}
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	tasks, page, err := store.ListTasks(ctx, access, project.ID, "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(tasks) != 1 || tasks[0].Name != "Design" {
		t.Fatalf("expected task list to include Design, got page=%#v tasks=%#v", page, tasks)
	}
	activity := &domain.Activity{WorkspaceID: access.WorkspaceID, ProjectID: &project.ID, Name: "Build", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activity); err != nil {
		t.Fatal(err)
	}
	favorite := &domain.Favorite{WorkspaceID: access.WorkspaceID, UserID: user.ID, Name: "Morning build", CustomerID: customer.ID, ProjectID: project.ID, ActivityID: activity.ID, TaskID: &task.ID, Tags: "focus"}
	if err := store.CreateFavorite(ctx, favorite); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Favorite(ctx, access.WorkspaceID, user.ID, favorite.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.TaskID == nil || *loaded.TaskID != task.ID {
		t.Fatalf("favorite should preserve task scope: %#v", loaded)
	}
	report := &domain.SavedReport{WorkspaceID: access.WorkspaceID, UserID: user.ID, Name: "Task summary", GroupBy: "task", FiltersJSON: `{"project_id":"1"}`}
	if err := store.CreateSavedReport(ctx, report); err != nil {
		t.Fatal(err)
	}
	saved, err := store.ListSavedReports(ctx, access.WorkspaceID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].GroupBy != "task" {
		t.Fatalf("expected saved task report, got %#v", saved)
	}
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := time.Now().UTC().Add(-time.Hour)
	entry := &domain.Timesheet{WorkspaceID: access.WorkspaceID, UserID: user.ID, CustomerID: customer.ID, ProjectID: project.ID, ActivityID: activity.ID, TaskID: &task.ID, StartedAt: start, EndedAt: &end, Billable: true, RateCents: 10000}
	if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListReports(ctx, access, domain.ReportFilter{Group: "task", TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Design" {
		t.Fatalf("expected task report row, got %#v", rows)
	}
	dashboard, err := store.ProjectDashboard(ctx, access, project.ID, domain.ProjectDashboardFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.EstimatePercent < 100 || !dashboard.Alert {
		t.Fatalf("expected project dashboard to flag estimate threshold, got %#v", dashboard)
	}
}

func TestProjectDashboardBreakdownsAndFilterScope(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || admin == nil {
		t.Fatal("missing admin")
	}
	if err := store.CreateUser(ctx, domain.User{Email: "teammate@example.com", Username: "teammate", DisplayName: "Teammate", Timezone: "UTC", Enabled: true}, "secret12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	teammate, err := store.FindUserByEmail(ctx, "teammate@example.com")
	if err != nil || teammate == nil {
		t.Fatal("missing teammate")
	}
	access, err := store.AccessForUser(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}

	customer := &domain.Customer{WorkspaceID: access.WorkspaceID, Name: "Acme", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: access.WorkspaceID, CustomerID: customer.ID, Name: "Delivery", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	otherProject := &domain.Project{WorkspaceID: access.WorkspaceID, CustomerID: customer.ID, Name: "Other", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, otherProject); err != nil {
		t.Fatal(err)
	}

	wsBuild := &domain.Workstream{WorkspaceID: access.WorkspaceID, Name: "Build", Visible: true}
	if err := store.UpsertWorkstream(ctx, wsBuild); err != nil {
		t.Fatal(err)
	}
	wsQA := &domain.Workstream{WorkspaceID: access.WorkspaceID, Name: "QA", Visible: true}
	if err := store.UpsertWorkstream(ctx, wsQA); err != nil {
		t.Fatal(err)
	}

	activityDev := &domain.Activity{WorkspaceID: access.WorkspaceID, ProjectID: &project.ID, Name: "Development", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activityDev); err != nil {
		t.Fatal(err)
	}
	activityReview := &domain.Activity{WorkspaceID: access.WorkspaceID, ProjectID: &project.ID, Name: "Review", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activityReview); err != nil {
		t.Fatal(err)
	}
	taskAPI := &domain.Task{WorkspaceID: access.WorkspaceID, ProjectID: project.ID, Name: "API", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, taskAPI); err != nil {
		t.Fatal(err)
	}
	taskDocs := &domain.Task{WorkspaceID: access.WorkspaceID, ProjectID: project.ID, Name: "Docs", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, taskDocs); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	addEntry := func(userID int64, projectID int64, workstreamID *int64, activityID int64, taskID *int64, durationSeconds int64, billable bool, exported bool, offsetHours int) {
		t.Helper()
		start := base.Add(time.Duration(offsetHours) * time.Hour)
		end := start.Add(time.Duration(durationSeconds) * time.Second)
		entry := &domain.Timesheet{
			WorkspaceID:     access.WorkspaceID,
			UserID:          userID,
			CustomerID:      customer.ID,
			ProjectID:       projectID,
			WorkstreamID:    workstreamID,
			ActivityID:      activityID,
			TaskID:          taskID,
			StartedAt:       start,
			EndedAt:         &end,
			DurationSeconds: durationSeconds,
			Billable:        billable,
			Exported:        exported,
			RateCents:       10000,
		}
		if err := store.CreateTimesheet(ctx, entry, nil); err != nil {
			t.Fatal(err)
		}
	}

	addEntry(admin.ID, project.ID, &wsBuild.ID, activityDev.ID, &taskAPI.ID, 7200, true, false, 0)
	addEntry(admin.ID, project.ID, &wsQA.ID, activityReview.ID, &taskDocs.ID, 3600, false, false, 3)
	addEntry(teammate.ID, project.ID, &wsBuild.ID, activityDev.ID, &taskAPI.ID, 1800, true, false, 5)
	addEntry(teammate.ID, project.ID, nil, activityReview.ID, nil, 900, true, false, 6)
	addEntry(admin.ID, otherProject.ID, &wsBuild.ID, activityDev.ID, &taskAPI.ID, 18000, true, false, 8)

	dashboard, err := store.ProjectDashboard(ctx, access, project.ID, domain.ProjectDashboardFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TrackedSeconds != 13500 {
		t.Fatalf("tracked seconds = %d, want 13500", dashboard.TrackedSeconds)
	}
	if breakdownSeconds(dashboard.WorkstreamBreakdown, "Build") != 9000 {
		t.Fatalf("workstream Build seconds = %d, want 9000", breakdownSeconds(dashboard.WorkstreamBreakdown, "Build"))
	}
	if breakdownSeconds(dashboard.WorkstreamBreakdown, "QA") != 3600 {
		t.Fatalf("workstream QA seconds = %d, want 3600", breakdownSeconds(dashboard.WorkstreamBreakdown, "QA"))
	}
	if breakdownSeconds(dashboard.WorkstreamBreakdown, "Unassigned workstream") != 900 {
		t.Fatalf("unassigned workstream seconds = %d, want 900", breakdownSeconds(dashboard.WorkstreamBreakdown, "Unassigned workstream"))
	}
	if contributionSeconds(dashboard.WorkstreamContributors, teammate.ID, "Build") != 1800 {
		t.Fatalf("teammate build contribution = %d, want 1800", contributionSeconds(dashboard.WorkstreamContributors, teammate.ID, "Build"))
	}
	if breakdownSeconds(dashboard.TaskBreakdown, "API") != 9000 {
		t.Fatalf("task API seconds = %d, want 9000", breakdownSeconds(dashboard.TaskBreakdown, "API"))
	}
	if breakdownSeconds(dashboard.TaskBreakdown, "Unassigned task") != 900 {
		t.Fatalf("unassigned task seconds = %d, want 900", breakdownSeconds(dashboard.TaskBreakdown, "Unassigned task"))
	}

	begin := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	filtered, err := store.ProjectDashboard(ctx, access, project.ID, domain.ProjectDashboardFilter{Begin: &begin, End: &end, UserID: teammate.ID, WorkstreamID: wsBuild.ID})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.TrackedSeconds != 1800 {
		t.Fatalf("filtered tracked seconds = %d, want 1800", filtered.TrackedSeconds)
	}
	if len(filtered.WorkstreamBreakdown) != 1 || filtered.WorkstreamBreakdown[0].Name != "Build" {
		t.Fatalf("filtered workstream breakdown = %#v, want one Build row", filtered.WorkstreamBreakdown)
	}
}

func breakdownSeconds(items []domain.ProjectBreakdownSlice, name string) int64 {
	for _, item := range items {
		if item.Name == name {
			return item.TrackedSeconds
		}
	}
	return 0
}

func contributionSeconds(items []domain.ProjectContributionSummary, userID int64, itemName string) int64 {
	for _, item := range items {
		if item.UserID == userID && item.ItemName == itemName {
			return item.TrackedSeconds
		}
	}
	return 0
}

func TestHistoricalRatesAndUserCostResolution(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatal("missing admin")
	}
	customer := &domain.Customer{WorkspaceID: 1, Name: "Rates", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Historical", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	activity := &domain.Activity{WorkspaceID: 1, ProjectID: &project.ID, Name: "Build", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activity); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{WorkspaceID: 1, ProjectID: project.ID, Name: "Feature", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	oldFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newFrom := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	taskTo := task.ID
	if err := store.UpsertRate(ctx, &domain.Rate{WorkspaceID: 1, TaskID: &taskTo, AmountCents: 12000, EffectiveFrom: oldFrom}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRate(ctx, &domain.Rate{WorkspaceID: 1, TaskID: &taskTo, AmountCents: 15000, EffectiveFrom: newFrom}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserCostRate(ctx, &domain.UserCostRate{WorkspaceID: 1, UserID: user.ID, AmountCents: 7000, EffectiveFrom: oldFrom}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserCostRate(ctx, &domain.UserCostRate{WorkspaceID: 1, UserID: user.ID, AmountCents: 9000, EffectiveFrom: newFrom}); err != nil {
		t.Fatal(err)
	}
	rate, cost, err := store.ResolveRateAt(ctx, 1, user.ID, customer.ID, project.ID, activity.ID, &task.ID, time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rate != 12000 || cost == nil || *cost != 7000 {
		t.Fatalf("expected old rate/cost, got rate=%d cost=%v", rate, cost)
	}
	rate, cost, err = store.ResolveRateAt(ctx, 1, user.ID, customer.ID, project.ID, activity.ID, &task.ID, time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rate != 15000 || cost == nil || *cost != 9000 {
		t.Fatalf("expected new rate/cost, got rate=%d cost=%v", rate, cost)
	}
}

func TestUpdateTimesheetRecalculatesAndReplacesTags(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SeedAdmin(ctx, "admin@example.com", "secret12345", "UTC", "USD"); err != nil {
		t.Fatal(err)
	}
	user, err := store.FindUserByEmail(ctx, "admin@example.com")
	if err != nil || user == nil {
		t.Fatal("missing admin")
	}
	customer := &domain.Customer{WorkspaceID: 1, Name: "Acme", Currency: "USD", Timezone: "UTC", Visible: true, Billable: true}
	if err := store.UpsertCustomer(ctx, customer); err != nil {
		t.Fatal(err)
	}
	project := &domain.Project{WorkspaceID: 1, CustomerID: customer.ID, Name: "Migration", Visible: true, Billable: true}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	activity := &domain.Activity{WorkspaceID: 1, ProjectID: &project.ID, Name: "Build", Visible: true, Billable: true}
	if err := store.UpsertActivity(ctx, activity); err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{WorkspaceID: 1, ProjectID: project.ID, Name: "Design", Visible: true, Billable: true}
	if err := store.UpsertTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	oldFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newFrom := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertRate(ctx, &domain.Rate{WorkspaceID: 1, TaskID: &task.ID, AmountCents: 10000, EffectiveFrom: oldFrom}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRate(ctx, &domain.Rate{WorkspaceID: 1, TaskID: &task.ID, AmountCents: 15000, EffectiveFrom: newFrom}); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 2, 12, 9, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	entry := &domain.Timesheet{WorkspaceID: 1, UserID: user.ID, CustomerID: customer.ID, ProjectID: project.ID, ActivityID: activity.ID, TaskID: &task.ID, StartedAt: start, EndedAt: &end, Billable: true, Description: "Initial", Timezone: "UTC"}
	if err := store.CreateTimesheet(ctx, entry, []string{"legacy"}); err != nil {
		t.Fatal(err)
	}
	if entry.RateCents != 10000 {
		t.Fatalf("created rate = %d, want 10000", entry.RateCents)
	}
	updatedStart := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	updatedEnd := updatedStart.Add(90 * time.Minute)
	entry.StartedAt = updatedStart
	entry.EndedAt = &updatedEnd
	entry.BreakSeconds = 15 * 60
	entry.Billable = false
	entry.Description = "Updated"
	if err := store.UpdateTimesheet(ctx, entry, []string{"qa", "review"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Timesheet(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("updated timesheet not found")
	}
	if loaded.RateCents != 15000 {
		t.Fatalf("updated rate = %d, want 15000", loaded.RateCents)
	}
	if loaded.DurationSeconds != 4500 {
		t.Fatalf("updated duration = %d, want 4500", loaded.DurationSeconds)
	}
	if loaded.Billable {
		t.Fatal("updated timesheet should be non-billable")
	}
	if loaded.Description != "Updated" {
		t.Fatalf("updated description = %q", loaded.Description)
	}
	if len(loaded.Tags) != 2 || loaded.Tags[0].Name != "qa" || loaded.Tags[1].Name != "review" {
		t.Fatalf("updated tags = %#v", loaded.Tags)
	}
}
