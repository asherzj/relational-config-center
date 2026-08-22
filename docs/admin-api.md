# Admin managed-table API

Admin exposes only tables with an active Table Policy. Policies are persisted runtime data in the `table_policies` catalog table; the dedicated Policy Catalog API manages them, and a saved policy is active immediately — no rebuild, no restart.

> The first iteration has no authentication. Keep Admin on a trusted local network until authentication and authorization are implemented.

## Start locally

```bash
docker compose up --build
```

Admin listens on `http://localhost:8080`. MySQL is initialized once from `deploy/mysql/schema.sql` when its data volume is empty: the `configs` table plus its seeded policy, the `table_policies` catalog, and a policy-less `feature_flags` table for trying the catalog API.

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

## Manage the Policy Catalog

The catalog is managed only through these dedicated endpoints. It is never reachable through the generic table API, and a policy can never target the `table_policies` table itself.

```http
GET /api/v1/policies
GET /api/v1/policies/configs
PUT /api/v1/policies/feature_flags
DELETE /api/v1/policies/feature_flags
```

`PUT` saves one policy document (create or replace). The document is the persisted policy format:

```http
PUT /api/v1/policies/feature_flags
Content-Type: application/json

{
  "resource": "feature_flags",
  "table": "feature_flags",
  "primary_key": "id",
  "fields": {
    "id": {"column": "id", "type": "unsigned_integer", "readable": true, "sortable": true, "auto_increment": true},
    "name": {"column": "name", "type": "string", "readable": true, "creatable": true, "updatable": true, "sortable": true, "required_on_create": true, "min_length": 1, "max_length": 128},
    "enabled": {"column": "enabled", "type": "boolean", "readable": true, "creatable": true, "updatable": true},
    "rollout_percent": {"column": "rollout_percent", "type": "unsigned_integer", "readable": true, "creatable": true, "updatable": true}
  },
  "allow_create": true,
  "allow_update": true,
  "allow_delete": true
}
```

Rules enforced before anything is persisted:

- `resource` in the body must match the URL.
- The document must validate: safe identifiers, a declared readable and sortable primary key, consistent per-field capabilities, and coherent query limits. Omitted limits default to safe values; the saved canonical document is returned.
- The physical table and every mapped column must exist in the deployment's single configured database, checked against `information_schema`.
- The catalog API carries no datasource or DSN field anywhere; a policy cannot store a connection or reach another database.

A saved policy takes effect immediately: the next `GET /api/v1/tables` lists the resource and the generic table API serves it. Deleting a policy deactivates the resource on the next request.

## Add another managed table

1. Create the physical table in the deployment's MySQL database (add its DDL to `deploy/mysql/schema.sql` while no database has been deployed yet).
2. Save a policy document for it through `PUT /api/v1/policies/<resource>`.
3. The table is now externally reachable; no rebuild, no restart, no code change.
4. Add a MySQL integration scenario before exposing it to Web.

Do not accept table names or column names from HTTP requests beyond the validated policy document. A table becomes externally reachable only through a policy row present in the catalog; what is in the catalog is entirely what the database says.
