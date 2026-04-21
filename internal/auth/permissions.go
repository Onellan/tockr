package auth

import "tockr/internal/domain"

const (
	PermAdmin          = "admin"
	PermManageUsers    = "manage_users"
	PermManageMaster   = "manage_master_data"
	PermManageRates    = "manage_rates"
	PermTrackTime      = "track_time"
	PermViewReports    = "view_reports"
	PermManageInvoices = "manage_invoices"
	PermUseAPI         = "api_access"
	PermManageWebhooks = "manage_webhooks"
)

var rolePermissions = map[domain.Role]map[string]bool{
	domain.RoleUser: {
		PermTrackTime:   true,
		PermViewReports: true,
		PermUseAPI:      true,
	},
	domain.RoleTeamLead: {
		PermTrackTime:   true,
		PermViewReports: true,
		PermUseAPI:      true,
	},
	domain.RoleAdmin: {
		PermAdmin:          true,
		PermManageUsers:    true,
		PermManageMaster:   true,
		PermManageRates:    true,
		PermTrackTime:      true,
		PermViewReports:    true,
		PermManageInvoices: true,
		PermUseAPI:         true,
		PermManageWebhooks: true,
	},
	domain.RoleSuperAdmin: {
		PermAdmin:          true,
		PermManageUsers:    true,
		PermManageMaster:   true,
		PermManageRates:    true,
		PermTrackTime:      true,
		PermViewReports:    true,
		PermManageInvoices: true,
		PermUseAPI:         true,
		PermManageWebhooks: true,
	},
}

func HasRole(user *domain.User, role domain.Role) bool {
	if user == nil {
		return false
	}
	for _, item := range user.Roles {
		if item == role {
			return true
		}
	}
	return false
}

func HasPermission(user *domain.User, permission string) bool {
	if user == nil || !user.Enabled {
		return false
	}
	for _, role := range user.Roles {
		if rolePermissions[role][permission] {
			return true
		}
	}
	return false
}
