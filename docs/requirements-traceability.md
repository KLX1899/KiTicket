# Requirements traceability

This matrix is the delivery ledger for `SE_Project.pdf`. The PDF was read in full on
2026-07-14 (11 pages; Sections 1-7). A row is **Complete** only when its artifact exists
and the evidence column names a reproducible check or an inspectable repository artifact.

Status meanings:

- **Complete**: implemented and backed by the cited evidence.
- **Partial**: useful evidence exists, but one or more required behaviours are absent or unverified.
- **Not started**: no qualifying implementation or evidence exists yet.
- **Not applicable**: cannot be delivered as repository work; the rationale is recorded.

Historical PDFs under `Documents/` are preserved as prior work. They are supporting
evidence, not editable sources and not substitutes for missing implementation.

| Requirement ID | Requirement | Implementation / artifact path | Test / evidence | Status |
|---|---|---|---|---|
| PDF-1.1-01 | Frictionless discovery and booking with live layout, availability, and pricing | `internal/catalog`, `internal/reservation`, `api/openapi.yaml` | End-to-end buyer test (planned) | Not started |
| PDF-1.1-02 | Prevent double booking and remain graceful during traffic bursts | `internal/reservation`, `internal/waitingroom` | High-contention race and load tests (planned) | Not started |
| PDF-1.1-03 | Reliable checkout, immediate issuance, and automatic release after failure/timeout | `internal/checkout`, `internal/ticket`, `internal/reservation` | Saga failure-path and end-to-end tests (planned) | Not started |
| PDF-1.1-04 | Decoupled domains that can evolve and scale independently | `docs/adr/0001-service-boundaries.md`, `cmd/` | Dependency/build checks (planned) | Partial |
| PDF-1.2-01 | Dependable, zero-downtime buyer journey that earns organizer trust | `deployments/k8s`, `docs/operations` | Deployment probes, rollout policy, SLO evidence (planned) | Not started |
| PDF-1.2-02 | Organizer real-time sales, revenue, and remaining-capacity analytics | `internal/reporting`, `api/openapi.yaml` | Analytics API tests (planned) | Not started |
| PDF-1.2-03 | Transactional SMS/email updates for booking, payment, and schedule changes | `internal/notification` | Consumer/provider retry tests (planned) | Not started |
| PDF-1.2-04 | Extensible infrastructure and domains for future market growth | `docs/adr/0001-service-boundaries.md`, `infra/terraform`, `deployments/k8s` | Architecture review; IaC validation (planned) | Partial |
| PDF-2.1-01 | Centralized authentication and buyer/organizer/admin authorization | `internal/identity`, `internal/platform/httpx`, `api/openapi.yaml` | Identity and authorization tests (planned) | Not started |
| PDF-2.1-02 | Admin venue layouts, sectors/rows/seats, schedules, and custom pricing | `internal/catalog`, `migrations`, `api/openapi.yaml` | Catalog domain/API/integration tests (planned) | Not started |
| PDF-2.1-03 | Optimized filtering by genre, date, location, and live availability | `internal/catalog`, `migrations`, `api/openapi.yaml` | Query-plan and API tests (planned) | Not started |
| PDF-2.1-04 | Temporary, race-safe, high-performance seat locking | `internal/reservation` | Redis integration, contention, benchmark, and race tests (planned) | Not started |
| PDF-2.1-05 | Saga/compensation for failed or aborted distributed checkout | `internal/checkout`, `docs/adr/0004-saga-outbox-inbox.md` | Saga transition/failure tests (planned) | Partial |
| PDF-2.1-06 | Broker-backed live order status and unique QR ticket issuance | `internal/ticket`, `internal/realtime`, `internal/messaging` | Ticket, broker, and stream contract tests (planned) | Not started |
| PDF-2.1-07 | Virtual waiting room and traffic throttling | `internal/waitingroom`, `docs/adr/0005-waiting-room-fairness.md` | Token, ordering, admission, and failure tests (planned) | Partial |
| PDF-2.2-01 | Comprehensive architectural risk mitigation document | `docs/risk-analysis.md`, `Documents/Risk Mitigation Analysis - KiTicket.pdf` | Required risk-scenario checklist (planned); historical PDF covers four scenarios | Partial |
| PDF-2.2-02 | Product vision with long-term goals, personas, and business metrics | `docs/product-vision.md`, `Documents/Product Vision - KiTicket.pdf` | Inspectable three-page historical PDF with goals, four personas, and KPIs | Complete |
| PDF-3.0-01 | Professional standard UML exported as structured high-quality vector files | `docs/diagrams/*.puml`, `docs/diagrams/rendered/*.svg`, `Documents/UML Architecture Models - KiTicket.pdf` | Source/render parity check (planned); historical PDF is incomplete | Partial |
| PDF-3.1-UC-01 | Use case: discovery by tags, dates, and available capacity | `docs/diagrams/use-case.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-UC-02 | Use case: dynamic seat selection and temporary locks | `docs/diagrams/use-case.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-UC-03 | Use case: secure checkout and final ticket token | `docs/diagrams/use-case.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-UC-04 | Use case: venue layout, pricing, and schedule administration | `docs/diagrams/use-case.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-01 | Class model: event details, dates, classifications, organizer | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-02 | Class model: venue/hall dimensions, capacity, and sectors | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-03 | Class model: seat coordinates and available/locked/booked state | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-04 | Class model: user profiles and permissions | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-05 | Class model: reservation state and TTL | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-06 | Class model: payment token, reference, and accounting details | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CL-07 | Class model: verifiable ticket QR hash and metadata | `docs/diagrams/class.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-SQ-01 | Sequence: waiting room through lock, payment, issuance, and timeout rollback | `docs/diagrams/booking-sequence.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-SQ-02 | Sequence: asynchronous order events through broker to notifications | `docs/diagrams/notification-sequence.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-AC-01 | Activity: buyer login-to-checkout success, timeout, and cancellation paths | `docs/diagrams/user-purchase-activity.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-AC-02 | Activity: venue onboarding, pricing, scheduling, and publication | `docs/diagrams/admin-event-activity.puml` | Rendered SVG (planned) | Not started |
| PDF-3.1-CO-01 | Component: identity and access boundary | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and dependency checks (planned) | Partial |
| PDF-3.1-CO-02 | Component: event catalog and discovery boundary | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and dependency checks (planned) | Partial |
| PDF-3.1-CO-03 | Component: reservation and inventory boundary | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and dependency checks (planned) | Partial |
| PDF-3.1-CO-04 | Component: billing and checkout boundary | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and dependency checks (planned) | Partial |
| PDF-3.1-CO-05 | Component: notification and messaging boundary | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and dependency checks (planned) | Partial |
| PDF-3.1-CO-06 | Component: API gateway edge, security, and rate limiting | `docs/diagrams/component.puml`, `docs/adr/0001-service-boundaries.md` | Rendered SVG and gateway tests (planned) | Partial |
| PDF-3.1-DP-01 | Deployment: Kubernetes orchestration and autoscaling | `docs/diagrams/deployment.puml`, `deployments/k8s` | Rendered SVG; manifest validation (planned) | Not started |
| PDF-3.1-DP-02 | Deployment: PostgreSQL durability and Redis transient state | `docs/diagrams/deployment.puml`, `docs/adr/0002-data-and-reservation-consistency.md` | Rendered SVG; integration tests (planned) | Partial |
| PDF-3.1-DP-03 | Deployment: asynchronous broker topology | `docs/diagrams/deployment.puml`, `docs/adr/0003-message-broker.md` | Rendered SVG; broker tests (planned) | Partial |
| PDF-3.1-IAC-01 | Declarative Terraform for instances, load balancers, and managed databases | `infra/terraform` | `terraform fmt -check` and `terraform validate` (planned) | Not started |
| PDF-4.1-01 | Clean prioritized product backlog including layout, queue, concurrency, and reporting | `docs/planning/product-backlog.md` | Backlog completeness review (planned) | Not started |
| PDF-4.1-02 | Domain epics and granular value-oriented user stories | `docs/planning/product-backlog.md`, `docs/planning/jira-import.csv` | Every story has actor/value/acceptance criteria (planned) | Not started |
| PDF-4.1-03 | Sprint plan: schema/access controls before queue/gateway integration | `docs/planning/sprint-plan.md` | Sequenced sprint evidence (planned) | Not started |
| PDF-4.1-04 | Maintainable Git workflow with merge requests and peer review | `.gitlab-ci.yml`, `CONTRIBUTING.md`, `.gitlab/merge_request_templates` | Repository policy inspection (planned) | Not started |
| PDF-4.1-05 | Sprint review and retrospective practice | `docs/planning/sprint-review-template.md`, `docs/planning/retrospective-template.md` | Inspectable templates (planned) | Not started |
| PDF-4.1-06 | Load/stress tests simulating heavy contention on the same seats | `tests/load/reservation-contention.js` | Executed k6 report (planned) | Not started |
| PDF-4.1-07 | Mutation testing of core reservation logic | `Makefile`, `.gitlab-ci.yml` | Executed mutation command/report (planned) | Not started |
| PDF-4.1-08 | Explicit transactional-module coverage metrics | `Makefile`, `.gitlab-ci.yml` | Coverage profile and enforced threshold (planned) | Not started |
| PDF-4.2-01 | Structured Jira epic/task hierarchy | `docs/planning/jira-import.csv` | Jira-importable CSV validation (planned) | Not started |
| PDF-4.2-02 | Precise burndown data/chart for velocity | `docs/planning/burndown.csv`, `docs/planning/burndown.md` | Chart generated from committed data (planned) | Not started |
| PDF-4.2-03 | GitLab CI/CD runs unit tests on every commit | `.gitlab-ci.yml` | Pipeline definition currently includes security templates only | Not started |
| PDF-5.1-01 | Justified DDD boundaries with availability ownership and no circular coupling | `docs/adr/0001-service-boundaries.md` | ADR dependency direction and ownership rationale | Complete |
| PDF-5.2-01 | Broker decouples transaction outcomes from notification triggers | `docs/adr/0003-message-broker.md`, `internal/messaging`, `internal/notification` | Broker/outbox integration tests (planned) | Partial |
| PDF-5.3-01 | Architecture mapping for Kubernetes autoscaling, canary rollout, Prometheus/Grafana | `docs/diagrams/deployment.puml`, `docs/observability.md`, `deployments/k8s` | Diagram/design inspection (planned) | Not started |
| PDF-5.4-01 | Incident severity, response, escalation, and on-call framework | `docs/operations/incident-response.md`, `docs/operations/on-call.md` | Operations tabletop checklist (planned) | Not started |
| PDF-5.4-02 | Example on-call rotation and threshold-based alert ownership | `docs/operations/on-call.md` | Inspectable rotation and handoff policy (planned) | Not started |
| PDF-5.5-01 | Blameless postmortem: payment outage and lock compensation | `docs/operations/postmortems/payment-provider-outage.md` | Required-section checklist (planned) | Not started |
| PDF-5.5-02 | Blameless postmortem: queue worker failure and HTTP 503 | `docs/operations/postmortems/queue-worker-failure.md` | Required-section checklist (planned) | Not started |
| PDF-5.5-03 | Blameless postmortem: inter-service network partition | `docs/operations/postmortems/network-partition.md` | Required-section checklist (planned) | Not started |
| PDF-6-01 | Consider backend, analysis, QA, and operations perspectives | `docs/planning/product-backlog.md`, `docs/risk-analysis.md`, `docs/operations` | Cross-role artifact review (planned) | Not started |
| PDF-6-02 | Architecture is pragmatic and translatable to production infrastructure | `docs/adr`, `deployments/k8s`, `infra/terraform` | ADR/IaC consistency review (planned) | Partial |
| PDF-6-03 | Portable services, database scripts, and containers with reliable local startup | `docker-compose.yml`, `Makefile`, `README.md` | Fresh-start smoke test (planned) | Not started |
| PDF-6-04 | Modern standards across code, schemas, APIs, docs, and diagrams | Repository-wide | Format, lint, tests, schema/API validation (planned) | Not started |
| PDF-6-05 | Continuous clear documentation for schemas, tests, and code patterns | `README.md`, `docs`, package docs | Documentation audit (planned) | Partial |
| PDF-6-06 | Extensible integration points for providers, hooks, and mobile clients | Port interfaces in domain applications; `api/openapi.yaml` | Provider contract tests (planned) | Not started |
| PDF-6-07 | Team attendance at live presentation and architecture defense | Course-team responsibility outside repository | Requires human attendance; repository will supply defense notes (planned) | Not applicable |
| PDF-7.2-01 | Consolidated single ZIP submission | `Makefile` (`package` target planned) | Reproducible archive contents check (planned) | Not started |
| PDF-7.2-02 | Submission contains baseline docs, UML, Jira export, repository, and setup scripts | Repository deliverables mapped above | Final traceability audit and archive manifest (planned) | Not started |

## User-specified production acceptance overlay

The implementation request adds stricter acceptance criteria than the PDF. These are
tracked here so a PDF-level completion claim cannot hide a missing production behaviour.

| Requirement ID | Requirement | Implementation / artifact path | Test / evidence | Status |
|---|---|---|---|---|
| USR-SEC-01 | Argon2id passwords, expiring JWT, three roles, gateway and application authorization, sensitive-data redaction | `internal/identity`, `internal/platform` | Unit/API/security tests (planned) | Not started |
| USR-CAT-01 | Venue/event constraints, migrations, indexes, deterministic pagination, availability freshness, no N+1 | `internal/catalog`, `migrations` | Domain/repository/contract tests and query plans (planned) | Not started |
| USR-RES-01 | Atomic all-or-nothing Redis Lua locks with owner, TTL, idempotency, stale-client fencing, expiry | `internal/reservation` | Real Redis integration/contention/race tests (planned) | Not started |
| USR-RES-02 | PostgreSQL is durable booking truth with unique seat/schedule constraint and row locking | `internal/reservation`, `migrations` | Real PostgreSQL contention test (planned) | Not started |
| USR-PAY-01 | Deterministic provider, idempotent saga, compensation, signed duplicate/out-of-order callbacks | `internal/checkout` | State-machine and failure-path tests (planned) | Not started |
| USR-MSG-01 | Transactional outbox/inbox, duplicate tolerance, retry, and dead letters | `internal/messaging`, `migrations` | Broker/outbox/inbox integration tests (planned) | Not started |
| USR-TKT-01 | Unpredictable unique ticket, non-sensitive signed QR, verification, revocation, no duplicates | `internal/ticket` | Domain/repository/API tests (planned) | Not started |
| USR-WRM-01 | Bound expiring queue tokens, fair admission, rate cap, anti-forgery/replay, degraded mode | `internal/waitingroom` | Redis/token/load/failure tests (planned) | Not started |
| USR-RT-01 | Async provider interfaces plus SSE status updates | `internal/notification`, `internal/realtime` | Retry/DLQ and stream tests (planned) | Not started |
| USR-API-01 | Uniform errors, validation, IDs, cancellation, shutdown, probes, rate limits, and retry semantics | `internal/platform/httpx`, `cmd` | API middleware and shutdown tests (planned) | Not started |
| USR-OPS-01 | Compose, Terraform, Kubernetes, OpenAPI, metrics/logs, CI, incident and threat-model artifacts | Root files, `api`, `infra`, `deployments`, `docs` | Tool validation and documentation audit (planned) | Not started |
| USR-E2E-01 | Complete buyer workflow from registration through notification | `tests/e2e` | Executed end-to-end purchase test (planned) | Not started |

## Audit procedure

Before changing any row to **Complete**:

1. confirm the path exists and contains non-placeholder work;
2. execute the cited automated evidence where code or configuration is involved;
3. record the exact command and result in `docs/test-evidence.md`;
4. inspect for invalid completion shortcuts (`TODO`, `FIXME`, `panic("not implemented")`,
   empty handlers, unconditional success); and
5. update this matrix in the same change.
