# ADR 0004: Persisted checkout orchestration with outbox/inbox

- Status: Accepted
- Date: 2026-07-14

## Context

Reservation validation, an external payment, durable booking, ticket issuance, and
notification cannot share a distributed transaction. Payment callbacks can be duplicated,
late, forged, or arrive after cancellation.

## Decision

Checkout is an explicit persisted orchestration state machine. Its normal path is
`pending -> payment_pending -> paid -> booking_confirmed -> ticket_issued -> completed`.
Failure states retain reason and transition version. Each command has a scoped idempotency
record whose request hash prevents key reuse with different input.

The deterministic local payment adapter simulates initiate/confirm/fail/timeout without
card data. Production adapters expose the same port. Webhook payloads are authenticated,
time-bounded, and replay-protected. State transitions, accounting references, booking,
ticket, and outbox records are committed in local PostgreSQL transactions.

Compensation releases a reservation after payment failure, timeout, cancellation, or
unavailable provider. If payment succeeded but booking cannot be created, the saga enters
`refund_pending`; it retries a provider refund and never reports completion. Duplicate or
out-of-order facts are resolved using inbox uniqueness and expected state/version.

## Consequences

- There is no fake atomicity across services; intermediate and recovery states are visible.
- Support can replay outbox records and inspect transition history.
- Provider uncertainty is represented explicitly rather than treated as failure.
