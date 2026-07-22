# ADR 0002: Redis lock plus PostgreSQL allocation record

**Status:** accepted

## Context

Seat selection is concurrent and a pending reservation must not oversell a seat.

## Decision

Use a Redis Lua script to acquire all requested seat locks atomically with one TTL. After the lock succeeds, create the Reservation and ReservationSeat rows in a PostgreSQL transaction. PostgreSQL enforces UNIQUE(eventId, seatId) on ReservationSeat as the durable final guard.

## Consequences

- A failed database transaction attempts to release the acquired Redis locks.
- Cancellation and expiry remove ReservationSeat rows and release owner-held locks.
- Redis is coordination state, not the source of booking truth.
- Integration tests against real PostgreSQL and Redis are still needed to prove failure and race behavior beyond the current unit tests.

