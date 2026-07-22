# Verification

Run these checks with Node.js 22 or newer and from a clean dependency installation. They validate TypeScript compilation and the focused unit tests that exist today.

## Backend

~~~
cd backend
npm ci
npm run lint
npm run build
npm test -- --runInBand
~~~

The Jest suite covers DTO validation, exception mapping, Redis-lock behavior, payment transition policy, idempotent payment completion behavior and waiting-room admission validation.

## Frontend

~~~
cd frontend
npm ci
npm run lint
npm run build
~~~

## Local smoke check

~~~
docker compose up --build --wait
docker compose ps
curl --fail http://localhost:3000/api/health/ready
docker compose down
~~~

This verifies that the containers start and the core dependencies become ready. It does not exercise a customer purchase.

## Existing test limitations

- tests/e2e/purchase_test.go uses an older multi-service API and database schema. It skips unless its old environment variables are set and should not be treated as a KiTicket end-to-end test.
- tests/load/k6-booking-test.js requests only the event-list endpoint. It is not a reservation-contention or checkout test.
- No CI workflow is checked in to execute these commands automatically.
- A passing unit suite does not prove PostgreSQL, Redis or RabbitMQ failure handling in a deployed environment.
