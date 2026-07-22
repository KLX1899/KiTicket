# ADR 0005: Stored waiting-room admission token

**Status:** accepted

## Context

An event can need to limit admission to reservation commands without treating a queue position as a seat reservation.

## Decision

Create a WaitingRoomEntry for a signed-in user. Redis assigns the per-event sequence number; PostgreSQL stores the entry. Initial capacity can immediately admit entries, and an administrator can admit queued entries in sequence order. Admission is represented by a random stored token with an expiry.

## Consequences

- The reservation endpoint can verify event, user, token state and expiry directly from PostgreSQL.
- The token is not signed, single-use, rate-limited or bound to a device.
- Queue recovery, anti-abuse measures and adaptive admission are future work.

