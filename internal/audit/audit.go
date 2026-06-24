package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Outcome marks whether the audited action succeeded or failed.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
)

// Entry is a single cross-tenant audit record. Slide 2 flags that "Audit routes
// have no permission gate at all" today and Slide 8 counts the audit chain as
// largely existing; this is the platform-side contract those records satisfy.
type Entry struct {
	ID         uuid.UUID      `json:"id"`
	OccurredAt time.Time      `json:"occurred_at"`
	ActorID    string         `json:"actor_id"`
	ActorRoles []string       `json:"actor_roles"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Outcome    Outcome        `json:"outcome"`
	Metadata   map[string]any `json:"metadata,omitempty"`

	// Impersonation attribution (Slide 9: "Fully attributable — act-as claims
	// + propagation header"). Set when the action ran inside an act-as session.
	ImpersonatedTenant string `json:"impersonated_tenant,omitempty"`
}

// Sink persists audit entries. Implementations decide their own durability
// guarantees; callers choose between Record (best-effort) and MustRecord
// (fail-closed) per the sensitivity of the action.
type Sink interface {
	Write(ctx context.Context, e Entry) error
}

// Recorder wraps a Sink with the two write disciplines the design calls for.
type Recorder struct {
	sink Sink
}

// NewRecorder builds a Recorder over the given Sink.
func NewRecorder(s Sink) *Recorder { return &Recorder{sink: s} }

// Record writes an entry best-effort. A failure is returned but callers may
// choose to proceed. Use for read and non-destructive operations (OQ4:
// "Accept best-effort Kafka" for the general case).
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	return r.sink.Write(ctx, stamp(e))
}

// MustRecord writes an entry fail-closed: if the write fails, the caller MUST
// abort the operation it was about to perform. Use for every destructive,
// cross-tenant action — suspend, suite toggle, impersonation start/stop.
//
// OQ4 → "Fail-closed for destructive ops". Slide 9 → "Mandatory dual audit,
// non-bypassable on start and stop."
func (r *Recorder) MustRecord(ctx context.Context, e Entry) error {
	if err := r.sink.Write(ctx, stamp(e)); err != nil {
		return fmt.Errorf("audit: fail-closed write rejected the operation: %w", err)
	}
	return nil
}

func stamp(e Entry) Entry {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	return e
}
