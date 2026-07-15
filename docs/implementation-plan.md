# Implementation plan

## Baseline and gaps

The repository began as a documentation-only course submission. The three PDFs in
`Documents/` and four PNG exports are valid historical artifacts and remain untouched.
There was no Go module, application code, database schema, container definition,
editable diagram source, infrastructure-as-code, functional CI pipeline, or automated
test at baseline. The Git worktree was clean before implementation began.

## Delivery sequence

1. **Foundation**: module layout, configuration, structured logs, HTTP conventions,
   probes, metrics, Compose infrastructure, migrations, seed data, Make targets, and CI.
2. **Identity and catalog**: secure credentials and tokens, dual-layer RBAC, venue/event
   constraints, publication, indexed discovery, pagination, and availability freshness.
3. **Reservation proof**: Redis Lua acquisition/release with fencing and idempotency;
   PostgreSQL booking constraints; high-contention real-infrastructure and race tests.
4. **Waiting room**: signed queue credentials, fair Redis ordering, controlled admission,
   replay defence, status, and documented degraded behaviour.
5. **Purchase saga**: deterministic payment port, persisted transitions, signed callbacks,
   durable booking, ticket issuance, compensation, and transactional outbox/inbox.
6. **Messaging and clients**: RabbitMQ publisher/consumer, retries and dead letters,
   local email/SMS adapters, and SSE order updates.
7. **Assurance**: contract, integration, broker, saga, end-to-end, load, fault, race,
   coverage, mutation, security, and benchmark evidence.
8. **Operational artifacts**: editable/rendered UML, product/risk/threat documents,
   Scrum/Jira materials, Terraform, Kubernetes, telemetry design, runbooks, and
   postmortems.
9. **Final audit**: rebuild from a clean local environment, inspect all traceability rows,
   remove abandoned scaffolding, and create the submission archive.

## Architecture guardrails

- Each bounded context owns its domain model, application use cases, ports, adapters,
  migrations, and executable. Domain packages import only the standard library and
  context-neutral value packages.
- A context may call another context only through a versioned HTTP contract or an
  asynchronous event. It never imports another context's domain package or reads its
  tables directly.
- PostgreSQL is authoritative for identity, catalog, orders, payments, bookings, tickets,
  inboxes, and outboxes. Redis owns only reconstructible/expiring coordination state.
- Commands carry idempotency and correlation identifiers across every boundary. State
  machines reject invalid or stale transitions.
- Local development may share one PostgreSQL and RabbitMQ cluster, but uses separate
  schemas/users/queues to preserve ownership. Production topology can split these without
  changing domain logic.

## Verification cadence

At the end of each implementation phase run the targets that exist at that point:
`make format`, `make lint`, `make test`, `make integration`, `make race`, and
`git diff --check`. Record executed results in `docs/test-evidence.md`; unavailable tools
or infrastructure are reported as blockers and never converted into passing evidence.
