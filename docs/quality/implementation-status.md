# Implementation status

KiTicket is runnable as a local event-ticketing demonstration. The following distinctions prevent the repository from overstating its maturity.

## Implemented in the application

- NestJS API with Swagger, validation, JWT authentication, role checks and rate limiting.
- React/Vite frontend for discovery, management, checkout and tickets.
- PostgreSQL entities for the catalog, reservations, payments, tickets, notifications and waiting-room entries.
- Redis atomic locks for multi-seat reservations and Redis sequence numbers for waiting-room positions.
- Reservation cancellation and a periodic expiry sweep.
- Sandbox payment completion, idempotency keys, ticket tokens, QR data URLs and organizer/admin check-in.
- RabbitMQ topic publishing and an in-process consumer that saves sandbox notification records.
- Docker Compose, health probes and in-process metrics.

## Known gaps

| Area | Current behavior | Required before a production claim |
|---|---|---|
| Database schema | TypeORM synchronization is available for development | versioned migrations, reviewed rollout and rollback procedures |
| Payment | authenticated caller supplies success, failure or timeout outcome | provider adapter, signed callbacks, reconciliation and refund policy |
| Messaging | direct publish after database work; consumer writes a notification record | transactional outbox/inbox, retries, dead-letter handling and delivery provider |
| Concurrency evidence | Redis lock logic has unit tests | PostgreSQL/Redis integration contention and failure-race tests |
| Waiting room | stored token and manual/initial admission | rate limiting, adaptive capacity, single-use policy and recovery |
| Observability | in-memory request counters and health endpoints | persistent metrics, logs, traces, dashboards and alerts |
| Delivery | Docker Compose works locally; Kubernetes manifests are starter files | CI, image publishing, manifest validation, secrets, backups and controlled rollout |
| End-to-end testing | Go test targets an obsolete multi-service API | a test suite using the current /api routes and data model |

This document records current reality; it is not a release certification.

