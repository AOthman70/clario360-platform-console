-- V1__platform_console_baseline.sql
-- Baseline schema for the Platform Administrative Console (P0).
-- Slide 8 notes tenant lifecycle, licence admin and the audit chain "largely
-- exist"; this baseline expresses the columns the /platform read and mutate
-- paths depend on. Adapt to your live schema rather than applying verbatim if
-- these tables already exist upstream.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Tenants -------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
    id                  TEXT PRIMARY KEY,
    name                TEXT        NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'active'
                                    CHECK (status IN ('active', 'suspended')),
    seats_used          INTEGER     NOT NULL DEFAULT 0,
    seats_limit         INTEGER     NOT NULL DEFAULT 0,
    licence_tier        TEXT        NOT NULL DEFAULT 'starter',
    licence_expires_at  TIMESTAMPTZ NOT NULL DEFAULT now() + interval '365 days',
    region              TEXT        NOT NULL DEFAULT 'ksa-central',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Suite catalog -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS suites (
    key          TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    model        TEXT NOT NULL CHECK (model IN ('seat-based', 'capacity-based'))
);

-- Per-tenant suite entitlement ---------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_suites (
    tenant_id  TEXT        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    suite_key  TEXT        NOT NULL REFERENCES suites(key) ON DELETE CASCADE,
    enabled    BOOLEAN     NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, suite_key)
);

-- Last-known service health (OQ3: fed by collector or Prometheus mirror) -----
CREATE TABLE IF NOT EXISTS service_health (
    name         TEXT PRIMARY KEY,
    status       TEXT        NOT NULL CHECK (status IN ('healthy', 'degraded', 'down')),
    breaker      TEXT        NOT NULL DEFAULT 'closed',
    p95_ms       DOUBLE PRECISION NOT NULL DEFAULT 0,
    version      TEXT        NOT NULL DEFAULT '',
    error_rate   DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_checked TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Cross-tenant platform audit (OQ4: fail-closed for destructive ops) --------
CREATE TABLE IF NOT EXISTS platform_audit (
    id                  UUID PRIMARY KEY,
    occurred_at         TIMESTAMPTZ NOT NULL,
    actor_id            TEXT        NOT NULL,
    actor_roles         TEXT[]      NOT NULL DEFAULT '{}',
    action              TEXT        NOT NULL,
    target_type         TEXT        NOT NULL,
    target_id           TEXT        NOT NULL,
    outcome             TEXT        NOT NULL CHECK (outcome IN ('success', 'failure')),
    metadata            JSONB,
    impersonated_tenant TEXT
);

CREATE INDEX IF NOT EXISTS idx_platform_audit_occurred_at
    ON platform_audit (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_platform_audit_target
    ON platform_audit (target_type, target_id);

-- Seed the 14 services from Slide 5 so the Overview grid is populated --------
INSERT INTO service_health (name, status, version) VALUES
    ('api-gateway',     'healthy', '1.0.0'),
    ('iam-service',     'healthy', '1.0.0'),
    ('audit-service',   'healthy', '1.0.0'),
    ('licence-service', 'healthy', '1.0.0'),
    ('file-service',    'healthy', '1.0.0'),
    ('workflow-service','healthy', '1.0.0'),
    ('notify-service',  'healthy', '1.0.0'),
    ('automation-service','healthy','1.0.0'),
    ('cyber-service',   'healthy', '1.0.0'),
    ('siem-service',    'healthy', '1.0.0'),
    ('data-service',    'healthy', '1.0.0'),
    ('acta-service',    'healthy', '1.0.0'),
    ('watheeq-service', 'healthy', '1.0.0'),
    ('visus-service',   'healthy', '1.0.0'),
    ('dr-service',      'healthy', '1.0.0')
ON CONFLICT (name) DO NOTHING;

-- Seed the suite catalog (Slides 5, 14) -------------------------------------
INSERT INTO suites (key, display_name, model) VALUES
    ('business_plus', 'Business+',  'seat-based'),
    ('datastream',    'DataStream', 'capacity-based')
ON CONFLICT (key) DO NOTHING;
