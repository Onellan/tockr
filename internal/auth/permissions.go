package auth

import "tockr/internal/domain"

const (
	PermAdmin          = "admin"
	PermManageOrg      = "manage_organization"
	PermManageUsers    = "manage_users"
	PermManageMaster   = "manage_master_data"
	PermManageRates    = "manage_rates"
	PermTrackTime      = "track_time"
	PermViewReports    = "view_reports"
	PermManageInvoices = "manage_invoices"
	PermUseAPI         = "api_access"
	PermManageWebhooks = "manage_webhooks"
	PermManageGroups   = "manage_groups"
	PermManageProjects = "manage_projects"
)

func HasPermission(access domain.AccessContext, permission string) bool {
	switch permission {
	case PermAdmin, PermManageOrg:
		return access.IsOrgAdmin()
	case PermManageUsers, PermManageMaster, PermManageRates, PermManageInvoices, PermManageWebhooks, PermManageGroups, PermManageProjects:
		return access.IsWorkspaceAdmin()
	case PermTrackTime, PermUseAPI:
		return access.WorkspaceID > 0
	case PermViewReports:
		return access.CanViewWorkspaceReports() || len(access.ManagedProjectIDs) > 0 || access.WorkspaceID > 0
	default:
		return false
	}
}
