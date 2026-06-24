package auth

import "testing"

// TestMatches locks in the wildcard semantics from Slide 4 ("The Critical
// Nuance"). The central cases are the trap and the fix: admin:* must satisfy
// admin:console, but must NOT satisfy platform:read; tenant_admin must satisfy
// neither.
func TestMatches(t *testing.T) {
	cases := []struct {
		name     string
		held     []string
		required string
		want     bool
	}{
		// The fix: super-admin holding admin:* reaches the console.
		{"admin wildcard matches admin:console", []string{"admin:*"}, "admin:console", true},
		// Global wildcard matches everything.
		{"global wildcard matches admin:console", []string{"*"}, "admin:console", true},
		{"global wildcard matches platform:read", []string{"*"}, "platform:read", true},
		// The trap: admin:* must NOT match a platform:* requirement.
		{"admin wildcard does not match platform:read", []string{"admin:*"}, "platform:read", false},
		// tenant_admin never reaches the console.
		{"tenant_admin does not match admin:console", []string{"tenant_admin"}, "admin:console", false},
		{"tenant_admin wildcard does not cross namespace", []string{"tenant_admin:*"}, "admin:console", false},
		// Exact match.
		{"exact match", []string{"admin:console"}, "admin:console", true},
		// Namespace wildcard must not match a bare namespace token.
		{"admin wildcard does not match bare admin", []string{"admin:*"}, "admin", false},
		// Prefix must be a full namespace, not a substring.
		{"administration wildcard does not match admin namespace", []string{"administration:*"}, "admin:console", false},
		// Deeper required strings still match on first-segment namespace.
		{"platform wildcard matches nested impersonate perm", []string{"platform:*"}, "platform:tenants:impersonate", true},
		// Empty held set matches nothing.
		{"empty held set", nil, "admin:console", false},
		// Unrelated permission does not match.
		{"unrelated exact permission", []string{"licence:read"}, "admin:console", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Matches(c.held, c.required); got != c.want {
				t.Errorf("Matches(%v, %q) = %v, want %v", c.held, c.required, got, c.want)
			}
		})
	}
}

// TestConsolePermissionConstant guards against accidentally changing the gate
// string, which would silently lock out every super-admin (G-0).
func TestConsolePermissionConstant(t *testing.T) {
	if ConsolePermission != "admin:console" {
		t.Fatalf("ConsolePermission = %q, want %q — changing this re-opens the G-0 trap", ConsolePermission, "admin:console")
	}
}
