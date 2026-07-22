# Architecture overview

KiTicket is a modular NestJS application with a React/Vite client. It runs as one API process rather than as separately deployed domain services.

## Runtime components

| Component | Responsibility | Backing dependency |
|---|---|---|
| React client | discovery, venue/event management, checkout and ticket views | REST and Socket.IO |
| NestJS API | authentication, authorization, catalog, reservations, payments, tickets and waiting room | PostgreSQL, Redis, RabbitMQ |
| PostgreSQL | durable users, catalog, reservations, payments, tickets, notifications and waiting-room entries | TypeORM |
| Redis | atomic temporary seat locks and per-event waiting-room sequence counters | Lua scripts and INCR |
| RabbitMQ | best-effort domain-event fan-out to the in-process notification consumer | durable topic exchange and queue |
| Socket.IO | user- and event-scoped update hints | API process memory |

## Reservation and payment flow

1. A signed-in customer joins the waiting room if WAITING_ROOM_REQUIRED=true. Admission tokens are random, stored with the waiting-room entry, tied to the event and user, and expire after the configured TTL.
2. POST /api/reservations verifies the event, selected seats, prices and admission token. Redis then atomically locks every requested seat. PostgreSQL writes the pending reservation and its locked seats in one transaction.
3. A background sweep expires pending reservations after LOCK_TTL_SECONDS, removes their seat records and releases their Redis locks. Customers may also cancel a pending reservation.
4. POST /api/payments creates one pending payment per idempotency key. POST /api/payments/:id/complete applies the sandbox outcome inside a database transaction. Success books seats and creates tickets; failure or timeout removes the temporary seat records. The Redis locks are released after the transaction.

## Important limits

- RabbitMQ publishing is not transactional with PostgreSQL: there is no outbox, inbox, retry scheduler or reconciliation worker.
- The waiting room uses an atomic Redis sequence but does not yet have rate limits, adaptive admission or a recovery mechanism for Redis loss.
- Metrics are process-local counters only. They reset on restart and are not a complete Prometheus instrumentation layer.
- DB_SYNCHRONIZE is convenient for local development. Production requires migrations and must keep it disabled.

See [implementation status](../quality/implementation-status.md) for the complete list of follow-up work.

