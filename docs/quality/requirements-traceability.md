# Requirements traceability

Status is based on the checked-in source and tests. “Implemented” means a code path exists; it does not imply a production-ready integration. “Unit tested” means a focused Jest test is present, not that a full end-to-end environment has been exercised.

| Capability | Main implementation | Evidence | Status |
|---|---|---|---|
| Registration, login and JWT authentication | backend/src/auth.controller.ts, backend/src/security.ts | DTO validation tests | Implemented; partially unit tested |
| Customer, organizer and administrator roles | backend/src/entities.ts, backend/src/security.ts and controller role decorators | no authorization integration test | Implemented |
| Venue, sector and seat layout | backend/src/catalog.controller.ts | no controller integration test | Implemented |
| Event publishing, pricing, search and analytics | backend/src/catalog.controller.ts | no controller integration test | Implemented |
| Atomic multi-seat Redis lock | backend/src/lock.service.ts | lock.service.spec.ts | Implemented; unit tested |
| Durable seat-allocation guard | backend/src/entities.ts: ReservationSeat unique event/seat pair | no real PostgreSQL contention test | Implemented; needs integration evidence |
| Reservation cancellation and expiry sweep | backend/src/reservations.controller.ts | no expiry-service test | Implemented |
| Idempotent sandbox payment transition | backend/src/payments.controller.ts, payment-policy.ts | payment-policy.spec.ts and payments.controller.spec.ts | Implemented; partially unit tested |
| Ticket issue, QR generation, verification and check-in | backend/src/tickets.controller.ts | no focused test | Implemented |
| Waiting-room entry and expiring admission | backend/src/waiting-room.controller.ts | waiting-room.spec.ts | Implemented; partially unit tested |
| Socket.IO user/event update rooms | backend/src/realtime.gateway.ts | no gateway test | Implemented |
| RabbitMQ event consumption into notification records | backend/src/broker.service.ts | no broker integration test | Implemented with delivery gap |
| Health and process-local metrics | backend/src/health.controller.ts | no endpoint test | Implemented |
| Docker Compose local environment | docker-compose.yml, Dockerfiles and seed script | compose startup is documented; no recorded automated smoke test | Provided |
| Kubernetes and Terraform configuration | infra/k8s/, infra/terraform/ | no target-environment validation | Provided as starter configuration |

## Explicitly not implemented

- external payment-provider integration, signed callbacks, reconciliation and refunds;
- transactional outbox/inbox, durable retry policy and real notification delivery;
- production metrics, dashboards, alerting, backups, restore exercises and deployment CI;
- queue abuse protection, adaptive admission and Redis-loss recovery;
- a current API end-to-end test and a seat-contention load test.

The [implementation status](implementation-status.md) and [backlog](../planning/backlog.md) turn these gaps into concrete next steps.

