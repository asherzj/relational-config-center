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
| Initial schema | `deploy/mysql/schema.sql` |
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

- The catalog table ships in `deploy/mysql/schema.sql`; dedicated APIs manage it, and the registry loads policies from the database instead of Go code.
- A saved policy takes effect immediately; draft/activation workflows wait for multi-instance or audit needs.
- Saving a policy validates its physical table and columns against `information_schema` of the single deployment-configured datasource. Policies cannot store DSNs or reach other databases.
- Web can discover the public table policy, submit policy-limited AND/OR filters, sorting, and one-based pagination, and create, update, and delete rows using public field names.
- Requests cannot select arbitrary physical tables, columns, operators, sort expressions, or SQL.
- Query depth, node count, `IN` size, page size, string length, enum values, and JSON types are validated before GORM executes anything.

The original prototype packages (`httpapi`, `managedtable`, `mysqlstore`) were rewritten into the layout above rather than preserved; the compile-time `bootstrap` registry was deleted when the runtime Policy Catalog landed (issue #2).

Relations, authentication, authorization, audit history, publishing workflows, runtime gRPC reads, and in-place schema upgrades belong to later iterations.
