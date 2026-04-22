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
	DefaultCurrency string
	Timezone        string
	CreatedAt       time.Time
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
	ID             int64
	OrganizationID int64
	Email          string
	Username       string
	DisplayName    string
	PasswordHash   string
	Timezone       string
	Enabled        bool
	Roles          []Role
	CreatedAt      time.Time
	LastLoginAt    *time.Time
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
	UserID              *int64
	Kind                string
	AmountCents         int64
	InternalAmountCents *int64
	Fixed               bool
}

type Timesheet struct {
	ID                int64
	WorkspaceID       int64
	UserID            int64
	CustomerID        int64
	ProjectID         int64
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
	ID          int64
	WorkspaceID int64
	UserID      int64
	Name        string
	GroupBy     string
	FiltersJSON string
	Shared      bool
	CreatedAt   time.Time
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
}

type ProjectDashboard struct {
	Project         Project
	TrackedSeconds  int64
	BillableCents   int64
	EstimatePercent int64
	BudgetPercent   int64
	OverEstimate    bool
	OverBudget      bool
	Alert           bool
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
