# ADR 0003: RabbitMQ for asynchronous domain events

- Status: Accepted
- Date: 2026-07-14

## Context

The platform needs burst buffering, notification isolation, routing, retries, and dead-letter
handling. The existing repository did not standardize on a broker.

## Decision

Use RabbitMQ with durable topic exchanges, quorum queues in production, explicit
publisher confirms, manual consumer acknowledgements, bounded retry queues, and a
dead-letter exchange. Event envelopes are versioned and include event ID, type, aggregate
ID, occurrence time, correlation ID, causation ID, and JSON payload.

Services publish only from a PostgreSQL transactional outbox. Consumers record event IDs
in an inbox in the same transaction as their local effect before acknowledging. Ordering is
required only per aggregate and consumers reject or harmlessly ignore stale transitions.

## Consequences

- RabbitMQ is simple to run locally and directly supports the required routing/retry/DLQ
  topology.
- Delivery is at least once; idempotent consumers are mandatory.
- The outbox dispatcher is observable and replayable, avoiding database/broker dual writes.
