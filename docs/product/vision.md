# Product vision

KiTicket is an event-ticketing application for customers, organizers and administrators. Its core purpose is to let a customer discover a published event, obtain admission when the waiting room is enabled, reserve specific seats, complete a sandbox payment and receive QR-verifiable tickets.

## Current product scope

- Customers can register, sign in, browse and filter upcoming published events, join a waiting room, reserve one to ten seats, complete or cancel a payment, and view issued tickets.
- Organizers and administrators can create venues and sector/seat layouts, create and publish events, configure prices, view event analytics, and check tickets in.
- Administrators can admit queued users and change user roles.
- The UI receives seat, payment and ticket updates through Socket.IO; REST endpoints remain the source of truth for a refresh or reconnect.

## Product guarantees implemented in this repository

- A seat cannot be present in more than one ReservationSeat record for the same event because the database has a unique (eventId, seatId) constraint.
- Redis acquires all requested seat locks together or acquires none; locks have a configurable TTL and can be released only by the reservation that owns them.
- Payment completion holds database write locks, treats a completed payment as immutable, and creates tickets in the same database transaction as the successful reservation transition.
- QR codes carry an opaque ticket token. The database stores only its SHA-256 lookup hash.

## Boundary of the current release

Payment completion is a sandbox command supplied with an outcome by an authenticated user; it is not an integration with a bank or a payment-provider callback. Notifications are persisted after RabbitMQ messages are consumed, but no email or SMS is sent. The included Kubernetes and Terraform files are deployment starting points, not evidence of a production deployment or service-level objective.

