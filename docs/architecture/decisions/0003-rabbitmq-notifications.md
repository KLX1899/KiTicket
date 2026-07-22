# ADR 0003: RabbitMQ for asynchronous notification records

**Status:** accepted with known delivery gap

## Context

Reservation, payment and ticket changes should not block the HTTP response while a notification record is created.

## Decision

Publish domain events to the durable ticketing.events topic exchange. The API process also consumes payment and ticket messages from ticketing.notifications and creates a Notification record with a sandbox provider reference.

## Consequences

- RabbitMQ outages do not prevent the database transaction from completing; the API logs the event instead.
- Publishing is not atomic with the database transaction, and the consumer has no inbox or scheduled retry policy.
- The notification consumer does not send email or SMS. A production integration requires an outbox, idempotent consumer storage and a provider adapter.

