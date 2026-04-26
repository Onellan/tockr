package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"tockr/internal/domain"
)

type DemoSeedSummary struct {
	WorkspaceID       int64
	UsersCreated      int
	CustomersCreated  int
	ProjectsCreated   int
	ActivitiesCreated int
	TasksCreated      int
	FavoritesCreated  int
	TimesheetsCreated int
}

type demoUserSeed struct {
	Email         string
	Username      string
	DisplayName   string
	Password      string
	WorkspaceRole domain.WorkspaceRole
	CostCents     int64
}

type demoProjectSeed struct {
	CustomerName string
	Company      string
	ProjectName  string
	Number       string
	RateCents    int64
	BudgetCents  int64
	TaskNames    []string
}

type demoProjectBundle struct {
	customer   domain.Customer
	project    domain.Project
	activities map[string]domain.Activity
	tasks      []domain.Task
	rateCents  int64
}

const demoSeedTagName = "demo-seed"

var demoSeedUserEmails = []string{
	"anika.demo@tockr.local",
	"sipho.demo@tockr.local",
	"maya.demo@tockr.local",
	"daniel.demo@tockr.local",
}

var demoSeedCustomerNames = []string{
	"Northwind Mining",
	"GreenLine Foods",
	"MetroGrid Estates",
}

func (s *Store) DefaultWorkspaceForOrganization(ctx context.Context, organizationID int64) (int64, error) {
	var workspaceID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE organization_id=? ORDER BY id LIMIT 1`, organizationID).Scan(&workspaceID)
	if err != nil {
		return 0, err
	}
	return workspaceID, nil
}

func (s *Store) SeedDemoData(ctx context.Context, adminEmail, timezone, currency string) (DemoSeedSummary, error) {
	workspace, err := s.demoSeedWorkspace(ctx)
	if err != nil {
		return DemoSeedSummary{}, err
	}
	return s.seedDemoDataInWorkspace(ctx, workspace, adminEmail, timezone, currency)
}

func (s *Store) SeedDemoDataForWorkspace(ctx context.Context, workspaceID int64, adminEmail, timezone, currency string) (DemoSeedSummary, error) {
	workspace, err := s.demoSeedWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return DemoSeedSummary{}, err
	}
	return s.seedDemoDataInWorkspace(ctx, workspace, adminEmail, timezone, currency)
}

func (s *Store) ClearDemoData(ctx context.Context, organizationID int64) (int64, error) {
	workspaceID, err := s.DefaultWorkspaceForOrganization(ctx, organizationID)
	if err != nil {
		return 0, err
	}
	if err := s.ClearDemoDataForWorkspace(ctx, workspaceID); err != nil {
		return 0, err
	}
	return workspaceID, nil
}

func (s *Store) ClearDemoDataForWorkspace(ctx context.Context, workspaceID int64) error {
	workspace, err := s.demoSeedWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM groups WHERE workspace_id=? AND lower(name)=lower(?)`, workspace.ID, "Demo Delivery Team"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM customers WHERE workspace_id=? AND lower(name) IN (lower(?), lower(?), lower(?))`,
		workspace.ID, demoSeedCustomerNames[0], demoSeedCustomerNames[1], demoSeedCustomerNames[2]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE workspace_id=? AND lower(name)=lower(?)`, workspace.ID, demoSeedTagName); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE organization_id=? AND lower(email) IN (lower(?), lower(?), lower(?), lower(?))`,
		workspace.OrganizationID, demoSeedUserEmails[0], demoSeedUserEmails[1], demoSeedUserEmails[2], demoSeedUserEmails[3]); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) seedDemoDataInWorkspace(ctx context.Context, workspace domain.Workspace, adminEmail, timezone, currency string) (DemoSeedSummary, error) {
	var summary DemoSeedSummary
	summary.WorkspaceID = workspace.ID
	if strings.TrimSpace(timezone) == "" {
		timezone = workspace.Timezone
	}
	if strings.TrimSpace(currency) == "" {
		currency = workspace.DefaultCurrency
	}
	if strings.TrimSpace(currency) == "" {
		currency = "ZAR"
	}
	loc, err := time.LoadLocation(defaultString(strings.TrimSpace(timezone), "Africa/Johannesburg"))
	if err != nil {
		loc = time.FixedZone("SAST", 2*60*60)
	}

	admin, err := s.demoSeedAdmin(ctx, adminEmail)
	if err != nil {
		return summary, err
	}
	if err := s.SetWorkspaceMember(ctx, workspace.ID, admin.ID, domain.WorkspaceRoleAdmin); err != nil {
		return summary, err
	}

	people := []demoUserSeed{
		{
			Email:         "anika.demo@tockr.local",
			Username:      "anika.demo",
			DisplayName:   "Anika Maseko",
			Password:      "demo12345",
			WorkspaceRole: domain.WorkspaceRoleAnalyst,
			CostCents:     92000,
		},
		{
			Email:         "sipho.demo@tockr.local",
			Username:      "sipho.demo",
			DisplayName:   "Sipho Dlamini",
			Password:      "demo12345",
			WorkspaceRole: domain.WorkspaceRoleMember,
			CostCents:     84000,
		},
		{
			Email:         "maya.demo@tockr.local",
			Username:      "maya.demo",
			DisplayName:   "Maya Naidoo",
			Password:      "demo12345",
			WorkspaceRole: domain.WorkspaceRoleMember,
			CostCents:     88000,
		},
		{
			Email:         "daniel.demo@tockr.local",
			Username:      "daniel.demo",
			DisplayName:   "Daniel Jacobs",
			Password:      "demo12345",
			WorkspaceRole: domain.WorkspaceRoleMember,
			CostCents:     79000,
		},
	}

	users := []*domain.User{admin}
	for _, person := range people {
		user, created, err := s.ensureDemoUser(ctx, workspace.OrganizationID, workspace.ID, person)
		if err != nil {
			return summary, err
		}
		if created {
			summary.UsersCreated++
		}
		if err := s.ensureDemoUserCost(ctx, workspace.ID, user.ID, person.CostCents); err != nil {
			return summary, err
		}
		users = append(users, user)
	}

	if err := s.ensureDemoUserCost(ctx, workspace.ID, admin.ID, 98000); err != nil {
		return summary, err
	}

	groupID, err := s.ensureDemoGroup(ctx, workspace.ID, "Demo Delivery Team", "Seeded delivery team for evaluation flows.")
	if err != nil {
		return summary, err
	}
	for _, user := range users {
		if err := s.AddUserToGroup(ctx, groupID, user.ID); err != nil {
			return summary, err
		}
	}

	activityNames := []string{"Design", "Coordination", "Site Visit", "QA", "Documentation"}
	projectSeeds := []demoProjectSeed{
		{
			CustomerName: "Northwind Mining",
			Company:      "Northwind Mining Ltd",
			ProjectName:  "Crusher Upgrade Phase 2",
			Number:       "NW-2407",
			RateCents:    185000,
			BudgetCents:  48500000,
			TaskNames:    []string{"Conveyor Layout", "Motor Control Review", "Commissioning Plan"},
		},
		{
			CustomerName: "GreenLine Foods",
			Company:      "GreenLine Foods",
			ProjectName:  "Cold Storage Expansion",
			Number:       "GL-2412",
			RateCents:    168000,
			BudgetCents:  39200000,
			TaskNames:    []string{"Panel Schedule", "Insulation Audit", "Procurement Review"},
		},
		{
			CustomerName: "MetroGrid Estates",
			Company:      "MetroGrid Estates",
			ProjectName:  "Substation Retrofit",
			Number:       "MG-2503",
			RateCents:    194000,
			BudgetCents:  52800000,
			TaskNames:    []string{"Protection Settings", "Cable Routing", "Shutdown Checklist"},
		},
	}

	bundles := make([]demoProjectBundle, 0, len(projectSeeds))
	for _, seed := range projectSeeds {
		customer, created, err := s.ensureDemoCustomer(ctx, domain.Customer{
			WorkspaceID: workspace.ID,
			Name:        seed.CustomerName,
			Company:     seed.Company,
			Currency:    currency,
			Timezone:    loc.String(),
			Visible:     true,
			Billable:    true,
			Comment:     "Demo customer",
		})
		if err != nil {
			return summary, err
		}
		if created {
			summary.CustomersCreated++
		}

		project, created, err := s.ensureDemoProject(ctx, domain.Project{
			WorkspaceID:        workspace.ID,
			CustomerID:         customer.ID,
			Name:               seed.ProjectName,
			Number:             seed.Number,
			Visible:            true,
			Billable:           true,
			EstimateSeconds:    240 * 3600,
			BudgetCents:        seed.BudgetCents,
			BudgetAlertPercent: 75,
			Comment:            "Demo project",
		})
		if err != nil {
			return summary, err
		}
		if created {
			summary.ProjectsCreated++
		}
		if err := s.AddProjectMember(ctx, project.ID, admin.ID, domain.ProjectRoleManager); err != nil {
			return summary, err
		}
		for _, user := range users[1:] {
			role := domain.ProjectRoleMember
			if user.Email == "anika.demo@tockr.local" && project.Name == "Crusher Upgrade Phase 2" {
				role = domain.ProjectRoleManager
			}
			if user.Email == "maya.demo@tockr.local" && project.Name == "Cold Storage Expansion" {
				role = domain.ProjectRoleManager
			}
			if err := s.AddProjectMember(ctx, project.ID, user.ID, role); err != nil {
				return summary, err
			}
		}
		if err := s.AddGroupToProject(ctx, project.ID, groupID); err != nil {
			return summary, err
		}
		if err := s.ensureDemoProjectRate(ctx, workspace.ID, project.ID, seed.RateCents); err != nil {
			return summary, err
		}

		bundle := demoProjectBundle{
			customer:   customer,
			project:    project,
			activities: make(map[string]domain.Activity, len(activityNames)),
			tasks:      make([]domain.Task, 0, len(seed.TaskNames)),
			rateCents:  seed.RateCents,
		}
		for _, name := range activityNames {
			projectID := project.ID
			activity, created, err := s.ensureDemoActivity(ctx, domain.Activity{
				WorkspaceID: workspace.ID,
				ProjectID:   &projectID,
				Name:        name,
				Visible:     true,
				Billable:    true,
				Comment:     "Demo activity",
			})
			if err != nil {
				return summary, err
			}
			if created {
				summary.ActivitiesCreated++
			}
			bundle.activities[name] = activity
		}
		for _, name := range seed.TaskNames {
			task, created, err := s.ensureDemoTask(ctx, domain.Task{
				WorkspaceID:     workspace.ID,
				ProjectID:       project.ID,
				Name:            name,
				Visible:         true,
				Billable:        true,
				EstimateSeconds: 72 * 3600,
			})
			if err != nil {
				return summary, err
			}
			if created {
				summary.TasksCreated++
			}
			bundle.tasks = append(bundle.tasks, task)
		}
		bundles = append(bundles, bundle)
	}

	adminFavorites := []struct {
		name        string
		projectIdx  int
		activity    string
		taskIdx     int
		description string
		tags        string
	}{
		{"Crusher coordination", 0, "Coordination", 0, "Weekly client and contractor sync", "demo-seed,coordination"},
		{"Cold store QA", 1, "QA", 1, "Quality review and snag checks", "demo-seed,qa"},
		{"Substation design review", 2, "Design", 0, "Package review and redlines", "demo-seed,design"},
	}
	for _, favorite := range adminFavorites {
		bundle := bundles[favorite.projectIdx]
		taskID := bundle.tasks[favorite.taskIdx].ID
		created, err := s.ensureDemoFavorite(ctx, domain.Favorite{
			WorkspaceID: workspace.ID,
			UserID:      admin.ID,
			Name:        favorite.name,
			CustomerID:  bundle.customer.ID,
			ProjectID:   bundle.project.ID,
			ActivityID:  bundle.activities[favorite.activity].ID,
			TaskID:      &taskID,
			Description: favorite.description,
			Tags:        favorite.tags,
		})
		if err != nil {
			return summary, err
		}
		if created {
			summary.FavoritesCreated++
		}
	}

	hasTimesheets, err := s.demoSeedHasTimesheets(ctx, workspace.ID)
	if err != nil {
		return summary, err
	}
	if hasTimesheets {
		return summary, nil
	}

	latest := time.Now().In(loc)
	for latest.Weekday() == time.Saturday || latest.Weekday() == time.Sunday {
		latest = latest.AddDate(0, 0, -1)
	}
	start := latest.AddDate(0, 0, -31)
	dayIndex := 0
	for day := start; !day.After(latest); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for userIndex, user := range users {
			primary := bundles[(dayIndex+userIndex)%len(bundles)]
			secondary := bundles[(dayIndex+userIndex+1)%len(bundles)]

			entrySpecs := []struct {
				bundle      demoProjectBundle
				activity    string
				taskIndex   int
				hour        int
				minute      int
				durationMin int
				billable    bool
				exported    bool
			}{
				{
					bundle:      primary,
					activity:    []string{"Design", "Site Visit", "QA"}[(dayIndex+userIndex)%3],
					taskIndex:   (dayIndex + userIndex) % len(primary.tasks),
					hour:        8 + (userIndex % 2),
					minute:      0,
					durationMin: 150 + ((dayIndex + userIndex) % 3 * 15),
					billable:    true,
					exported:    day.Before(latest.AddDate(0, 0, -14)),
				},
				{
					bundle:      secondary,
					activity:    []string{"Coordination", "Design", "QA"}[(dayIndex+userIndex+1)%3],
					taskIndex:   (dayIndex + 2 + userIndex) % len(secondary.tasks),
					hour:        11,
					minute:      30,
					durationMin: 150 + ((dayIndex + userIndex + 1) % 3 * 15),
					billable:    true,
					exported:    day.Before(latest.AddDate(0, 0, -10)) && userIndex%2 == 0,
				},
			}
			if day.Weekday() != time.Friday || userIndex%2 == 0 {
				entrySpecs = append(entrySpecs, struct {
					bundle      demoProjectBundle
					activity    string
					taskIndex   int
					hour        int
					minute      int
					durationMin int
					billable    bool
					exported    bool
				}{
					bundle:      primary,
					activity:    "Documentation",
					taskIndex:   (dayIndex + userIndex + 1) % len(primary.tasks),
					hour:        15,
					minute:      0,
					durationMin: 90 + ((dayIndex + userIndex) % 3 * 15),
					billable:    day.Weekday() != time.Friday,
					exported:    false,
				})
			}

			for slotIndex, spec := range entrySpecs {
				startAt := time.Date(day.Year(), day.Month(), day.Day(), spec.hour, spec.minute, 0, 0, loc)
				endAt := startAt.Add(time.Duration(spec.durationMin) * time.Minute)
				activity := spec.bundle.activities[spec.activity]
				task := spec.bundle.tasks[spec.taskIndex]
				entry := &domain.Timesheet{
					WorkspaceID: workspace.ID,
					UserID:      user.ID,
					CustomerID:  spec.bundle.customer.ID,
					ProjectID:   spec.bundle.project.ID,
					ActivityID:  activity.ID,
					TaskID:      &task.ID,
					StartedAt:   startAt.UTC(),
					EndedAt:     ptrTime(endAt.UTC()),
					Timezone:    loc.String(),
					Billable:    spec.billable,
					Exported:    spec.exported,
					Description: demoSeedDescription(spec.activity, task.Name, slotIndex),
				}
				if err := s.CreateTimesheet(ctx, entry, demoSeedTags(spec.activity, spec.billable)); err != nil {
					return summary, err
				}
				summary.TimesheetsCreated++
			}
		}
		dayIndex++
	}

	return summary, nil
}

func (s *Store) demoSeedWorkspace(ctx context.Context) (domain.Workspace, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, organization_id, name, slug, description, default_currency, timezone, archived, created_at FROM workspaces ORDER BY id LIMIT 1`)
	var workspace domain.Workspace
	var created string
	if err := row.Scan(&workspace.ID, &workspace.OrganizationID, &workspace.Name, &workspace.Slug, &workspace.Description, &workspace.DefaultCurrency, &workspace.Timezone, &workspace.Archived, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace, errors.New("workspace not found; seed admin first")
		}
		return workspace, err
	}
	workspace.CreatedAt = parseTime(created)
	return workspace, nil
}

func (s *Store) demoSeedWorkspaceByID(ctx context.Context, workspaceID int64) (domain.Workspace, error) {
	workspace, err := s.Workspace(ctx, workspaceID)
	if err != nil {
		return domain.Workspace{}, err
	}
	if workspace == nil {
		return domain.Workspace{}, sql.ErrNoRows
	}
	return *workspace, nil
}

func (s *Store) demoSeedAdmin(ctx context.Context, email string) (*domain.User, error) {
	if strings.TrimSpace(email) != "" {
		user, err := s.FindUserByEmail(ctx, email)
		if err != nil {
			return nil, err
		}
		if user != nil {
			return user, nil
		}
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("admin user not found; seed admin first")
		}
		return nil, err
	}
	return s.FindUserByID(ctx, id)
}

func (s *Store) ensureDemoUser(ctx context.Context, organizationID, workspaceID int64, seed demoUserSeed) (*domain.User, bool, error) {
	user, err := s.FindUserByEmail(ctx, seed.Email)
	if err != nil {
		return nil, false, err
	}
	created := false
	if user == nil {
		if err := s.CreateUser(ctx, domain.User{
			OrganizationID: organizationID,
			Email:          seed.Email,
			Username:       seed.Username,
			DisplayName:    seed.DisplayName,
			Timezone:       "Africa/Johannesburg",
			Enabled:        true,
		}, seed.Password, []domain.Role{domain.RoleUser}); err != nil {
			return nil, false, err
		}
		user, err = s.FindUserByEmail(ctx, seed.Email)
		if err != nil {
			return nil, false, err
		}
		if user == nil {
			return nil, false, fmt.Errorf("demo user %s was not created", seed.Email)
		}
		created = true
	}
	if err := s.SetWorkspaceMember(ctx, workspaceID, user.ID, seed.WorkspaceRole); err != nil {
		return nil, false, err
	}
	return user, created, nil
}

func (s *Store) ensureDemoCustomer(ctx context.Context, customer domain.Customer) (domain.Customer, bool, error) {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM customers WHERE workspace_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, customer.WorkspaceID, customer.Name).Scan(&existingID)
	switch {
	case err == nil:
		customer.ID = existingID
		if err := s.UpsertCustomer(ctx, &customer); err != nil {
			return customer, false, err
		}
		return customer, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := s.UpsertCustomer(ctx, &customer); err != nil {
			return customer, false, err
		}
		return customer, true, nil
	default:
		return customer, false, err
	}
}

func (s *Store) ensureDemoProject(ctx context.Context, project domain.Project) (domain.Project, bool, error) {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE workspace_id=? AND customer_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, project.WorkspaceID, project.CustomerID, project.Name).Scan(&existingID)
	switch {
	case err == nil:
		project.ID = existingID
		if err := s.UpsertProject(ctx, &project); err != nil {
			return project, false, err
		}
		return project, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := s.UpsertProject(ctx, &project); err != nil {
			return project, false, err
		}
		return project, true, nil
	default:
		return project, false, err
	}
}

func (s *Store) ensureDemoActivity(ctx context.Context, activity domain.Activity) (domain.Activity, bool, error) {
	var existingID int64
	var projectID int64
	if activity.ProjectID != nil {
		projectID = *activity.ProjectID
	}
	err := s.db.QueryRowContext(ctx, `SELECT id FROM activities WHERE workspace_id=? AND project_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, activity.WorkspaceID, projectID, activity.Name).Scan(&existingID)
	switch {
	case err == nil:
		activity.ID = existingID
		if err := s.UpsertActivity(ctx, &activity); err != nil {
			return activity, false, err
		}
		return activity, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := s.UpsertActivity(ctx, &activity); err != nil {
			return activity, false, err
		}
		return activity, true, nil
	default:
		return activity, false, err
	}
}

func (s *Store) ensureDemoTask(ctx context.Context, task domain.Task) (domain.Task, bool, error) {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM tasks WHERE workspace_id=? AND project_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, task.WorkspaceID, task.ProjectID, task.Name).Scan(&existingID)
	switch {
	case err == nil:
		task.ID = existingID
		if err := s.UpsertTask(ctx, &task); err != nil {
			return task, false, err
		}
		return task, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := s.UpsertTask(ctx, &task); err != nil {
			return task, false, err
		}
		return task, true, nil
	default:
		return task, false, err
	}
}

func (s *Store) ensureDemoFavorite(ctx context.Context, favorite domain.Favorite) (bool, error) {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM favorites WHERE workspace_id=? AND user_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, favorite.WorkspaceID, favorite.UserID, favorite.Name).Scan(&existingID)
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, sql.ErrNoRows):
		return true, s.CreateFavorite(ctx, &favorite)
	default:
		return false, err
	}
}

func (s *Store) ensureDemoGroup(ctx context.Context, workspaceID int64, name, description string) (int64, error) {
	var groupID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM groups WHERE workspace_id=? AND lower(name)=lower(?) ORDER BY id LIMIT 1`, workspaceID, name).Scan(&groupID)
	switch {
	case err == nil:
		return groupID, nil
	case errors.Is(err, sql.ErrNoRows):
		return s.CreateGroup(ctx, workspaceID, name, description)
	default:
		return 0, err
	}
}

func (s *Store) ensureDemoProjectRate(ctx context.Context, workspaceID, projectID int64, amountCents int64) error {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM rates WHERE workspace_id=? AND project_id=? AND customer_id IS NULL AND activity_id IS NULL AND task_id IS NULL AND user_id IS NULL ORDER BY id LIMIT 1`, workspaceID, projectID).Scan(&existingID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return s.UpsertRate(ctx, &domain.Rate{
			WorkspaceID:   workspaceID,
			ProjectID:     &projectID,
			Kind:          "hourly",
			AmountCents:   amountCents,
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	default:
		return err
	}
}

func (s *Store) ensureDemoUserCost(ctx context.Context, workspaceID, userID, amountCents int64) error {
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM user_cost_rates WHERE workspace_id=? AND user_id=? ORDER BY id LIMIT 1`, workspaceID, userID).Scan(&existingID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return s.UpsertUserCostRate(ctx, &domain.UserCostRate{
			WorkspaceID:   workspaceID,
			UserID:        userID,
			AmountCents:   amountCents,
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	default:
		return err
	}
}

func (s *Store) demoSeedHasTimesheets(ctx context.Context, workspaceID int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timesheet_tags tt JOIN tags t ON t.id=tt.tag_id JOIN timesheets ts ON ts.id=tt.timesheet_id WHERE t.workspace_id=? AND t.name=? AND ts.workspace_id=?`, workspaceID, demoSeedTagName, workspaceID).Scan(&count)
	return count > 0, err
}

func demoSeedDescription(activity, task string, slotIndex int) string {
	switch activity {
	case "Design":
		return fmt.Sprintf("%s package updates", task)
	case "Coordination":
		return fmt.Sprintf("%s coordination follow-up", task)
	case "Site Visit":
		return fmt.Sprintf("%s site walkdown", task)
	case "QA":
		return fmt.Sprintf("%s quality review", task)
	case "Documentation":
		if slotIndex == 2 {
			return fmt.Sprintf("%s handover notes", task)
		}
	}
	return fmt.Sprintf("%s working session", task)
}

func demoSeedTags(activity string, billable bool) []string {
	tags := []string{demoSeedTagName}
	switch activity {
	case "Design":
		tags = append(tags, "design")
	case "Coordination":
		tags = append(tags, "coordination")
	case "Site Visit":
		tags = append(tags, "site")
	case "QA":
		tags = append(tags, "qa")
	case "Documentation":
		tags = append(tags, "docs")
	}
	if !billable {
		tags = append(tags, "internal")
	}
	return tags
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
