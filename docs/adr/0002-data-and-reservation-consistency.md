# ADR 0002: PostgreSQL/Redis responsibilities and reservation consistency

- Status: Accepted
- Date: 2026-07-14

## Context

Seat selection needs low-latency, atomic, expiring coordination. Completed bookings must
survive Redis loss and must never double-book a schedule/seat pair.

## Decision

Redis is the transient lock coordinator only. One Lua invocation validates all requested
keys and then creates every lock atomically with a common reservation ID, owner, fencing
token, idempotency key, and TTL. Seat identifiers are sorted before scripting, producing a
deterministic conflict result. Repeating the same command returns the original outcome;
reuse with a different payload is rejected. Release/renew operations compare owner,
reservation, and fencing values so stale clients cannot delete or extend a newer lock.

PostgreSQL is the durable source of truth. Finalization locks relevant inventory rows in a
stable order and inserts bookings in one transaction. A unique constraint on
`(schedule_id, seat_id)` is the final invariant. The transaction persists the order change,
booking, ticket, and outbox event together where applicable. Redis locks are released only
after commit and may safely expire if cleanup fails.

## Consistency and failures

Availability before checkout is an expiring projection and therefore eventually
consistent. Booking finalization is strongly consistent in PostgreSQL. Redis loss rejects
new locks (fail closed), while existing locks may disappear; checkout still validates
ownership data and durable availability before charging/finalizing. A database outage
prevents payment finalization and triggers retry/compensation according to the persisted
saga state. A process crash between database commit and Redis cleanup leaves only a
temporary stale lock, never a double booking.

## Consequences

- Redis is never evidence of a completed purchase.
- Multi-seat requests are all-or-nothing.
- Database constraints remain necessary even when the Redis algorithm is correct.
- Fencing and TTL values are part of the reservation contract and observable state.
