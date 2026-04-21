package legacy

import (
	"math"
	"regexp"
	"strings"
)

var phpRolePattern = regexp.MustCompile(`ROLE_[A-Z_]+`)

func ParseRoles(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"user"}
	}
	matches := phpRolePattern.FindAllString(raw, -1)
	if len(matches) == 0 {
		if strings.Contains(raw, "superadmin") {
			return []string{"superadmin"}
		}
		return []string{"user"}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, match := range matches {
		role := mapRole(match)
		if !seen[role] {
			seen[role] = true
			out = append(out, role)
		}
	}
	if len(out) == 0 {
		return []string{"user"}
	}
	return out
}

func Cents(value float64) int64 {
	return int64(math.Round(value * 100))
}

func mapRole(role string) string {
	switch role {
	case "ROLE_SUPER_ADMIN":
		return "superadmin"
	case "ROLE_ADMIN":
		return "admin"
	case "ROLE_TEAMLEAD":
		return "teamlead"
	default:
		return "user"
	}
}
