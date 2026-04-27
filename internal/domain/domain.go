package domain

import "time"

type Role string

const (
	RoleUser       Role = "user"
	RoleTeamLead   Role = "teamlead"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "superadmin"
)

type OrganizationRole string

const (
	OrgRoleOwner OrganizationRole = "owner"
	OrgRoleAdmin OrganizationRole = "admin"
)

type WorkspaceRole string

const (
	WorkspaceRoleAdmin   WorkspaceRole = "admin"
	WorkspaceRoleAnalyst WorkspaceRole = "analyst"
	WorkspaceRoleMember  WorkspaceRole = "member"
)

type ProjectRole string

const (
	ProjectRoleManager ProjectRole = "manager"
	ProjectRoleMember  ProjectRole = "member"
)

type Organization struct {
	ID        int64
	Name      string
	Slug      string
	CreatedAt time.Time
}

type Workspace struct {
	ID              int64
	OrganizationID  int64
	Name            string
	Slug            string
	Description     string
	DefaultCurrency string
	Timezone        string
	SMTP            WorkspaceSMTPSettings
	Archived        bool
	CreatedAt       time.Time
}

type WorkspaceSMTPSettings struct {
	Host              string
	Port              int
	Username          string
	Password          string
	PasswordEncrypted string
	PasswordSet       bool
	FromEmail         string
	FromName          string
	TLS               bool
}

type WorkspaceMember struct {
	WorkspaceID         int64
	UserID              int64
	Role                WorkspaceRole
	DisplayName         string
	Email               string
	Enabled             bool
	GroupCount          int64
	ProjectMemberCount  int64
	ManagedProjectCount int64
	CreatedAt           time.Time
}

type WorkspaceSummary struct {
	Workspace
	MemberCount    int64
	ProjectCount   int64
	SMTPConfigured bool
}

type Group struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Description string
	CreatedAt   time.Time
}

type ProjectMember struct {
	ProjectID int64
	UserID    int64
	Role      ProjectRole
	CreatedAt time.Time
}

type AccessContext struct {
	UserID            int64
	OrganizationID    int64
	WorkspaceID       int64
	OrganizationRole  OrganizationRole
	WorkspaceRole     WorkspaceRole
	ManagedProjectIDs map[int64]bool
	MemberProjectIDs  map[int64]bool
}

func (a AccessContext) IsOrgAdmin() bool {
	return a.OrganizationRole == OrgRoleOwner || a.OrganizationRole == OrgRoleAdmin
}

func (a AccessContext) IsWorkspaceAdmin() bool {
	return a.IsOrgAdmin() || a.WorkspaceRole == WorkspaceRoleAdmin
}

func (a AccessContext) CanViewWorkspaceReports() bool {
	return a.IsWorkspaceAdmin() || a.WorkspaceRole == WorkspaceRoleAnalyst
}

func (a AccessContext) ManagesProject(projectID int64) bool {
	return a.IsWorkspaceAdmin() || a.ManagedProjectIDs[projectID]
}

func (a AccessContext) CanAccessProject(projectID int64) bool {
	return a.IsWorkspaceAdmin() || a.ManagedProjectIDs[projectID] || a.MemberProjectIDs[projectID]
}

type User struct {
	ID              int64
	OrganizationID  int64
	Email           string
	Username        string
	DisplayName     string
	PasswordHash    string
	Timezone        string
	Enabled         bool
	TOTPSecret      string
	TOTPEnabled     bool
	EmailOTPEnabled bool
	Roles           []Role
	CreatedAt       time.Time
	LastLoginAt     *time.Time
}

type Customer struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Number      string
	Company     string
	Contact     string
	Email       string
	Currency    string
	Timezone    string
	Visible     bool
	Billable    bool
	Comment     string
	LegacyJSON  string
	CreatedAt   time.Time
}

// Workstream represents a discipline or phase category within a project (e.g. Civil, Mechanical, Electrical).
// Workstreams are workspace-level entities that can be assigned to projects.
type Workstream struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Code        string // e.g. "WS-000001"
	Description string
	Visible     bool
	CreatedAt   time.Time
}

// ProjectWorkstream associates a workstream to a project with optional budget allocation.
type ProjectWorkstream struct {
	ID             int64
	ProjectID      int64
	WorkstreamID   int64
	WorkstreamName string
	BudgetCents    int64
	Active         bool
	CreatedAt      time.Time
}

// WorkSchedule holds configurable working-time expectations for a workspace.
type WorkSchedule struct {
	// WorkingDays is a bitmask: bit 1=Mon, 2=Tue, 3=Wed, 4=Thu, 5=Fri, 6=Sat, 7=Sun
	WorkingDaysOfWeek  []time.Weekday // e.g. [Mon, Tue, Wed, Thu, Fri]
	WorkingHoursPerDay float64        // e.g. 8.0
}

type EmailSettings struct {
	NotifyOldEmailOnChange bool
}

// MonthOverride stores an admin-set working-day count for a specific year/month.
type MonthOverride struct {
	Year  int
	Month time.Month
	Days  int
}

type Project struct {
	ID                 int64
	WorkspaceID        int64
	CustomerID         int64
	Name               string
	Number             string
	OrderNo            string
	Visible            bool
	Private            bool
	Billable           bool
	EstimateSeconds    int64
	BudgetCents        int64
	BudgetAlertPercent int64
	Comment            string
	LegacyJSON         string
	CreatedAt          time.Time
}

type ProjectCreateDraft struct {
	Project     Project
	Workstreams []ProjectCreateWorkstreamDraft
	Activities  []ProjectCreateActivityDraft
	Members     []ProjectCreateMemberDraft
}

type ProjectCreateWorkstreamDraft struct {
	ExistingWorkstreamID int64
	Name                 string
	Code                 string
	Description          string
	BudgetCents          int64
}

type ProjectCreateActivityDraft struct {
	ExistingActivityID int64
	Name               string
	Number             string
	Comment            string
	Visible            bool
	Billable           bool
}

type ProjectCreateMemberDraft struct {
	UserID int64
	Role   ProjectRole
}

type Activity struct {
	ID          int64
	WorkspaceID int64
	ProjectID   *int64
	Name        string
	Number      string
	Visible     bool
	Billable    bool
	Comment     string
	LegacyJSON  string
	CreatedAt   time.Time
}

type Tag struct {
	ID          int64
	WorkspaceID int64
	Name        string
	Visible     bool
}

type Task struct {
	ID              int64
	WorkspaceID     int64
	ProjectID       int64
	Name            string
	Number          string
	Visible         bool
	Billable        bool
	Archived        bool
	EstimateSeconds int64
	CreatedAt       time.Time
}

type Favorite struct {
	ID          int64
	WorkspaceID int64
	UserID      int64
	Name        string
	CustomerID  int64
	ProjectID   int64
	ActivityID  int64
	TaskID      *int64
	Description string
	Tags        string
	CreatedAt   time.Time
}

type Rate struct {
	ID                  int64
	WorkspaceID         int64
	CustomerID          *int64
	ProjectID           *int64
	ActivityID          *int64
	TaskID              *int64
	UserID              *int64
	Kind                string
	AmountCents         int64
	InternalAmountCents *int64
	Fixed               bool
	EffectiveFrom       time.Time
	EffectiveTo         *time.Time
}

type UserCostRate struct {
	ID            int64
	WorkspaceID   int64
	UserID        int64
	AmountCents   int64
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	CreatedAt     time.Time
}

type Timesheet struct {
	ID                int64
	WorkspaceID       int64
	UserID            int64
	CustomerID        int64
	ProjectID         int64
	WorkstreamID      *int64
	ActivityID        int64
	TaskID            *int64
	StartedAt         time.Time
	EndedAt           *time.Time
	Timezone          string
	DurationSeconds   int64
	BreakSeconds      int64
	RateCents         int64
	InternalRateCents *int64
	Billable          bool
	Exported          bool
	Description       string
	Tags              []Tag
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Invoice struct {
	ID            int64
	WorkspaceID   int64
	Number        string
	CustomerID    int64
	UserID        int64
	Status        string
	Currency      string
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	Filename      string
	PaymentDate   *time.Time
	CreatedAt     time.Time
}

type InvoiceItem struct {
	ID          int64
	InvoiceID   int64
	TimesheetID *int64
	Description string
	Quantity    int64
	UnitCents   int64
	TotalCents  int64
}

type WebhookEndpoint struct {
	ID          int64
	WorkspaceID int64
	Name        string
	URL         string
	Secret      string
	Events      []string
	Enabled     bool
	CreatedAt   time.Time
}

type SavedReport struct {
	ID             int64
	WorkspaceID    int64
	UserID         int64
	Name           string
	GroupBy        string
	FiltersJSON    string
	Shared         bool
	ShareToken     string
	ShareExpiresAt *time.Time
	CreatedAt      time.Time
}

type ExchangeRate struct {
	ID              int64
	WorkspaceID     int64
	FromCurrency    string
	ToCurrency      string
	RateThousandths int64
	EffectiveFrom   time.Time
	CreatedAt       time.Time
}

type InvoiceDetail struct {
	Invoice
	Customer      Customer
	Items         []InvoiceItem
	WorkspaceName string
}

type RecalcPreviewRow struct {
	TimesheetID       int64
	StartedAt         time.Time
	UserID            int64
	ProjectID         int64
	CurrentRateCents  int64
	ResolvedRateCents int64
	DeltaCents        int64
	Description       string
	Exported          bool
}

type UtilizationRow struct {
	UserID             int64
	DisplayName        string
	TotalSeconds       int64
	BillableSeconds    int64
	NonBillableSeconds int64
	ExpectedSeconds    int64
	MissingSeconds     int64
	EntryCents         int64
	EntryCount         int64
}

type ReportFilter struct {
	Group      string
	Begin      *time.Time
	End        *time.Time
	CustomerID int64
	ProjectID  int64
	ActivityID int64
	TaskID     int64
	UserID     int64
	GroupID    int64
	Billable   *bool
}

type ProjectDashboardFilter struct {
	Begin        *time.Time
	End          *time.Time
	WorkstreamID int64
	ActivityID   int64
	TaskID       int64
	UserID       int64
	GroupID      int64
}

type ProjectBreakdownSlice struct {
	ItemID          int64
	Name            string
	TrackedSeconds  int64
	UnbilledSeconds int64
}

type ProjectContributionSummary struct {
	UserID         int64
	DisplayName    string
	ItemID         int64
	ItemName       string
	TrackedSeconds int64
}

type DashboardRecentWork struct {
	TimesheetID     int64
	CustomerID      int64
	ProjectID       int64
	WorkstreamID    *int64
	ActivityID      int64
	TaskID          *int64
	Description     string
	DurationSeconds int64
	StartedAt       time.Time
	Billable        bool
	Exported        bool
}

type DashboardProjectWatch struct {
	ProjectID          int64
	Name               string
	CustomerID         int64
	TrackedSeconds     int64
	BillableCents      int64
	EstimatePercent    int64
	BudgetPercent      int64
	UnbilledSeconds    int64
	UnbilledCents      int64
	NeedsEstimateAlert bool
	NeedsBudgetAlert   bool
}

type DashboardSummary struct {
	Stats               map[string]int64
	WeekTracked         int64
	MissingSeconds      int64
	ExpectedWeekSeconds int64
	RecentWork          []DashboardRecentWork
	ProjectWatchlist    []DashboardProjectWatch
}

type ProjectTaskSummary struct {
	TaskID          int64
	Name            string
	TrackedSeconds  int64
	UnbilledSeconds int64
	Billable        bool
}

type ProjectContributorSummary struct {
	UserID          int64
	DisplayName     string
	TrackedSeconds  int64
	BillableSeconds int64
}

type ProjectWorkstreamSummary struct {
	WorkstreamID   int64
	Name           string
	TrackedSeconds int64
}

type ProjectActivitySummary struct {
	ActivityID     int64
	Name           string
	TrackedSeconds int64
}

type ProjectDashboard struct {
	Project                Project
	Filter                 ProjectDashboardFilter
	TrackedSeconds         int64
	BillableCents          int64
	UnbilledSeconds        int64
	UnbilledCents          int64
	BillableSeconds        int64
	NonBillableSeconds     int64
	EstimatePercent        int64
	BudgetPercent          int64
	OverEstimate           bool
	OverBudget             bool
	Alert                  bool
	TaskSummaries          []ProjectTaskSummary
	Contributors           []ProjectContributorSummary
	WorkstreamSummaries    []ProjectWorkstreamSummary
	ActivitySummaries      []ProjectActivitySummary
	WorkstreamBreakdown    []ProjectBreakdownSlice
	WorkTypeBreakdown      []ProjectBreakdownSlice
	TaskBreakdown          []ProjectBreakdownSlice
	WorkstreamContributors []ProjectContributionSummary
	WorkTypeContributors   []ProjectContributionSummary
	TaskContributors       []ProjectContributionSummary
}

type ProjectTemplate struct {
	ID                 int64
	WorkspaceID        int64
	Name               string
	Description        string
	ProjectName        string
	ProjectNumber      string
	OrderNo            string
	Visible            bool
	Private            bool
	Billable           bool
	EstimateSeconds    int64
	BudgetCents        int64
	BudgetAlertPercent int64
	Archived           bool
	Tasks              []ProjectTemplateTask
	Activities         []ProjectTemplateActivity
	CreatedAt          time.Time
}

type ProjectTemplateTask struct {
	ID              int64
	TemplateID      int64
	Name            string
	Number          string
	Visible         bool
	Billable        bool
	EstimateSeconds int64
}

type ProjectTemplateActivity struct {
	ID         int64
	TemplateID int64
	Name       string
	Number     string
	Visible    bool
	Billable   bool
}

type Page struct {
	Page    int
	Size    int
	Total   int
	HasPrev bool
	HasNext bool
}

func NormalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
