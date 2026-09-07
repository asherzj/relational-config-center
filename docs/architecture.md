# Architecture

## Service boundaries

```text
Web (browser frontend)
        │ HTTP/JSON
        ▼
Admin (management backend, Gin)
        │ read/write
        ▼
MySQL 8.4 LTS
        ▲
        │ read-only
Server (runtime backend, grpc-go)
        ▲
        │ gRPC/Protobuf
Runtime clients and SDKs
```

- `web` is only the frontend. `admin` is a backend service, not the frontend bundle.
- Admin owns management reads and every configuration write. Server serves runtime reads and will use a read-only MySQL account.
- The first deployment has no separately developed application Gateway. An ingress or reverse proxy may still terminate TLS and route traffic.
- Admin and Server remain independent processes even though a Go process can technically host HTTP and gRPC servers together.

## Admin data access

```text
HTTP request
    ↓
Query Specification / mutation values
    ↓
Table Policy validation and complexity limits
    ↓
MySQL Query Compiler
    ↓
GORM Scopes / Clauses
    ↓
database/sql
    ↓
go-sql-driver/mysql
    ↓
MySQL
```

The project implements only domain-specific policy and query semantics. GORM owns SQL construction, binding, execution, scanning, and transactions. Business and HTTP code depend on repository interfaces rather than `*gorm.DB`.

Current DAL choices:

| Concern | Choice |
|---|---|
| Primary database | MySQL 8.0+, default image MySQL 8.4 LTS |
| ORM/DAL base | GORM |
| MySQL dialect | `gorm.io/driver/mysql` |
| Driver | `go-sql-driver/mysql` |
| Connection pool | `database/sql` |
| Dynamic queries | Query Specification → policy validation → GORM Clauses |
| Static and exceptional SQL | GORM repository methods and parameterized `Raw` only when needed |
| Initial schema | `deploy/mysql/init/001-schema.sql` |
| Integration tests | Testcontainers with a real MySQL 8.4 container |
| PostgreSQL | A later independent adapter and query compiler |

Not selected for the first iteration:

- sqlc, because GORM is the single primary DAL;
- Goose, because there are no existing installations to upgrade yet;
- GORM AutoMigrate, because the shipped schema remains explicit and reviewable;
- Kitex/Hertz, because RPC is standardized on grpc-go and Admin HTTP uses Gin;
- a separately implemented Gateway, until multiple backend APIs require routing or aggregation;
- Redis and message queues, because no near-term iteration has caching or async fan-out requirements that justify another stateful system.

## Admin structure

Admin's mandatory package layout; Server and Client repeat the same shape with their own domain models and transports:

```text
internal/interfaces/http       Gin handlers, HTTP DTOs
        ↓
internal/application           query, write, and policy-management use cases
        ↓
internal/domain                table policies, Query Specifications, validation, repository interfaces
        ↑
internal/infrastructure/mysql  GORM repository, MySQL query compiler
```

Dependencies point `interfaces → application → domain` and `infrastructure → domain` only (ADR 0003). Services never share Go domain packages; the only cross-service artifacts are Protobuf contracts in `shared` (ADR 0004), and grpc-go enters the workspace only when the Server iteration starts.

## Admin first iteration

The first iteration ships the Policy Catalog as runtime data (ADR 0005) and proves the generic seam through it:

- The catalog table ships in `deploy/mysql/init/001-schema.sql`; dedicated APIs manage it, and each data request loads its current Policy Snapshot from the database.
- A saved policy takes effect immediately; draft/activation workflows wait for multi-instance or audit needs.
- Saving a policy validates its physical table and columns against `information_schema` of the single deployment-configured datasource. Policies cannot store DSNs or reach other databases.
- Web can discover database tables and their policy state, submit policy-limited AND filters, sorting, and one-based pagination, and create, update, and delete rows using live column names.
- Requests cannot select arbitrary physical tables, columns, operators, sort expressions, or SQL.
- Query depth, node count, `IN` size, page size, string length, enum values, and JSON types are validated before GORM executes anything.

The original prototype packages (`httpapi`, `managedtable`, `mysqlstore`) were rewritten into the layout above rather than preserved; the compile-time `bootstrap` registry was deleted when the runtime Policy Catalog landed (issue #2).

Relations, end-user identity and authorization, audit history, publishing workflows, runtime gRPC reads, and in-place schema upgrades belong to later iterations.

## Local Accounts and business request identity

The account entry uses `interfaces/http/authentication.go` →
`application/Authentication` → Domain-owned account and rate-state contracts.
The MySQL adapter commits account creation and its initial Login Session in one
transaction. The password adapter owns Argon2id computation and its concurrency
bound. HTTP maps the current account into a safe identity response; the Web
account page consumes that response through its account API client.

Account IDs, normalized unique account fields, opaque session digests, pre-login
CSRF digests and rate windows live in protected `rcc_` control tables. Authenticated
reads check enabled status and both stored security versions in the same query;
session issuance rechecks the verified account under a database lock. Password
computation happens outside database transactions. Control-table admission uses
one MySQL lock row so capacity and rate decisions work across Admin processes.

Every business route now requires the account session and every non-GET/HEAD
request requires same-origin CSRF. Authentication produces an immutable
`AuthenticatedOperator` with a private Account ID, bound to one request context;
shared business services never store account state. Query/Mutation/Table Policy
writes and configuration Auto Fill read the same request identity. Revocation
blocks new authentication while allowing already-authenticated writes to finish.
Live metadata supplies unrestricted text capacity for 36-character Account IDs;
short columns and ENUM reject affected writes without changing historical text.

HTTP consumes Application contracts only, checked by
`TestHTTPDependsOnApplicationRatherThanDomainOrInfrastructure`. A second source
check rejects actor/account fields on shared business service structs. Concurrent
HTTP/MySQL tests verify actual attribution and revocation outcomes. The full
repository does not yet have a general layer dependency graph checker.

TMP-01 is removed. `OpenMaintenance` and `LoadMySQL` initialize maintenance
connections independently from normal Admin HTTP and required-schema readiness.
#38 continues draft recovery, #39 account maintenance commands, and #40 final
startup/readiness and release acceptance.
