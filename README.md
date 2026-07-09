# Search Service

A RESTful API for managing and searching internal services and their versions. Built with Go, Echo, PostgreSQL, and JWT-based authentication.

---

## Table of Contents

- [Starting the application](#starting-the-application)
- [Environment variables](#environment-variables)
- [API reference](#api-reference)
  - [Authentication](#authentication)
  - [Services](#services)
- [Logs](#logs)
- [Running tests](#running-tests)
- [Design decisions](#design-decisions)

---

## Starting the application

### With Docker Compose (recommended)

```bash
docker-compose up --build
```

This starts two containers:

- **postgres** — PostgreSQL 16, exposed on `${DB_PORT:-5432}`
- **api** — the Go service, exposed on `${SERVER_PORT:-3000}`

Database migrations run automatically via the entrypoint before the server starts. The postgres container must pass its health check before the API container starts.

### Locally (requires a running postgres)

```bash
# 1. Export the environment variables (or use a tool like direnv with .envrc)
export DB_HOST=127.0.0.1
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=secret
export DB_NAME=search_service
export JWT_SECRET=your-secret-here

# 2. Apply migrations
migrate -path migrations \
        -database "postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable" \
        up

# 3. Run
go run ./cmd/search
```

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `DB_HOST` | `127.0.0.1` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_NAME` | `search_service` | Database name |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | `secret` | Database password |
| `SERVER_PORT` | `3000` | Port the API listens on |
| `LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_DIR` | `/var/log/search_service/` | Log file directory |
| `LOG_FILE` | `api.log` | Log file name |
| `JWT_SECRET` | `secret` | Secret used to sign and verify JWT tokens |

All variables can also be overridden via CLI flags of the same name with `--` prefix and `-` instead of `_` (e.g. `--db-host=localhost`). CLI flags take precedence over environment variables.

---

## API reference

All endpoints that require authentication expect a JWT in the `Authorization` header:

```
Authorization: Bearer <token>
```

The token is issued by the `POST /login` endpoint. Alternatively, the `session` cookie set by that same endpoint is accepted (e.g. from a browser).

Base URL: `http://localhost:3000`

---

### Authentication

#### Sign up

```
POST /signup
```

**Request body**

```json
{
  "email_id": "user@example.com",
  "user_name": "alice",
  "password": "mysecret"
}
```

- `email_id` — unique
- `user_name` — required
- `password` — minimum 6 characters

**Response `201 Created`**

```json
{
  "data": {
    "id": 1,
    "email_id": "user@example.com",
    "user_name": "alice",
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

**Validation errors `400 Bad Request`**

When one or more fields fail validation the response includes a structured `errors` array so clients can surface per-field messages:

```json
{
  "message": "validation failed",
  "errors": [
    { "field": "email_id", "message": "is required" },
    { "field": "password", "message": "must be at least 6 characters" }
  ]
}
```

---

#### Log in

```
POST /login
```

**Request body**

```json
{
  "email_id": "user@example.com",
  "password": "mysecret"
}
```

**Response `200 OK`**

Sets an `HttpOnly` session cookie containing the JWT. The token expires after 24 hours.

```json
{
  "data": {
    "id": 1,
    "email_id": "user@example.com",
    "user_name": "alice",
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

---

#### Log out

```
POST /logout
```

Clears the session cookie. No request body required.

**Response `200 OK`**

```json
{ "message": "logged out" }
```

---

### Services

All service endpoints require authentication.

#### Create a service

```
POST /services
```

**Request body**

```json
{
  "name": "Payment Gateway",
  "description": "Handles all payment processing",
  "version": {
    "version_string": "v1.0.0",
    "status": "stable"
  }
}
```

- `name` — 3–30 characters, unique
- `description` — free text, optional
- `version.version_string` — required
- `version.status` — required (e.g. `stable`, `beta`, `deprecated`)

**Response `201 Created`**

```json
{
  "data": {
    "id": 1,
    "name": "Payment Gateway",
    "description": "Handles all payment processing",
    "number_of_versions": 1,
    "created_by": 1,
    "updated_by": 1,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

---

#### Search services

```
GET /services
```

**Query parameters** — all optional

| Parameter | Type | Description |
|---|---|---|
| `name` | string | Case-insensitive partial match on service name (trigram index) |
| `description` | string | Full-text search on service description (tsvector index) |
| `page` | int | Page number (default: `1`) |
| `page_size` | int | Results per page, clamped to 10–20 (default: `10`) |
| `sort_by` | string | `name` or `created_at` (default: `created_at`) |
| `order` | string | `asc` or `desc` (default: `asc`) |

**Example**

```
GET /services?name=payment&sort_by=name&order=asc&page=1&page_size=10
```

**Response `200 OK`**

The response includes a `meta` object with pagination details alongside the `data` array. `total` is the number of services that match the query (before pagination); `total_pages` is the total number of pages at the requested `page_size`.

```json
{
  "data": [
    {
      "id": 1,
      "name": "Payment Gateway",
      "description": "Handles all payment processing",
      "number_of_versions": 2,
      "created_by": 1,
      "updated_by": 1,
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 10,
    "total": 42,
    "total_pages": 5
  }
}
```

When no services match the query `data` is an empty array (never `null`) and `total` is `0`.

---

#### Get a service

```
GET /services/:id
```

**Response `200 OK`** — returns a single service object wrapped in `data`:

```json
{
  "data": {
    "id": 1,
    "name": "Payment Gateway",
    "description": "Handles all payment processing",
    "number_of_versions": 2,
    "created_by": 1,
    "updated_by": 1,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

---

#### Update a service

```
PUT /services/:id
```

**Request body**

```json
{
  "name": "Payment Gateway v2",
  "description": "Updated description"
}
```

- `name` — minimum 3 characters, required

**Response `200 OK`** — returns the updated service object.

```json
{
  "data": {
    "id": 1,
    "name": "Payment Gateway v2",
    "description": "Updated description",
    "number_of_versions": 2,
    "created_by": 1,
    "updated_by": 2,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-16T08:00:00Z"
  }
}
```

---

#### Delete a service

```
DELETE /services/:id
```

Deletes the service and all its versions (cascade).

**Response `200 OK`**

---

#### Add a version to a service

```
POST /services/:id/versions
```

**Request body**

```json
{
  "version_string": "v2.0.0",
  "status": "beta"
}
```

- `version_string` — required
- `status` — required (e.g. `stable`, `beta`, `deprecated`)

**Response `201 Created`**

```json
{
  "data": {
    "id": 3,
    "service_id": 1,
    "version_string": "v2.0.0",
    "status": "beta",
    "created_by": 1,
    "created_at": "2024-01-16T09:00:00Z"
  }
}
```

---

#### List versions for a service

```
GET /services/:id/versions
```

Returns all versions ordered by creation time, newest first.

**Response `200 OK`**

```json
{
  "data": [
    {
      "id": 3,
      "version_string": "v2.0.0",
      "status": "beta",
      "created_by": 1,
      "created_at": "2024-01-16T09:00:00Z"
    },
    {
      "id": 1,
      "version_string": "v1.0.0",
      "status": "stable",
      "created_by": 1,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

## Logs

Logs are written in JSON format using [zap](https://github.com/uber-go/zap).

**Docker** — logs go to stdout by default; view them with:

```bash
docker-compose logs -f api
```

**Local / file logging** — set both `LOG_DIR` and `LOG_FILE` to write to a file. Logs are rotated automatically (max 100 MB, 5 backups, 30-day retention, gzip compressed). When `LOG_DIR` is empty the service logs to stdout only.

---

## Running tests

### Unit tests

```bash
go test ./...
# or via Make
make test
```

### Integration tests (requires a running postgres)

Integration tests cover the full PostgreSQL query layer. Point `TEST_DATABASE_URL` at a database that already has the schema applied:

```bash
TEST_DATABASE_URL="postgresql://postgres:secret@localhost:5432/search_service?sslmode=disable" \
    go test ./internal/datastores/postgres/... -v -run Integration

# or via Make (uses Makefile variable defaults)
make test-integration
```

---

## Design decisions

### Why not Elasticsearch?

Introducing Elasticsearch (ES) prematurely creates significant architectural drag:

**1. Distributed state problem**

ES cannot act as a primary source of truth for relational application state — it requires a primary database (Postgres) alongside it. This forces dual-writes and complex synchronisation patterns: every write must succeed in both stores, failures must be handled consistently, and replication lag means search results can be stale. This is a solved problem in large systems, but an expensive one to maintain correctly.

**2. Operational and infrastructure cost**

Elasticsearch is resource-intensive and complex to operate: cluster sizing, shard management, index lifecycle policies, monitoring, and upgrades all require dedicated expertise. For a service at this stage, that overhead is not justified.

---

### Why PostgreSQL for search?

By using PostgreSQL natively for both storage and search, the architecture achieves high reliability and low latency through a single data store, with no synchronisation layer required.

**Fuzzy string matching via `pg_trgm`**

Standard B-Tree indexes fail on partial matches (e.g. `WHERE name LIKE '%auth%'`). The `pg_trgm` extension adds a GIN index over trigrams — 3-character substrings — which enables fast, index-backed substring and fuzzy search without a full table scan. This is how service name search is implemented.

**Full-text search via `tsvector`**

For description search, PostgreSQL converts text into a `tsvector`: it tokenises words, discards common stop words (`"and"`, `"the"`, etc.), and applies linguistic stemming so that `"running"`, `"runs"`, and `"ran"` all resolve to the root lexeme `"run"`. The stored `tsvector` column is covered by a GIN index, making full-text queries fast and index-backed. This gives Elasticsearch-style relevance search at a fraction of the operational cost.

**Transactional integrity for 1-to-many relationships**

Relational requirements — a service must always have at least one version — are enforced via ACID transactions (`db.BeginTx`). A service can never enter an inconsistent state where it exists without an initial version. This guarantee is straightforward in PostgreSQL and would require extra coordination logic if a separate search store were involved.

**Summary**

Using one data store reduces operational cost, eliminates a synchronisation problem, and means engineers only need to be proficient in one technology. Queries are faster and the codebase stays simpler.

---

### Why Echo?

1. **Highly optimised router** — near-zero memory allocations per request; predictable routing with no ambiguity.
2. **Reduced boilerplate** — helpers like `c.Bind()` handle query parameter and body extraction cleanly.
3. **Clean middleware chain** — easy to compose and configure; ordering is explicit.
4. **Rich built-in middleware** — JWT authentication, rate limiting, request IDs, logging, CORS, and more are available out of the box and configurable without external dependencies.
5. **Production-proven** — Echo has been used in production at scale and is reliable under load.

---

### Why Zap for logging?

1. **Performance** — avoids reflection and uses strongly-typed fields (`zap.String()`, `zap.Int()`, etc.), which reduces allocations and garbage collection pressure significantly compared to `fmt`-based loggers.
2. **Structured JSON output** — logs are emitted as JSON by default, making them directly ingestible by log aggregation and analytics systems (Datadog, Loki, ELK, etc.).
3. **Extensibility** — highly configurable: log levels, sampling, output destinations, and log rotation (via [lumberjack](https://github.com/natefinch/lumberjack)) are all supported.
---

### Rate limiting note

A simple in-memory rate limiter (20 req/s, burst 100) is included. **This is not suitable for production use as-is:** the limit resets on restart and does not apply across multiple replicas. For a production deployment, replace it with a centralised rate limiter backed by Redis, or enforce limits at the API gateway layer (Kong Gateway handles this natively).
