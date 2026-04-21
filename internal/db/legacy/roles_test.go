package legacy

import "testing"

func TestParsePHPRoles(t *testing.T) {
	roles := ParseRoles(`a:1:{i:0;s:16:"ROLE_SUPER_ADMIN";}`)
	if len(roles) != 1 || roles[0] != "superadmin" {
		t.Fatalf("unexpected roles: %#v", roles)
	}
}

func TestCents(t *testing.T) {
	if got := Cents(12.345); got != 1235 {
		t.Fatalf("expected rounded cents, got %d", got)
	}
}
