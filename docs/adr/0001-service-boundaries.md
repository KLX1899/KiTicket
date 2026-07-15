# ADR 0001: Service boundaries and dependency direction

- Status: Accepted
- Date: 2026-07-14

## Context

Seat availability changes much more frequently than event metadata. Checkout must not
make catalog or notification availability part of the payment consistency boundary. The
course artifacts require decoupling, while a portfolio repository must remain runnable by
one developer.

## Decision

Use a service-oriented monorepo with independently runnable Go executables for gateway,
identity, catalog, reservation, waiting-room, checkout/ticket, and notification-worker.
Each context follows domain -> application -> port <- adapter dependency direction.

The reservation context owns sellable inventory state and answers availability summaries.
Catalog owns structural venue/event/schedule/pricing metadata and may display a
time-bounded availability projection supplied by reservation. Checkout owns the purchase
saga; ticket issuance is kept in the same transactional service because a paid booking and
its unique ticket require one local consistency boundary. Notification consumes facts and
cannot block purchases.

The gateway authenticates, rate-limits, and routes, but every privileged application use
case repeats authorization. Synchronous internal calls use bounded HTTP timeouts;
cross-context facts use RabbitMQ and transactional outbox/inbox delivery.

## Consequences

- No context imports another context's domain package or queries its tables.
- One local PostgreSQL/RabbitMQ installation is economical, while ownership remains
  separable through schemas, credentials, and queues.
- Checkout/ticket can be split later at the outbox boundary if their scaling profiles
  diverge; doing so now would add a distributed consistency problem without value.
- Availability is explicitly freshness-bounded rather than presented as a catalog truth.
