# Final defense question guide

This guide describes the repository as it exists today: a locally runnable demonstration,
not a production-certified ticketing platform. Use the limits below explicitly in the defense.

## Core architecture answers

- **Why not microservices now?** A modular monolith fits a two-person operational budget. Auth, Catalog,
  Reservations, Payments, Tickets, Waiting Room and Notifications are clear code-level responsibilities,
  so they can be extracted later if scale or isolation evidence justifies it.
- **Who owns availability?** Reservation, not Catalog. Catalog can show stale availability; allocation
  validates the selected seats, acquires temporary locks and writes the authoritative reservation.
- **How is double booking prevented?** Redis Lua acquires all requested seat locks atomically with one TTL.
  PostgreSQL records the reservation and enforces the durable event/seat uniqueness guard. A lock may be
  released only by its owning reservation.
- **Why TTL plus a sweep?** Redis TTL recovers abandoned coordination state. The periodic sweep expires
  durable pending reservations, removes their temporary seat records and attempts to release their locks.
- **How does payment work?** It is a sandbox workflow: an authenticated caller supplies success, failure or
  timeout. Payment creation uses an idempotency key; completion runs in a database transaction. Success
  confirms the reservation and issues tickets; failure or timeout releases the seats.
- **How are DB state and RabbitMQ atomic?** They are not. Events are published directly after database work,
  so a database commit and a publish can diverge. There is currently no transactional outbox, inbox, retry
  scheduler or reconciliation worker; this is a stated production gap.
- **Why RabbitMQ?** The current durable topic exchange and in-process consumer decouple sandbox notification
  records from the request path. It is not yet a real notification-delivery pipeline.
- **Why Socket.IO plus REST?** Socket.IO provides authenticated, user- and event-scoped update hints. REST
  remains the authoritative way to read the current state after reconnects or missed events.
- **Why a waiting room?** When enabled, it requires an expiring admission token tied to the user and event.
  Redis assigns a per-event sequence; initial capacity or an administrator admits entries in sequence order.
  It does not yet include rate limits, adaptive capacity, single-use tokens or Redis-loss recovery.
- **What happens if Redis fails?** The current application cannot safely make the stronger claim of a
  graceful fail-closed/recovery workflow. New Redis-dependent operations fail, while PostgreSQL remains the
  system of record; recovery design and integration evidence are backlog work.
- **What proves correctness today?** Focused unit tests cover validation, exceptions, Redis-lock behavior,
  payment transitions/idempotency and waiting-room admission. Real PostgreSQL/Redis contention and failure
  races, RabbitMQ integration, and current end-to-end purchase evidence are still required.

## Product and delivery answers

- The demonstrated product includes discovery, management, waiting-room admission, seat reservations,
  sandbox payment, QR tickets and organizer/admin check-in.
- Metrics are process-local request counters exposed in Prometheus text format; they reset on restart. There
  are no persistent dashboards, alerts or production SLO evidence.
- Docker Compose, Kubernetes manifests and Terraform are delivery starting points. The repository has no
  checked-in CI workflow or validated production deployment pipeline.
- The included k6 script exercises event listing only, and the Go end-to-end test targets an obsolete API;
  neither should be presented as purchase-contention proof.
- State these limits plainly, then point to the backlog: migrations, provider-backed payments, outbox/inbox,
  retry/dead-letter handling, integration/load tests, observability, backups and controlled delivery.

## Demo order

Product goal → component/deployment diagram → login and discovery → waiting-room admission → two-buyer seat
conflict → sandbox payment success and QR → failed/timeout payment and seat release → unit-test and health/
metrics evidence → Docker/Kubernetes/Terraform artifacts → known limits and backlog.

For supporting material, use [the documentation index](docs/README.md),
[architecture overview](docs/architecture/overview.md),
[verification guide](docs/quality/verification.md),
[implementation status](docs/quality/implementation-status.md), and
[backlog](docs/planning/backlog.md).
