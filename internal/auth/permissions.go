package auth

import "strings"

// Permission matching implements the wildcard semantics described in the
// Solution Design, Slide 4 ("The Critical Nuance").
//
// The trap: a held wildcard must match the REQUIRED string, not the other way
// around. `platform:read` is matched by `*` but NOT by `admin:*`. A super-admin
// holding only `admin:*` would therefore be locked out of a console gated on
// `platform:read`.
//
// The fix: gate the console on `admin:console`, which is matched by `admin:*`
// and by `*`, but never by `tenant_admin`. This works for the super-admin
// immediately, while leaving room for granular `platform:*` permissions to be
// delegated later (OQ2 → admin:* for P0, granular in P1).
//
// Foundational gap G-0: the runtime gate resolves only hard-coded role slugs,
// so new platform:* permissions only take effect once added to the super_admin
// role. P0 deliberately reuses admin:*, which super_admin already holds.

// ConsolePermission is the single permission string the entire /platform area
// is gated on. Every screen checks this.
const ConsolePermission = "admin:console"

// ImpersonatePermission gates the guarded impersonation flow (Slide 9). It is
// intentionally separate so it can be granted independently of console access.
const ImpersonatePermission = "platform:tenants:impersonate"

// Matches reports whether any of the held permission strings satisfies the
// required permission, honouring `*` (global) and `prefix:*` (namespace)
// wildcards.
//
// A held permission satisfies a required one when:
//   - it is exactly equal, OR
//   - it is "*" (matches everything), OR
//   - it is "ns:*" and the required permission is "ns:<anything>".
//
// Wildcards are only ever expanded on the HELD side. A required permission is
// always a concrete string; we never treat a required "*" as a wildcard.
func Matches(held []string, required string) bool {
	for _, h := range held {
		if h == required {
			return true
		}
		if h == "*" {
			return true
		}
		if ns, ok := strings.CutSuffix(h, ":*"); ok {
			// "admin:*" matches "admin:console" but not "admin" alone and not
			// "administration:console".
			if reqNS, found := namespaceOf(required); found && reqNS == ns {
				return true
			}
		}
	}
	return false
}

// namespaceOf returns the portion of a permission string before the FIRST
// colon, e.g. "admin" for "admin:console" and "platform" for
// "platform:tenants:impersonate". The second return value is false when the
// string contains no colon (and therefore has no namespace).
func namespaceOf(perm string) (string, bool) {
	i := strings.IndexByte(perm, ':')
	if i < 0 {
		return "", false
	}
	return perm[:i], true
}
