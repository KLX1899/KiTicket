# ADR 0005: Waiting-room fairness and admission

- Status: Accepted
- Date: 2026-07-14

## Context

Flash-sale traffic must be admitted at a bounded rate without trusting client-provided
position or identity. Strict global fairness is impossible across partitions and retries.

## Decision

Redis holds an event-scoped sorted set ordered by server enqueue time with a monotonic
sequence tie-breaker. Joining is idempotent per user/event. An admission worker uses one
Lua script to move the oldest members into an expiring admitted set under a token-bucket
rate and concurrency cap.

Queue credentials are server-signed and bind token ID, user, event, issued/expiry times,
and admission generation. Reservation requests require an admitted token for protected
events. A token is consumed once for its intended boundary; Redis records replay state.
Signing uses key rotation identifiers and constant-time verification.

If Redis is unavailable, protected-event joins/status return a retryable 503 and core
services fail closed; unprotected discovery remains available. Workers are horizontally
safe because admission is atomic. If workers stop, positions remain durable in Redis but
no new users are admitted; alerts fire on queue age and admission lag.

## Consequences

- FIFO is best-effort by accepted server arrival, not by client clock or network send time.
- Redis persistence improves recovery but queue state remains operational, not financial.
- Operators may pause admission without discarding positions.
