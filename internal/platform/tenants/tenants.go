package tenants

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/clario360/platform-console/internal/audit"
	"github.com/clario360/platform-console/internal/auth"
	"github.com/clario360/platform-console/internal/httpx"
)

// ErrNotFound is returned by a Store when a tenant id does not exist.
var ErrNotFound = errors.New("tenant not found")

// Tenant is a row in the all-tenant list (Slide 10: "all-tenant list with
// seats & licence").
type Tenant struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"` // active | suspended
	SeatsUsed   int    `json:"seats_used"`
	SeatsLimit  int    `json:"seats_limit"`
	LicenceTier string `json:"licence_tier"`
	CreatedAt   string `json:"created_at"`
}

// Detail extends Tenant with the per-tenant view backing the detail panel.
type Detail struct {
	Tenant
	ActiveSuites []string `json:"active_suites"`
	Region       string   `json:"region"`
}

// Store reads and mutates tenant state. Slide 8 marks "Tenant lifecycle …
// already built and reused"; this interface re-homes that capability under
// /platform (Slide 3: "the old /admin/tenants re-homes under /platform").
type Store interface {
	List(ctx context.Context) ([]Tenant, error)
	Get(ctx context.Context, id string) (Detail, error)
	SetSuspended(ctx context.Context, id string, suspended bool) error
}

// Handler serves the Tenants screens.
type Handler struct {
	store    Store
	recorder *audit.Recorder
}

// New builds a Tenants handler.
func New(store Store, recorder *audit.Recorder) *Handler {
	return &Handler{store: store, recorder: recorder}
}

// List returns every tenant in the estate.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to list tenants")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tenants": rows})
}

// Get returns a single tenant's detail.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tenantID")
	d, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "tenant not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to read tenant")
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

// Suspend performs a real suspend — Slide 2 calls out today's gap: "Suspending
// a tenant only flips a column — access keeps working." Slide 10 requires "real
// suspend … with confirm + audit". The audit write is fail-closed (OQ4): if it
// fails, the suspend does not happen.
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	h.setSuspended(w, r, true)
}

// Reinstate lifts a suspension, also with fail-closed audit.
func (h *Handler) Reinstate(w http.ResponseWriter, r *http.Request) {
	h.setSuspended(w, r, false)
}

func (h *Handler) setSuspended(w http.ResponseWriter, r *http.Request, suspended bool) {
	id := chi.URLParam(r, "tenantID")
	claims, _ := auth.FromContext(r.Context())

	action := "platform.tenant.reinstate"
	if suspended {
		action = "platform.tenant.suspend"
	}

	// Fail-closed audit BEFORE the mutation. If we cannot prove we recorded the
	// intent, we refuse to act.
	entry := audit.Entry{
		ActorID:    actorID(claims),
		ActorRoles: actorRoles(claims),
		Action:     action,
		TargetType: "tenant",
		TargetID:   id,
		Outcome:    audit.OutcomeSuccess,
	}
	if err := h.recorder.MustRecord(r.Context(), entry); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "audit unavailable; operation refused")
		return
	}

	if err := h.store.SetSuspended(r.Context(), id, suspended); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "tenant not found")
			return
		}
		// Record the failed outcome best-effort; the operation did not complete.
		failed := entry
		failed.Outcome = audit.OutcomeFailure
		_ = h.recorder.Record(r.Context(), failed)
		httpx.Error(w, http.StatusBadGateway, "unable to update tenant status")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "suspended": suspended})
}

func actorID(c *auth.Claims) string {
	if c == nil {
		return ""
	}
	return c.Subject
}

func actorRoles(c *auth.Claims) []string {
	if c == nil {
		return nil
	}
	return c.Roles
}
