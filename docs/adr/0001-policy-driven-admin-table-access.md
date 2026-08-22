# Store policy-driven Admin table access as runtime data

Admin exposes generic APIs only for relational tables with an active Table Policy. Users configure Table Policies after deployment and the policies are persisted in a database table, so exposing or changing a managed table does not require rebuilding Admin. Requests use a database-independent Query Specification that is checked against the persisted policy before the MySQL adapter compiles it to GORM clauses; clients still cannot submit unapproved table names, column names, operators, or SQL.

The persisted Policy Catalog is a built-in Admin resource with dedicated management APIs. It never manages itself through the generic table API, which removes the first-start bootstrap cycle and keeps policy authorization separate from ordinary table access.

## Consequences

- Admin needs dedicated Policy Catalog endpoints before any ordinary table is exposed.
- Policy changes need explicit activation, validation, and cache-consistency semantics.
- GORM remains an adapter concern; domain and HTTP packages do not expose `*gorm.DB` or raw SQL.
