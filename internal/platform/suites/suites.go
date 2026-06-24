package suites

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/clario360/platform-console/internal/audit"
	"github.com/clario360/platform-console/internal/auth"
	"github.com/clario360/platform-console/internal/httpx"
)

// ErrNotFound is returned when a tenant or suite reference does not resolve.
var ErrNotFound = errors.New("not found")

// Suite is an entry in the catalog. Slide 5 lists the plan-gated suite services
// (Cyber, SIEM, Data, Acta, Watheeq, Visus, ClarioDR); Slide 14 frames the
// commercial suites (Business+, DataStream).
type Suite struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Model       string `json:"model"` // seat-based | capacity-based
}

// TenantSuiteState reports whether a suite is enabled for a given tenant.
type TenantSuiteState struct {
	SuiteKey string `json:"suite_key"`
	Enabled  bool   `json:"enabled"`
}

// Store reads the catalog and toggles suite entitlement per tenant.
type Store interface {
	Catalog(ctx context.Context) ([]Suite, error)
	TenantState(ctx context.Context, tenantID string) ([]TenantSuiteState, error)
	SetEnabled(ctx context.Context, tenantID, suiteKey string, enabled bool) error
}

// Handler serves the Suite catalog screen.
type Handler struct {
	store    Store
	recorder *audit.Recorder
}

// New builds a Suites handler.
func New(store Store, recorder *audit.Recorder) *Handler {
	return &Handler{store: store, recorder: recorder}
}

// Catalog returns the full suite catalog.
func (h *Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.Catalog(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to read suite catalog")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suites": rows})
}

// TenantState returns the per-suite enablement for one tenant.
func (h *Handler) TenantState(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	rows, err := h.store.TenantState(r.Context(), tenantID)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "tenant not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to read suite state")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"state": rows})
}

type toggleRequest struct {
	Enabled bool `json:"enabled"`
}

// Toggle enables or disables a suite for a tenant (Slide 10: "suite toggle …
// with confirm + audit"). Audit is fail-closed.
func (h *Handler) Toggle(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	suiteKey := chi.URLParam(r, "suiteKey")

	var body toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, _ := auth.FromContext(r.Context())
	entry := audit.Entry{
		ActorID:    subject(claims),
		ActorRoles: roles(claims),
		Action:     "platform.suite.toggle",
		TargetType: "tenant_suite",
		TargetID:   tenantID + "/" + suiteKey,
		Outcome:    audit.OutcomeSuccess,
		Metadata:   map[string]any{"enabled": body.Enabled},
	}
	if err := h.recorder.MustRecord(r.Context(), entry); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "audit unavailable; operation refused")
		return
	}

	if err := h.store.SetEnabled(r.Context(), tenantID, suiteKey, body.Enabled); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "tenant or suite not found")
			return
		}
		failed := entry
		failed.Outcome = audit.OutcomeFailure
		_ = h.recorder.Record(r.Context(), failed)
		httpx.Error(w, http.StatusBadGateway, "unable to toggle suite")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"suite_key": suiteKey,
		"enabled":   body.Enabled,
	})
}

func subject(c *auth.Claims) string {
	if c == nil {
		return ""
	}
	return c.Subject
}

func roles(c *auth.Claims) []string {
	if c == nil {
		return nil
	}
	return c.Roles
}
