package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/clario360/platform-console/internal/auth"
	"github.com/clario360/platform-console/internal/httpx"
	"github.com/clario360/platform-console/internal/platform/licensing"
	"github.com/clario360/platform-console/internal/platform/overview"
	"github.com/clario360/platform-console/internal/platform/suites"
	"github.com/clario360/platform-console/internal/platform/tenants"
)

// Handlers bundles the per-screen handlers the router mounts. Each is
// constructed in main with its concrete Store implementation.
type Handlers struct {
	Overview  *overview.Handler
	Tenants   *tenants.Handler
	Suites    *suites.Handler
	Licensing *licensing.Handler
}

// Router builds the full chi router. The /platform subtree sits behind
// Authenticate + RequirePermission(admin:console) — Slide 5 ("chi v5 … RS256
// JWT … strips X-Tenant-ID") and Slide 6 ("All screens gate on admin:console").
func Router(verifier *auth.Verifier, h Handlers) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Liveness — ungated, no tenant context.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// /platform — cross-tenant control plane. Slide 3: "platform is absent
	// from the suite route set, so the new section renders everywhere
	// automatically." The whole subtree is authenticated and gated.
	r.Route("/platform", func(pr chi.Router) {
		pr.Use(auth.Authenticate(verifier))
		pr.Use(auth.RequirePermission(auth.ConsolePermission))

		// P0 · Overview (read)
		pr.Get("/overview", h.Overview.Get)

		// P0 · Tenants
		pr.Get("/tenants", h.Tenants.List)
		pr.Get("/tenants/{tenantID}", h.Tenants.Get)
		// Destructive: confirm + fail-closed audit, blocked under read-only
		// impersonation.
		pr.With(auth.DenyImpersonatedWrites).Post("/tenants/{tenantID}/suspend", h.Tenants.Suspend)
		pr.With(auth.DenyImpersonatedWrites).Post("/tenants/{tenantID}/reinstate", h.Tenants.Reinstate)

		// P0 · Suite catalog
		pr.Get("/suites", h.Suites.Catalog)
		pr.Get("/tenants/{tenantID}/suites", h.Suites.TenantState)
		pr.With(auth.DenyImpersonatedWrites).Put("/tenants/{tenantID}/suites/{suiteKey}", h.Suites.Toggle)

		// P0 · Licensing
		pr.Get("/licensing/fleet", h.Licensing.Fleet)
	})

	return r
}
