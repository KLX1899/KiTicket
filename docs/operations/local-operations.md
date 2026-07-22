# Local operations

## Start and stop

From the repository root, start the complete local stack with:

~~~
docker compose up --build --wait
~~~

The stack contains PostgreSQL 17, Redis 7, RabbitMQ 3, the NestJS API, the React frontend and a one-shot seed container. The API waits for the database, Redis, RabbitMQ and the seed job before becoming healthy.

Use docker compose ps to inspect service health and docker compose down to stop the stack. To discard only this stack's local volumes and recreate the demo data, use:

~~~
docker compose down -v --remove-orphans
docker compose up --build --wait
~~~

## Local endpoints

| Endpoint | Purpose |
|---|---|
| http://localhost:5173 | frontend |
| http://localhost:3000/api/docs | generated Swagger UI |
| http://localhost:3000/api/health/live | process liveness |
| http://localhost:3000/api/health/ready | database and Redis readiness; RabbitMQ may be reported as degraded |
| http://localhost:3000/api/metrics | process-local Prometheus text format counters |
| http://localhost:15672 | RabbitMQ management UI |

The host ports for PostgreSQL, Redis and RabbitMQ can be changed with POSTGRES_HOST_PORT, REDIS_HOST_PORT, RABBITMQ_HOST_PORT and RABBITMQ_MANAGEMENT_PORT.

## First checks when the stack is unhealthy

1. Run docker compose ps and inspect the unhealthy service with docker compose logs <service>.
2. For API readiness failures, confirm PostgreSQL and Redis are healthy first. RabbitMQ is reported in the readiness payload but does not make the endpoint fail when it is unavailable.
3. If the seed job failed, inspect docker compose logs seed; the API depends on its successful completion.
4. Do not use the compose JWT secret outside local development. Production startup rejects a missing or shorter-than-32-character JWT_SECRET and rejects DB_SYNCHRONIZE=true.

