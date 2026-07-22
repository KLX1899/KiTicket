# Production readiness

infra/k8s/ contains Kubernetes manifests for the frontend, API, PostgreSQL, Redis and RabbitMQ. infra/terraform/ contains a separate AWS-oriented starting configuration. They are useful deployment artifacts, but this repository does not contain a validated production deployment pipeline.

## What is present

- Backend and frontend deployments include probes; the backend rolling-update policy uses maxUnavailable: 0.
- The backend manifest defines non-root execution, a read-only root filesystem and resource requests and limits.
- An HPA, PodDisruptionBudgets, an ingress and example secrets/configuration are supplied.

## Work required before deployment

1. Replace the placeholder image names and example credentials; store every secret in an external secret manager or an approved Kubernetes secret workflow.
2. Run migrations through a controlled deployment step and set DB_SYNCHRONIZE=false.
3. Validate the manifests against the target cluster and make PostgreSQL, Redis and RabbitMQ suitable for the desired durability and availability target.
4. Add CI that builds immutable images, runs the checks in [verification](../quality/verification.md), scans dependencies and applies the manifests only after review.
5. Add persistent metrics, logs, alert rules, backup/restore tests, network policy and a real incident response process.
6. Add a transactional outbox and payment-provider integration before relying on RabbitMQ notifications or payment completion in a production workflow.

No uptime, autoscaling, canary, monitoring or disaster-recovery claim should be inferred from the presence of these starter manifests alone.

