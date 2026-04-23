# Workflow Validation Notes

## Automated validation

- `go test ./...`
- `go vet ./...`

## Added regression coverage

- Dashboard engineering workflow messaging and recent-work rendering
- Timesheet prefill rendering and reuse affordances
- Calendar readable labels and add-more-time affordance
- Reports billable filter and work-type labeling
- Existing admin/sidebar/workspace/member/template flows remain covered

## Manual runtime validation checklist

- Dashboard shows engineering-focused summary cards and recent work
- Timesheets supports fast reuse via prefilled links
- Calendar supports weekly review and backfill
- Reports supports billable-only review
- Project dashboard shows burn, unbilled work, tasks, and contributors
