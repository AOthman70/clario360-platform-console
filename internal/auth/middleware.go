package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/clario360/platform-console/internal/httpx"
)

type ctxKey int

const claimsKey ctxKey = iota

// FromContext returns the verified claims attached by Authenticate. The second
// return value is false if the request was never authenticated (which should
// not happen on routes mounted behind the middleware).
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

// Authenticate verifies the bearer token and attaches the resulting claims to
// the request context. It also strips any inbound X-Tenant-ID header.
//
// Slide 5 notes the gateway "strips X-Tenant-ID". The console is cross-tenant
// by default (Slide 2: "/admin is tenant-scoped … A platform operator needs
// the opposite: the whole estate"), so a tenant pin arriving from the client
// must never be honoured — tenant scope comes from the verified token alone.
func Authenticate(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Defence in depth: never let a client-supplied tenant pin survive.
			r.Header.Del("X-Tenant-ID")

			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := v.Parse(raw)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission gates a route subtree on a single permission string,
// honouring the wildcard semantics in Matches. Mount the whole /platform area
// behind RequirePermission(ConsolePermission) — Slide 6: "All screens gate on
// admin:console".
func RequirePermission(required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			if !Matches(claims.Permissions, required) {
				// 404, not 403: Slide 3 — "The sidebar drops a whole section
				// when the user lacks its permission … no leakage of
				// cross-tenant controls." We don't confirm the area exists.
				httpx.Error(w, http.StatusNotFound, "not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DenyImpersonatedWrites blocks state-changing requests that arrive inside a
// read-only impersonation session (Slide 9: "Read-only by default — minted
// token carries a readonly claim"). Mount on destructive handlers.
func DenyImpersonatedWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := FromContext(r.Context()); ok {
			if claims.IsImpersonating() && claims.ReadOnly {
				httpx.Error(w, http.StatusForbidden, "read-only impersonation session: writes are not permitted")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
