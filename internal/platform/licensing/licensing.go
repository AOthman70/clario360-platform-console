package licensing

import (
	"context"
	"net/http"

	"github.com/clario360/platform-console/internal/httpx"
)

// LicenceRow is one tenant's licence position in the fleet view. Slide 8 counts
// "Licence fleet queries — 5 gaps"; Slide 8 also notes "licence admin … already
// built and reused" — this screen aggregates that per-tenant data across the
// whole estate.
type LicenceRow struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Tier       string `json:"tier"`
	SeatsUsed  int    `json:"seats_used"`
	SeatsLimit int    `json:"seats_limit"`
	State      string `json:"state"` // active | expiring | expired | over_limit
	RenewsAt   string `json:"renews_at"`
}

// FleetSummary rolls the per-tenant rows into the KPI tiles.
type FleetSummary struct {
	TotalSeats     int          `json:"total_seats"`
	SeatsInUse     int          `json:"seats_in_use"`
	OverLimitCount int          `json:"over_limit_count"`
	ExpiringCount  int          `json:"expiring_count"`
	Rows           []LicenceRow `json:"rows"`
}

// Store reads fleet-wide licence state.
type Store interface {
	Fleet(ctx context.Context) (FleetSummary, error)
}

// Handler serves the Licensing screen.
type Handler struct {
	store Store
}

// New builds a Licensing handler.
func New(store Store) *Handler { return &Handler{store: store} }

// Fleet returns the estate-wide licence summary.
func (h *Handler) Fleet(w http.ResponseWriter, r *http.Request) {
	summary, err := h.store.Fleet(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to read licence fleet")
		return
	}
	httpx.JSON(w, http.StatusOK, summary)
}
