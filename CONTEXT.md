# Relational Configuration Center

Relational Configuration Center manages configuration as relational data with explicit schemas, constraints, references, and query rules.

## Language

**Managed Table**:
A relational table exposed through Admin because it has an active Table Policy.
_Avoid_: arbitrary table, raw table

**Table Policy**:
Runtime configuration maintained by a user after deployment that defines how a Managed Table may be queried or changed, including visible fields, permitted operators, sorting, and mutations. Changing a Table Policy does not require rebuilding or restarting Admin.
_Avoid_: table config, database permission

**Policy Catalog**:
The built-in collection of Table Policies managed through dedicated Admin capabilities. It is not a Managed Table and cannot be queried or changed through the generic table API.
_Avoid_: policy table resource, self-managed table

**Query Specification**:
A client-supplied, database-independent description of filters, sorting, and pagination that must be accepted by a Table Policy before execution.
_Avoid_: SQL, query string

**Admin**:
The management-plane backend that owns configuration authoring and exposes HTTP APIs to Web.
_Avoid_: admin frontend, Web

**Web**:
The browser frontend used to interact with Admin.
_Avoid_: Admin

**Server**:
The data-plane backend that serves configuration to runtime consumers.
_Avoid_: Admin, Gateway
