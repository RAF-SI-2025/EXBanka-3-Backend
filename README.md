# EXBanka 3 Backend

Go backend for the EXBanka 3 project. The active runtime is a set of HTTP-first
microservices behind nginx:

- `auth-service`
- `employee-service`
- `client-service`
- `account-service`
- `transfer-service`
- `payment-service`
- `exchange-service`
- `loan-service`

## Runtime shape

The shared backend in [cmd/server/main.go](/C:/Dev/Projects/SI/EXBanka-3-Backend/cmd/server/main.go)
is no longer the business runtime. It now serves only:

- `GET /health`
- `GET /swagger.json`
- `GET /swagger-ui`

All business API calls are exposed through the root nginx gateway in
[C:\Dev\Projects\SI\EXBanka-3-Frontend\nginx.conf](/C:/Dev/Projects/SI/EXBanka-3-Frontend/nginx.conf).

## Start the stack

From the project root:

```powershell
docker compose up -d --build
```

The main public entrypoint is:

- frontend + gateway: `http://localhost`

Common service ports exposed for debugging:

- shared backend docs/health: `8080`
- account-service: `8084`
- transfer-service: `8086`
- payment-service: `8087`
- exchange-service: `8088`
- loan-service: `8089`
- Mailhog UI: `8025`

## Gateway route surface

The nginx gateway proxies these prefixes:

- `/api/v1/auth`
- `/api/v1/permissions`
- `/api/v1/employees`
- `/api/v1/clients`
- `/api/v1/firme`
- `/api/v1/sifre-delatnosti`
- `/api/v1/currencies`
- `/api/v1/accounts`
- `/api/v1/cards`
- `/api/v1/transfers`
- `/api/v1/payments`
- `/api/v1/recipients`
- `/api/v1/exchange`
- `/api/v1/loans`

## Notifications

For the current submission scope, notification delivery remains per-service.
Auth, account, transfer, and payment flows send their own email notifications
through Mailhog/SMTP instead of a dedicated standalone notification service.

## Tests

Service-level tests live inside each microservice.

Gateway-level integration tests live under
[tests/integration](/C:/Dev/Projects/SI/EXBanka-3-Backend/tests/integration)
and are intended to run against the nginx-served stack:

```powershell
cd C:\Dev\Projects\SI\EXBanka-3-Backend
go test -tags=integration ./tests/integration
```
