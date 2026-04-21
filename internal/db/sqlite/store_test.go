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
