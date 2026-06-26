// Package postgres provides pgx-backed implementations of the platform store
// interfaces. Tenant lifecycle, licence admin and the audit chain are described
// in the Solution Design (Slide 8) as "largely exists" — these queries read and
// mutate that existing schema from the cross-tenant /platform context.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/clario360/platform-console/internal/audit"
	"github.com/clario360/platform-console/internal/platform/licensing"
	"github.com/clario360/platform-console/internal/platform/overview"
	"github.com/clario360/platform-console/internal/platform/suites"
	"github.com/clario360/platform-console/internal/platform/tenants"
)

// --- Tenant store ---------------------------------------------------------

// TenantStore implements tenants.Store over Postgres.
type TenantStore struct{ pool *pgxpool.Pool }

// NewTenantStore builds a TenantStore.
func NewTenantStore(pool *pgxpool.Pool) *TenantStore { return &TenantStore{pool: pool} }

func (s *TenantStore) List(ctx context.Context) ([]tenants.Tenant, error) {
	const q = `
		SELECT t.id, t.name, t.status, t.seats_used, t.seats_limit,
		       t.licence_tier, to_char(t.created_at, 'YYYY-MM-DD"T"HH24:MI:SSZ')
		FROM tenants t
		ORDER BY t.name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tenants.Tenant
	for rows.Next() {
		var t tenants.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.SeatsUsed,
			&t.SeatsLimit, &t.LicenceTier, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TenantStore) Get(ctx context.Context, id string) (tenants.Detail, error) {
	const q = `
		SELECT t.id, t.name, t.status, t.seats_used, t.seats_limit,
		       t.licence_tier, to_char(t.created_at, 'YYYY-MM-DD"T"HH24:MI:SSZ'),
		       t.region,
		       COALESCE(
		         (SELECT array_agg(ts.suite_key ORDER BY ts.suite_key)
		          FROM tenant_suites ts
		          WHERE ts.tenant_id = t.id AND ts.enabled), '{}')
		FROM tenants t
		WHERE t.id = $1`
	var d tenants.Detail
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&d.ID, &d.Name, &d.Status, &d.SeatsUsed, &d.SeatsLimit,
		&d.LicenceTier, &d.CreatedAt, &d.Region, &d.ActiveSuites)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenants.Detail{}, tenants.ErrNotFound
	}
	if err != nil {
		return tenants.Detail{}, err
	}
	return d, nil
}

func (s *TenantStore) SetSuspended(ctx context.Context, id string, suspended bool) error {
	status := "active"
	if suspended {
		status = "suspended"
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tenants.ErrNotFound
	}
	return nil
}

// --- Suite store ----------------------------------------------------------

// SuiteStore implements suites.Store over Postgres.
type SuiteStore struct{ pool *pgxpool.Pool }

// NewSuiteStore builds a SuiteStore.
func NewSuiteStore(pool *pgxpool.Pool) *SuiteStore { return &SuiteStore{pool: pool} }

func (s *SuiteStore) Catalog(ctx context.Context) ([]suites.Suite, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, display_name, model FROM suites ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []suites.Suite
	for rows.Next() {
		var su suites.Suite
		if err := rows.Scan(&su.Key, &su.DisplayName, &su.Model); err != nil {
			return nil, err
		}
		out = append(out, su)
	}
	return out, rows.Err()
}

func (s *SuiteStore) TenantState(ctx context.Context, tenantID string) ([]suites.TenantSuiteState, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)`, tenantID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, suites.ErrNotFound
	}

	const q = `
		SELECT s.key,
		       COALESCE(ts.enabled, false)
		FROM suites s
		LEFT JOIN tenant_suites ts
		       ON ts.suite_key = s.key AND ts.tenant_id = $1
		ORDER BY s.key`
	rows, err := s.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []suites.TenantSuiteState
	for rows.Next() {
		var st suites.TenantSuiteState
		if err := rows.Scan(&st.SuiteKey, &st.Enabled); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *SuiteStore) SetEnabled(ctx context.Context, tenantID, suiteKey string, enabled bool) error {
	const q = `
		INSERT INTO tenant_suites (tenant_id, suite_key, enabled, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (tenant_id, suite_key)
		DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, tenantID, suiteKey, enabled); err != nil {
		return err
	}
	return nil
}

// --- Licence store --------------------------------------------------------

// LicenceStore implements licensing.Store over Postgres.
type LicenceStore struct{ pool *pgxpool.Pool }

// NewLicenceStore builds a LicenceStore.
func NewLicenceStore(pool *pgxpool.Pool) *LicenceStore { return &LicenceStore{pool: pool} }

func (s *LicenceStore) Fleet(ctx context.Context) (licensing.FleetSummary, error) {
	const q = `
		SELECT t.id, t.name, t.licence_tier, t.seats_used, t.seats_limit,
		       CASE
		         WHEN t.licence_expires_at < now() THEN 'expired'
		         WHEN t.seats_used > t.seats_limit THEN 'over_limit'
		         WHEN t.licence_expires_at < now() + interval '30 days' THEN 'expiring'
		         ELSE 'active'
		       END AS state,
		       to_char(t.licence_expires_at, 'YYYY-MM-DD') AS renews_at
		FROM tenants t
		ORDER BY t.name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return licensing.FleetSummary{}, err
	}
	defer rows.Close()

	var summary licensing.FleetSummary
	for rows.Next() {
		var r licensing.LicenceRow
		if err := rows.Scan(&r.TenantID, &r.TenantName, &r.Tier,
			&r.SeatsUsed, &r.SeatsLimit, &r.State, &r.RenewsAt); err != nil {
			return licensing.FleetSummary{}, err
		}
		summary.Rows = append(summary.Rows, r)
		summary.TotalSeats += r.SeatsLimit
		summary.SeatsInUse += r.SeatsUsed
		if r.State == "over_limit" {
			summary.OverLimitCount++
		}
		if r.State == "expiring" {
			summary.ExpiringCount++
		}
	}
	return summary, rows.Err()
}

// --- Overview store -------------------------------------------------------

// OverviewStore implements overview.Store over Postgres. OQ3 leaves the metrics
// source open ("Query Prometheus if deployed"); this implementation reads the
// last-known per-service health rows persisted by the fleet health collector.
type OverviewStore struct{ pool *pgxpool.Pool }

// NewOverviewStore builds an OverviewStore.
func NewOverviewStore(pool *pgxpool.Pool) *OverviewStore { return &OverviewStore{pool: pool} }

func (s *OverviewStore) Snapshot(ctx context.Context) (overview.Snapshot, error) {
	var snap overview.Snapshot

	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM tenants`).Scan(&snap.TenantCount); err != nil {
		return overview.Snapshot{}, err
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM service_health`).Scan(&snap.ServicesTotal); err != nil {
		return overview.Snapshot{}, err
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(sum(seats_used), 0) FROM tenants`).Scan(&snap.SeatsInUse); err != nil {
		return overview.Snapshot{}, err
	}

	const q = `
		SELECT name, status, breaker, p95_ms, version, error_rate,
		       to_char(last_checked, 'YYYY-MM-DD"T"HH24:MI:SSZ')
		FROM service_health
		ORDER BY name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return overview.Snapshot{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var sh overview.ServiceHealth
		if err := rows.Scan(&sh.Name, &sh.Status, &sh.Breaker, &sh.P95Millis,
			&sh.Version, &sh.ErrorRate, &sh.LastChecked); err != nil {
			return overview.Snapshot{}, err
		}
		snap.Services = append(snap.Services, sh)
	}
	if err := rows.Err(); err != nil {
		return overview.Snapshot{}, err
	}

	snap.ServicesUp, snap.CriticalCount = overview.CountStatuses(snap.Services)
	return snap, nil
}

// --- Audit sink -----------------------------------------------------------

// AuditSink implements audit.Sink over Postgres. The destructive paths call
// MustRecord, which makes this write fail-closed (OQ4): the INSERT must commit
// before the operation proceeds.
type AuditSink struct{ pool *pgxpool.Pool }

// NewAuditSink builds an AuditSink.
func NewAuditSink(pool *pgxpool.Pool) *AuditSink { return &AuditSink{pool: pool} }

func (s *AuditSink) Write(ctx context.Context, e audit.Entry) error {
	const q = `
		INSERT INTO platform_audit
		  (id, occurred_at, actor_id, actor_roles, action, target_type,
		   target_id, outcome, metadata, impersonated_tenant)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''))`
	_, err := s.pool.Exec(ctx, q,
		e.ID, e.OccurredAt, e.ActorID, e.ActorRoles, e.Action, e.TargetType,
		e.TargetID, string(e.Outcome), e.Metadata, e.ImpersonatedTenant)
	return err
}
