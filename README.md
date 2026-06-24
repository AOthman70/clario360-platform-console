# Platform Administrative Console

The cross-tenant, cross-suite control plane described in the Clario360
**Platform Administrative Console — Solution Design** (Draft v1). A distinct
`/platform` area for the platform super-admin, SRE and owner — cross-tenant by
default, safe by design, built on the codebase's own primitives.

This repository is the **P0** service skeleton: the gateway wiring, the
`admin:console` permission gate (including the G-0 wildcard fix), and the four
must-have screens — Overview, Tenants, Suite catalog, Licensing — each with a
concrete Postgres-backed store and fail-closed audit on destructive actions.

## Stack

Grounded in the verified architecture (Slide 5):

| Concern        | Choice                                   |
|----------------|------------------------------------------|
| Router         | chi v5                                    |
| AuthN          | RS256 JWT, algorithm pinned on verify     |
| AuthZ          | `admin:console` gate, wildcard-aware       |
| Data           | Postgres via pgx/pgxpool                   |
| Audit          | fail-closed on destructive ops (OQ4)       |

## Initialize and run

```bash
# 1. Resolve dependencies (writes go.sum).
make tidy        # or: go mod tidy

# 2. Generate a local RSA keypair for JWT verification.
make keys

# 3. Stand up Postgres and apply the baseline schema.
#    (Flyway shown; adapt to your migration tool.)
make migrate

# 4. Configure the environment.
cp .env.example .env   # then edit values
set -a; . ./.env; set +a

# 5. Build and run.
make build && ./bin/platform-console
#    or just: make run
```

Liveness check:

```bash
curl -s localhost:8080/healthz
```

Every `/platform` route requires a bearer token whose claims satisfy
`admin:console`. A super-admin holding `admin:*` is admitted; a `tenant_admin`
is not (and receives 404, never a 403 that would confirm the area exists).

## How the code maps to the design

| Deck reference | Where it lives |
|----------------|----------------|
| Slide 3 — distinct `/platform` area, renders everywhere | `internal/gateway/router.go` |
| Slide 4 — `admin:console` gate, the wildcard trap & fix (G-0) | `internal/auth/permissions.go` (+ test) |
| Slide 5 — chi v5, RS256, strips `X-Tenant-ID` | `internal/auth/{jwt,middleware}.go` |
| Slide 6 — ten screens, all gated on `admin:console` | `internal/gateway/router.go` |
| Slide 7 — Overview anatomy, resilient when degraded | `internal/platform/overview` |
| Slide 9 — guarded impersonation controls | `auth.DenyImpersonatedWrites`, `Claims` act-as fields |
| Slide 10 — P0 scope: real suspend, suite toggle, confirm + audit | `internal/platform/{tenants,suites}` |
| OQ3 — fleet metrics source | `OverviewStore` (Prometheus- or scrape-backed) |
| OQ4 — fail-closed audit for destructive ops | `internal/audit`, `MustRecord` |

## Layout

```
platform-console/
├── cmd/server/            # entrypoint, graceful shutdown
├── internal/
│   ├── auth/              # JWT verify, permission gate (G-0 fix), middleware
│   ├── audit/             # fail-closed recorder + entry model
│   ├── config/            # env-driven config
│   ├── gateway/           # chi router; /platform gated on admin:console
│   ├── httpx/             # JSON + error helpers
│   ├── platform/          # P0 screen handlers
│   │   ├── overview/      ├── tenants/  ├── suites/  └── licensing/
│   └── store/postgres/    # pgx implementations of every store interface
├── migrations/            # baseline schema + seed
├── deploy/                # (reserved)
├── Dockerfile
├── Makefile
└── .env.example
```

## P1 / P2 — not yet built

Out of scope for this P0 skeleton, tracked for the next phases: Identity &
Access (ABAC), platform-wide Audit + export, Service ops (gateway breakers /
kill switches), AI governance, Provisioning status, and — pending owner
sign-off (OQ1) — guarded impersonation. The Revenue & Operations operator
screens (Slides 12–17) require a separate LLD.

## Note on the seeded baseline

`migrations/V1__platform_console_baseline.sql` expresses the columns the P0 read
and mutate paths depend on. Slide 8 marks tenant lifecycle, licence admin and
the audit chain as already existing upstream — reconcile this baseline against
the live schema rather than applying it blindly where those tables already exist.
