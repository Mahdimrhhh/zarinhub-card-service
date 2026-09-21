# Card Number to IBAN

A small full-stack technical assessment project that converts a validated card number to an IBAN through the ZarinHub API. The browser never calls ZarinHub directly.

## Overview

The application has three layers:

```text
Nuxt/Vue frontend
        |
        | POST /api/v1/card-to-iban
        v
Go HTTP handler
        |
        v
Application service -> ZarinHub client -> ZarinHub API
        |
        v
Audit repository -> SQLite
```

The service validates the card with a Luhn checksum before making an external request. Every request that reaches the service is audited, including validation failures and external failures. External response JSON is sanitized before it is stored.

## Architecture

- `backend/internal/handler`: JSON parsing, HTTP status mapping, CORS, request IDs, and safe error responses.
- `backend/internal/service`: validation-to-external-call business flow and audit orchestration.
- `backend/internal/client`: ZarinHub HTTP contract, bearer authentication, timeout, and response classification.
- `backend/internal/repository`: SQLite implementation behind a repository interface.
- `backend/internal/validation`: independent numeric length and Luhn validation.
- `backend/internal/model`: application request, response, error, and audit types.
- `backend/internal/logging`: structured `slog` JSON logger.
- `frontend`: Nuxt 3, Vue 3, and Tailwind CSS UI.

Interfaces are used at the service boundaries so the HTTP handler, ZarinHub client, and database can be replaced or faked independently. Constructors inject dependencies; there is no mutable package-level application state.

## Directory Structure

```text
.
├── .env.example
├── .gitignore
├── README.md
├── go.mod
├── go.sum
├── backend
│   ├── cmd
│   │   └── server
│   │       └── main.go
│   ├── internal
│   │   ├── client
│   │   ├── handler
│   │   ├── logging
│   │   ├── model
│   │   ├── repository
│   │   ├── service
│   │   └── validation
│   └── migrations
│       ├── 001_create_audit.sql
│       └── migrations.go
└── frontend
    ├── app.vue
    ├── assets/css/main.css
    ├── nuxt.config.ts
    ├── package.json
    └── package-lock.json
```

Generated directories such as `frontend/node_modules`, `frontend/.nuxt`, and `frontend/.output` are ignored by Git.

## Requirements

- Go 1.27.1 or a compatible Go toolchain
- Node.js 20 or newer
- npm
- Network access for the first dependency installation
- A ZarinHub bearer token for real external requests

No real credential is included in the repository.

## Installation

### Backend

From the project root:

```bash
go mod download
```

The SQLite driver is the pure-Go `github.com/ncruces/go-sqlite3/driver`, so the backend does not require a C compiler.

### Frontend

```bash
cd frontend
npm install
```

The lockfile is included for reproducible frontend installation.

## Environment Variables

The backend reads operating-system environment variables directly. It does not load a `.env` file automatically. Copy `.env.example` to a local environment setup and export the values in the shell that starts the process.

| Variable | Default | Purpose |
| --- | --- | --- |
| `ZARINHUB_ACCESS_TOKEN` | required | Bearer token for ZarinHub |
| `ZARINHUB_API_URL` | `https://zarin-hub.com/api/v5/KYC/CardToIban` | External endpoint |
| `ZARINHUB_TIMEOUT` | `5s` | Outgoing HTTP timeout |
| `DATABASE_PATH` | `./data/card-service.db` | SQLite file path |
| `SERVER_ADDR` | `:8080` | Backend listen address |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `CORS_ORIGIN` | `http://localhost:3000` | Allowed browser origin |
| `NUXT_PUBLIC_API_BASE` | `http://localhost:8080` | Frontend backend base URL |

Example PowerShell setup:

```powershell
$env:ZARINHUB_ACCESS_TOKEN = "replace-with-a-secret-value"
$env:ZARINHUB_API_URL = "https://zarin-hub.com/api/v5/KYC/CardToIban"
$env:ZARINHUB_TIMEOUT = "5s"
$env:DATABASE_PATH = ".\data\card-service.db"
$env:SERVER_ADDR = ":8080"
$env:CORS_ORIGIN = "http://localhost:3000"
$env:NUXT_PUBLIC_API_BASE = "http://localhost:8080"
```

Never place a real token in source control or in frontend configuration.

## Run the Backend

From the project root:

```bash
go run ./backend/cmd/server
```

Startup performs these steps:

1. Reads and validates configuration.
2. Creates the database parent directory when needed.
3. Opens SQLite.
4. Applies the embedded SQL migration.
5. Constructs the client, repository, service, handler, and HTTP server.
6. Starts graceful signal handling for `SIGINT` and `SIGTERM`.

The server exposes:

- `GET /health`
- `POST /api/v1/card-to-iban`

## Run the Frontend

In a second terminal:

```bash
cd frontend
npm run dev
```

Open the Nuxt development URL, normally `http://localhost:3000`. The frontend sends requests only to `NUXT_PUBLIC_API_BASE` and does not contain the ZarinHub token or endpoint.

For a production frontend build:

```bash
cd frontend
npm run build
```

## Database and Migrations

The migration is in `backend/migrations/001_create_audit.sql` and is embedded by `backend/migrations/migrations.go`. The server applies it at startup, so a separate migration command is not required for local operation.

The `audit_events` table contains:

| Column | Meaning |
| --- | --- |
| `id` | SQLite primary key |
| `request_id` | Correlation ID |
| `created_at` | UTC timestamp |
| `duration_ms` | Processing duration |
| `client_ip` | Safe remote client address |
| `card_masked` | Masked card, never the full card |
| `status` | Application outcome |
| `external_http_status` | Nullable upstream HTTP status |
| `response_json` | Nullable sanitized upstream JSON |

Indexes support request-ID and time-based lookup.

## Internal API

### Convert Card

`POST /api/v1/card-to-iban`

Request:

```json
{
  "card": "4532015112830366"
}
```

The backend requires a 16-digit numeric card that passes the Luhn checksum. The example above is a valid checksum test value; it is not a real credential.

Success response:

```json
{
  "data": {
    "iban": "IR820540102680028017900902",
    "bank_name": "Example Bank"
  }
}
```

The response also includes `X-Request-ID`.

Consistent error response:

```json
{
  "error": {
    "code": "INVALID_CARD",
    "message": "شماره کارت نامعتبر است"
  }
}
```

Status codes:

| HTTP status | Code | Meaning |
| --- | --- | --- |
| `400` | `INVALID_CARD` | Empty, non-numeric, wrong length, or failed Luhn |
| `400` | `INVALID_REQUEST` | Malformed JSON, unknown fields, or multiple JSON values |
| `502` | `ZARINHUB_AUTHENTICATION` | Upstream authentication failed |
| `502` | `ZARINHUB_REJECTED` | Upstream returned an application-level rejection |
| `504` | `ZARINHUB_TIMEOUT` | Upstream did not respond in time |
| `502` | `ZARINHUB_UNAVAILABLE` | Network, non-2xx, or invalid upstream response |
| `500` | `PERSISTENCE_FAILED` | Audit storage failed |
| `500` | `INTERNAL_SERVER_ERROR` | Unexpected internal failure |

The frontend receives only the clean application response. It never receives raw ZarinHub fields, credentials, or stack traces.

## Request Flow

1. Nuxt validates basic input shape for immediate feedback.
2. Nuxt posts the card to the Go backend.
3. The Go handler creates or safely accepts a request ID, limits the body, and parses strict JSON.
4. The service validates length, numeric content, and Luhn checksum.
5. Only a valid card is sent to the ZarinHub client.
6. The client sends `POST` with `Authorization: Bearer ...` and `Content-Type: application/json`.
7. The client interprets non-2xx responses, malformed JSON, timeouts, network failures, and `meta.isSuccess == false`.
8. The service creates a safe audit record and stores it through the repository interface.
9. The handler returns a clean success or error response with the request ID header.

## Error Handling

Errors are represented by explicit application categories. The handler maps categories to stable HTTP statuses and public messages. Underlying database errors, raw upstream bodies, tokens, and stack traces are not written to the response.

The ZarinHub client uses a dedicated `http.Client` with a configured timeout and propagates `context.Context`. Retry is intentionally omitted; retrying financial or identity-related lookups without an idempotency contract could create ambiguous behavior.

## Security Decisions

- `ZARINHUB_ACCESS_TOKEN` is environment-based and required at startup.
- The token is sent only in the upstream `Authorization` header and is never logged or returned.
- The frontend cannot call ZarinHub directly.
- Card validation happens before the external call.
- Request bodies are limited to 16 KiB.
- JSON decoding rejects unknown fields and extra values.
- Full card numbers are never written to audit records or logs.
- Cards are masked as `4532********0366`.
- External JSON is recursively sanitized before persistence. Keys named `card` or secret-like keys are replaced, and any occurrence of the submitted card in a string is replaced.
- Malformed upstream JSON is stored as a safe placeholder instead of raw bytes.
- Client IP is taken from `RemoteAddr`; proxy headers are not trusted by default.
- Client-supplied request IDs are accepted only when they match a short alphanumeric, underscore, or hyphen pattern. Otherwise a random ID is generated.
- Logs use JSON `slog` fields and contain request ID, operation, duration, status, external status, and error category, but no card or token.
- `.env` and runtime database/log/build artifacts are ignored.

## Audit Strategy

The service records:

- malformed request bodies through the handler;
- validation failures;
- successful external calls;
- upstream authentication, rejection, timeout, network, and malformed-response failures;
- request ID, timestamp, duration, masked card, status, upstream status, and sanitized response JSON.

If the database itself is unavailable, the current request cannot be persisted. The service returns `PERSISTENCE_FAILED` and logs the category. This is an unavoidable limitation of any single local audit store and is documented rather than silently ignored.

## Testing Strategy

Backend tests cover:

- valid, empty, non-numeric, wrong-length, and invalid-checksum cards;
- masking;
- successful service flow with a fake ZarinHub client;
- validation preventing an external call;
- upstream rejection and timeout mapping;
- repository failure mapping;
- sanitized audit data;
- HTTP endpoint behavior, request ID headers, and malformed JSON;
- real client header/body handling, application errors, authentication errors, malformed JSON, and timeout behavior;
- SQLite migration and audit persistence.

Run:

```bash
go test ./backend/...
go vet ./backend/...
go build ./backend/cmd/server
```

Frontend verification:

```bash
cd frontend
npm run build
```

No test uses a real ZarinHub credential.

## Assumptions

The provided API documentation is the source of truth. The implementation makes these explicit assumptions where the documentation is silent:

1. The documented URL and bearer-header contract are used without additional authentication.
2. The default external timeout is 5 seconds.
3. No retry is performed because no idempotency or retry contract is documented.
4. A successful application response requires `meta.isSuccess == true` and a non-empty `data.iban`.
5. The external response is limited to 1 MiB to prevent unbounded memory use.
6. The internal API accepts a 16-digit Luhn-valid card. The documentation's sample card is treated as an illustrative payload, not as a guaranteed checksum-valid test vector.
7. `RemoteAddr` is the trusted client IP source unless a deployment adds a trusted proxy layer.
8. A safe CORS origin defaults to the local Nuxt development origin and is configurable with `CORS_ORIGIN`.
9. The pure-Go ncruces SQLite driver was selected to keep local builds reproducible without a C toolchain.
10. The external response is stored in sanitized form rather than blindly storing raw JSON, because the documented response includes `data.card`.

## Known Limitations

- There is no application authentication, rate limiting, or abuse protection. Add them at the deployment boundary if this becomes a public service.
- SQLite is intended for a simple local assessment and is not a distributed audit system.
- The health endpoint is a safe liveness response; it does not call ZarinHub or expose database details.
- A database outage prevents the current audit row from being stored; the request returns `PERSISTENCE_FAILED`.
- No real ZarinHub integration test is included because no credential is available or committed.
- Docker Compose is intentionally omitted to keep local setup minimal.
- The frontend performs convenience validation only; the Go backend remains authoritative.

## Why These Choices Were Made

### SQLite

SQLite keeps setup reproducible and removes a separate database service from the assessment. The repository interface keeps the service independent of SQLite and leaves a path to another implementation.

### Interfaces

The service depends on small interfaces rather than concrete HTTP or database types. This makes the business flow testable with fakes and keeps the external contract replaceable.

### Dependency Injection

`main.go` wires concrete dependencies once. Constructors make ownership and test setup explicit and avoid mutable global state.

### Timeout Strategy

The ZarinHub client owns an HTTP client with an explicit timeout and uses request contexts. The default is five seconds and can be changed with `ZARINHUB_TIMEOUT`.

### Sensitive Data

The application follows a deny-by-default audit policy: store the response only after recursively removing or masking card and secret-like fields. This preserves useful audit structure without storing the full card number.

## Verification Commands Used

The project was checked with:

```text
go fmt ./backend/...
go test ./backend/...
go vet ./backend/...
go build ./backend/cmd/server
cd frontend && npm install
cd frontend && npm audit --omit=dev
cd frontend && npm run build
```

The generated backend binary and frontend build directories are not part of the source tree.
