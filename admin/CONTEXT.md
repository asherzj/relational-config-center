# Admin

Admin governs access to rows in existing relational configuration tables without owning those tables' schemas.

## Language

**Managed Data Source**:
The single MySQL database selected by deployment configuration and available to one deployment for management. It cannot be selected or changed by runtime policy.
_Avoid_: MySQL instance, arbitrary database, external DSN

**Managed Table**:
An existing base table governed by an enabled Table Policy. Its sole primary-key column is named `id`, and its schema is maintained outside Admin.
_Avoid_: View, system table, remote table, arbitrary table

**Table Policy**:
A runtime-configured rule that assigns one Query Policy and one Mutation Policy to an existing base table and declares whether ADD, MODIFY, and DELETE are allowed. Enabling the policy makes that table a Managed Table; the policy does not duplicate database field metadata or contain connection information.
_Avoid_: Table configuration, database configuration, schema migration

**Query Policy**:
The named strategy and its configuration that govern single-table queries against a Managed Table. A fresh strategy instance executes each request.
_Avoid_: Query Spec, SQL template

**Mutation Policy**:
The named strategy and its configuration that govern changes to rows in a Managed Table. A fresh strategy instance executes each request.
_Avoid_: Repository, database trigger

**Operator**:
The value attributed to server-managed audit fields during a mutation. In the first iteration it comes from deployment configuration and is not an authenticated end-user identity.
_Avoid_: User, auditor

**Policy Catalog**:
The authoritative collection of enabled or disabled Table Policies. It is governed independently from managed tables, cannot manage itself through the generic table-management capability, and denies generic access when no enabled valid policy can be obtained.
_Avoid_: Managed table, policy table

**Policy Snapshot**:
The complete Table Policy definition loaded at the start of one data request and used for that request's validation and execution.
_Avoid_: Live policy lookup

**Query Spec**:
A domain-level declaration of a requested single-table query against one Managed Table, before storage-specific validation and compilation.
_Avoid_: SQL, GORM query
