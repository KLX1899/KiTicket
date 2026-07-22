# Engineering backlog

This is a forward-looking backlog, not a record of completed work. Items are ordered by risk to booking correctness and operational safety.

| Priority | Work item | Why it matters |
|---:|---|---|
| 1 | Add PostgreSQL/Redis integration contention tests and expiry-versus-payment race tests | Unit tests currently cover lock and transition logic but not the real dependencies together. |
| 2 | Replace tests/e2e/purchase_test.go with an end-to-end test for the current /api contract | The checked-in Go test targets an older multi-service API and is not an executable test of KiTicket today. |
| 3 | Introduce migrations and disable schema synchronization outside local development | Durable schema changes need reviewable, repeatable rollout and rollback behavior. |
| 4 | Implement a payment-provider adapter, signed callbacks and reconciliation | The current payment-completion endpoint is intentionally a sandbox flow. |
| 5 | Add transactional outbox/inbox processing and retry/dead-letter policy | A database commit and RabbitMQ publish can currently diverge. |
| 6 | Harden the waiting room with request limits, adaptive admission and recovery | The current queue has token admission and ordering, but not traffic-abuse or Redis-failure controls. |
| 7 | Add persistent observability, backup/restore checks and deployment CI | Process-local counters and starter manifests are insufficient for production operations. |
| 8 | Build a seat-contention load scenario instead of the catalog-only k6 script | The existing k6 script exercises event listing only. |

