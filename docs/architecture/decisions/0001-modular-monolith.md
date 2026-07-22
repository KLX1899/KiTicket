# ADR 0001: Modular NestJS monolith

**Status:** accepted

## Context

The application is deployed as one NestJS API. Controllers and services are grouped by responsibility, but they share one TypeORM data source and one process.

## Decision

Keep the current deployment model as a modular monolith. Treat Auth, Catalog, Reservations, Payments, Tickets, Waiting Room and Notifications as code-level responsibilities, not independent network services.

## Consequences

- Local startup and database transactions remain simple.
- A controller can use another in-process service, so future extraction requires an explicit API or event contract first.
- Documentation and diagrams must not describe an API gateway, worker fleet or separate service databases as implemented behavior.

