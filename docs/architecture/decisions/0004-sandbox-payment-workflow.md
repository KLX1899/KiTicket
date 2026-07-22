# ADR 0004: Idempotent sandbox payment completion

**Status:** accepted for local demonstration

## Context

The project needs reproducible success, failure and timeout paths without storing payment-card data or integrating an external provider.

## Decision

Create one Payment per idempotency key and reservation. A caller completes the payment with a declared sandbox outcome. The API locks the Payment and Reservation rows, leaves a terminal payment unchanged on replay, and performs ticket issuance or reservation compensation in the same transaction.

## Consequences

- The flow demonstrates local state transitions and idempotent completion.
- The endpoint is not a payment gateway contract: it has no signed callback, capture status query, reconciliation or refund path.
- No production payment claim should rely on this workflow until a provider adapter and callback security are implemented.

