# Admin managed-table API

Admin exposes only tables registered by a server-side Table Policy. The first registered resource is `configs`; clients use public field names such as `key` and never submit the physical table name, physical column name, or SQL.

> The first iteration has no authentication. Keep Admin on a trusted local network until authentication and authorization are implemented.

## Start locally

```bash
docker compose up --build
```

Admin listens on `http://localhost:8080`. MySQL is initialized once from `deploy/mysql/schema.sql` when its data volume is empty.

## Discover policies

```http
GET /api/v1/tables
GET /api/v1/tables/configs
```

Definitions tell Web which fields, filter operators, sorts, and mutations it may render. Physical database names are intentionally omitted.

## Create a row

```http
POST /api/v1/tables/configs/rows
Content-Type: application/json

{
  "values": {
    "namespace": "default",
    "key": "checkout.timeout",
    "value": {"seconds": 10},
    "status": "draft"
  }
}
```

The response contains `affected_rows` and the generated primary key.
Primary keys and other 64-bit integer fields are encoded as decimal strings so Web does not lose precision in JavaScript.

## Query a page

```http
POST /api/v1/tables/configs/query
Content-Type: application/json

{
  "filter": {
    "logic": "and",
    "items": [
      {"field": "namespace", "operator": "eq", "value": "default"},
      {"field": "key", "operator": "contains", "value": "checkout"}
    ]
  },
  "sort": [
    {"field": "updated_at", "direction": "desc"}
  ],
  "page": {"number": 1, "size": 20}
}
```

Response:

```json
{
  "data": [],
  "page": {"number": 1, "size": 20, "total": 0}
}
```

Supported operators are policy-specific. The first iteration implements `eq`, `ne`, `in`, `contains`, `gt`, `gte`, `lt`, `lte`, and `is_null`.

## Update and delete

```http
PATCH /api/v1/tables/configs/rows/1
Content-Type: application/json

{"values": {"status": "published"}}
```

```http
DELETE /api/v1/tables/configs/rows/1
```

Admin rejects undeclared resources, fields, operators, sort expressions, oversized pages, excessive filter depth, and writes not allowed by the policy.

## Add another managed table

Table Policies are persisted runtime data in the `table_policies` catalog table, seeded by `deploy/mysql/schema.sql` and loaded at startup.

1. Add the table's DDL to `deploy/mysql/schema.sql` while no database has been deployed yet.
2. Insert a policy document row into `table_policies` in the same file; the `configs` seed is the reference example.
3. Define every public-to-physical field mapping and explicitly allow read, filter, sort, create, update, and delete capabilities.
4. Add a MySQL integration scenario before exposing it to Web.

Do not accept table names or column names from HTTP requests. A table becomes externally reachable only through a policy row present in the catalog; what is in the catalog is entirely what the database says. Dedicated catalog management APIs arrive with issue #3.
