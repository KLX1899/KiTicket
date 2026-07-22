# Final defense question guide

## Core architecture answers

- **Why not microservices now?** A modular monolith matches a two-person operational budget; explicit bounded
  contexts/events preserve extraction when scaling, cadence or isolation evidence justifies it.
- **Who owns availability?** Reservation, not Catalog. Catalog may project a stale count; allocation performs
  authoritative admission, lock and DB checks.
- **How is double booking prevented?** One atomic Redis multi-key lease gives fast ownership; PostgreSQL unique
  event/seat is the durable guard. Release verifies the reservation owner.
- **Why TTL plus sweeper?** Redis TTL self-recovers abandoned coordination; a conditional DB sweeper transitions
  durable PENDING state and repairs inventory/events. Neither alone is sufficient.
- **Why is payment timeout not failure?** Funds may be captured while the response is lost. The saga reconciles
  by idempotency/reference before compensating, preventing charged-without-seat races.
- **How are DB state and RabbitMQ atomic?** They are not dual-written. State and outbox commit together; relay
  publishes at least once; inbox IDs make consumers idempotent.
- **Why RabbitMQ?** Durable routing/retry/DLX decouples notification/analytics latency and failure from checkout.
- **Why WebSocket plus REST?** Socket is low-latency advisory delivery; REST/version reconciliation recovers gaps.
- **Why a waiting room?** It bounds consistency-sensitive load and gives explicit FIFO admission; raw rate limiting
  is not fair because aggressive retry can win.
- **What if Redis fails?** Stop admission and fail new allocation closed. Durable DB state remains safe; reconcile
  stale records/locks before gradual reopen.
- **What proves correctness?** Domain invariants, 100-way contention with one winner, expiry/callback race,
  idempotency/mutation tests and post-load SQL—not HTTP success rate alone.

## Product/process answers

- Metrics include zero oversell, paid-ticket reconciliation, latency/SLO, queue abandonment/conversion and
  dashboard freshness. Conversion never overrides correctness/fairness.
- Done requires acceptance, failure/authorization paths, independent review, CI evidence and updated docs/UML.
- Heavy mutation/load suites run in controlled CI; the live defense uses retained reports to protect the laptop.
- Postmortems are declared simulations, blameless and action-oriented; they do not fabricate production history.
- The traceability matrix distinguishes complete design from prototype/verification still required.

## Demo order

Product goal → component/deployment → login/discovery → waiting admission → two-buyer seat conflict → payment
success and QR → failure/compensation → invariant/QA evidence → Jira/CI/IaC → monitoring/postmortem → limitations.
Use `docs/team/TEAM_DEFENSE_PLAN.md` for timing and fallback.
