package overview

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestCountStatuses locks in the KPI tally semantics behind the Overview tiles
// (Slide 7): "up" counts only healthy services and "critical" counts only down
// ones. A degraded service is reported in the grid but flips neither KPI.
func TestCountStatuses(t *testing.T) {
	svc := func(status string) ServiceHealth { return ServiceHealth{Status: status} }

	cases := []struct {
		name         string
		services     []ServiceHealth
		wantUp       int
		wantCritical int
	}{
		{"empty", nil, 0, 0},
		{"all healthy", []ServiceHealth{svc("healthy"), svc("healthy")}, 2, 0},
		{"all down", []ServiceHealth{svc("down"), svc("down"), svc("down")}, 0, 3},
		{
			// Degraded is counted as neither — the documented semantics.
			"mixed with degraded ignored",
			[]ServiceHealth{svc("healthy"), svc("degraded"), svc("down"), svc("healthy")},
			2, 1,
		},
		{"only degraded", []ServiceHealth{svc("degraded"), svc("degraded")}, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			up, critical := CountStatuses(c.services)
			if up != c.wantUp || critical != c.wantCritical {
				t.Errorf("CountStatuses() = (up=%d, critical=%d), want (up=%d, critical=%d)",
					up, critical, c.wantUp, c.wantCritical)
			}
		})
	}
}

// fakeStore is a test double for overview.Store: it returns a fixed snapshot or
// a fixed error, never touching a database.
type fakeStore struct {
	snap Snapshot
	err  error
}

func (f fakeStore) Snapshot(context.Context) (Snapshot, error) {
	return f.snap, f.err
}

// TestHandlerGet_Success asserts the handler passes the store's snapshot through
// as 200 JSON with the expected envelope and field tags.
func TestHandlerGet_Success(t *testing.T) {
	want := Snapshot{
		TenantCount:   3,
		ServicesUp:    2,
		ServicesTotal: 3,
		SeatsInUse:    42,
		CriticalCount: 1,
		Services: []ServiceHealth{
			{Name: "api-gateway", Status: "healthy", Breaker: "closed", P95Millis: 12.5, Version: "1.0.0"},
			{Name: "iam-service", Status: "down", Breaker: "open", P95Millis: 0, Version: "1.0.0"},
			{Name: "file-service", Status: "degraded", Breaker: "half-open", P95Millis: 88, Version: "1.0.0"},
		},
	}
	h := New(fakeStore{snap: want})

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/platform/overview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot round-trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// TestHandlerGet_StoreError asserts a store failure maps to 502 with the shared
// error envelope (httpx.Error), not a 500 or a leaked internal error.
func TestHandlerGet_StoreError(t *testing.T) {
	h := New(fakeStore{err: errors.New("connection refused")})

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/platform/overview", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error != "unable to read fleet snapshot" {
		t.Errorf("error = %q, want %q", body.Error, "unable to read fleet snapshot")
	}
}

// TestHandlerGet_Resilient proves the screen renders even when the fleet is
// unhealthy (Slide 7: "resilient by design"). Degraded/down services arrive as
// data, so the handler still returns 200 rather than erroring.
func TestHandlerGet_Resilient(t *testing.T) {
	h := New(fakeStore{snap: Snapshot{
		ServicesTotal: 2,
		CriticalCount: 1,
		Services: []ServiceHealth{
			{Name: "siem-service", Status: "down"},
			{Name: "data-service", Status: "degraded"},
		},
	}})

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/platform/overview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — a degraded fleet must still render", rec.Code, http.StatusOK)
	}
}
