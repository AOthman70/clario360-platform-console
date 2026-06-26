package overview

import (
	"context"
	"net/http"

	"github.com/clario360/platform-console/internal/httpx"
)

// ServiceHealth is one service's status in the fleet grid (Slide 7: "status,
// breaker, p95, version per service").
type ServiceHealth struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"` // healthy | degraded | down
	Breaker     string  `json:"breaker"`
	P95Millis   float64 `json:"p95_ms"`
	Version     string  `json:"version"`
	ErrorRate   float64 `json:"error_rate"`
	LastChecked string  `json:"last_checked"`
}

// Snapshot is the Overview payload: KPI tiles plus the per-service grid.
type Snapshot struct {
	TenantCount   int             `json:"tenant_count"`
	ServicesUp    int             `json:"services_up"`
	ServicesTotal int             `json:"services_total"`
	SeatsInUse    int             `json:"seats_in_use"`
	CriticalCount int             `json:"critical_count"`
	Services      []ServiceHealth `json:"services"`
}

// CountStatuses returns how many services are "up" (status == "healthy") and how
// many are "critical" (status == "down"). Degraded services count as neither —
// they are reported in the grid but do not flip a KPI. Pure and DB-free so the
// Overview tallies can be tested without Postgres.
func CountStatuses(services []ServiceHealth) (up, critical int) {
	for _, s := range services {
		switch s.Status {
		case "healthy":
			up++
		case "down":
			critical++
		}
	}
	return
}

// Store reads the fleet snapshot. OQ3 leaves the metrics source open ("Query
// Prometheus if deployed"); this interface lets either a Prometheus-backed or
// a direct /health-scraping implementation satisfy the handler.
type Store interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

// Handler serves the Overview screen.
type Handler struct {
	store Store
}

// New builds an Overview handler over the given Store.
func New(store Store) *Handler { return &Handler{store: store} }

// Get returns the fleet snapshot. Slide 7: "Resilient by design — renders even
// when services are degraded"; the Store reports degraded services as data, not
// as errors.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	snap, err := h.store.Snapshot(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "unable to read fleet snapshot")
		return
	}
	httpx.JSON(w, http.StatusOK, snap)
}
