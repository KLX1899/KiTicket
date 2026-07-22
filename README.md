# KiTicket

End-to-end event-ticketing project demonstrating waiting-room admission, concurrency-safe
seat reservations, idempotent payments, ticket issuance, QR check-in, and real-time updates.

## Architecture

KiTicket is a modular NestJS API with a React/Vite client. PostgreSQL is the system of
record; Redis coordinates short-lived seat locks and waiting-room ordering; RabbitMQ carries
domain events. The browser communicates with the API over REST and receives seat, payment,
and ticket updates through Socket.IO.

## Quick start

Requires Docker Engine and Docker Compose. From the repository root, this one command
builds and starts the complete application. It also waits for PostgreSQL and imports the
demo users and events automatically:

```bash
docker compose up --build --wait
```

For a fresh presentation database (this removes only KiTicket's local Docker data), run:

```bash
docker compose down -v --remove-orphans && docker compose up --build --wait
```

- UI: http://localhost:5173
- Swagger: http://localhost:3000/api/docs
- Health: http://localhost:3000/api/health/ready
- Metrics: http://localhost:3000/api/metrics
- RabbitMQ: http://localhost:15672 (`guest` / `guest`)

Demo accounts use `Password123!`:

- `customer@KiTicket.local`
- `buyer@KiTicket.local`
- `organizer@KiTicket.local`
- `admin@KiTicket.local`

Check service status with `docker compose ps`; all long-running services should be
`healthy`. To stop them after the presentation, run `docker compose down`.

Local verification (requires Node.js 22+):

```bash
export NODE_OPTIONS=--max-old-space-size=1024
cd backend && npm ci && npm run lint && npm run build && npm test -- --runInBand
cd ../frontend && npm ci && npm run lint && npm run build
```

## Booking flow

Sign in, join an event's waiting room, then select available seats and create a reservation.
Redis acquires the selected seat locks atomically with a configurable TTL, while PostgreSQL
records the reservation in a transaction and prevents conflicting bookings. An admitted
waiting-room token is required when waiting-room protection is enabled.

Payment creation accepts an idempotency key. A successful payment confirms the reservation,
books its seats, generates random ticket tokens, and notifies the user in real time. A failed,
timed-out, cancelled, or expired reservation releases the seats for another customer. Issued
tickets can be retrieved as QR data and checked in through the ticket API.

## Project layout

```text
backend/         NestJS API, seed script, and Jest tests
frontend/        React/Vite user interface
infra/k8s/       Kubernetes manifests
infra/terraform/ AWS infrastructure configuration
docs/            Requirements, ADRs, diagrams, and project evidence
```

## Documentation

See the [presentation guide](PresentationGuide.md),
[requirements traceability](docs/requirements-traceability.md),
[implementation plan](docs/implementation-plan.md),
[test evidence](docs/test-evidence.md),
[architecture decisions](docs/adr/),
[UML architecture models](docs/UML%20Architecture%20Models%20-%20KiTicket.pdf), and
[Terraform deployment guide](infra/terraform/README.md).

For production, disable `DB_SYNCHRONIZE`, provide a unique `JWT_SECRET` of at least 32
characters, and manage every service secret outside source control. The checked-in
`.env.example` contains development-only placeholder values.
