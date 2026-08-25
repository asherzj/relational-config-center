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
A runtime-configured assignment of one predefined Query Policy and one predefined Mutation Policy to an existing base table by stable code. Enabling the assignment makes that table a Managed Table; the Table Policy does not override either assigned Policy, duplicate database field metadata, or contain connection information.
_Avoid_: Table configuration, database configuration, schema migration

**Query Policy**:
A predefined, versioned rule set governing single-table queries against a Managed Table. A Table Policy references it by stable code and cannot override it per table.
_Avoid_: Query Spec, SQL template

**Mutation Policy**:
A predefined, versioned rule set governing changes to rows in a Managed Table, including whether ADD, MODIFY, and DELETE are allowed and which standard audit fields are server-managed. A Table Policy references it by stable code and cannot override it per table.
_Avoid_: Repository, database trigger

**Policy Code**:
The immutable, versioned business identifier of one Query Policy or Mutation Policy. It remains stable for the lifetime of the Policy and does not expose the storage technology used to execute it.
_Avoid_: Database ID, Go type name, Strategy Code

**Policy Type**:
The stable category that defines the shape and execution contract shared by one or more Query Policies or Mutation Policies. A Policy supplies the rules for its Type; a Table Policy never selects a Type directly.
_Avoid_: Policy Code, runtime plugin

**Policy Status**:
The lifecycle state of a Query Policy or Mutation Policy. A Draft may be edited but not assigned, an Active Policy is immutable and assignable, and a Deprecated Policy remains valid for existing assignments but cannot receive new ones.
_Avoid_: Table Policy enabled state, deletion flag

**Operator**:
The value attributed to server-managed audit fields during a mutation. In the first iteration it comes from deployment configuration and is not an authenticated end-user identity.
_Avoid_: User, auditor

**Policy Catalog**:
The authoritative collection of Query Policies, Mutation Policies, and enabled or disabled Table Policies. It is governed independently from managed tables, cannot manage itself through the generic table-management capability, and denies generic access when no complete valid Policy assignment can be obtained.
_Avoid_: Managed table, policy table

**Policy Snapshot**:
A Table Policy assignment together with the complete Query Policy and Mutation Policy it references, read consistently for one data request and used for that request's validation and execution.
_Avoid_: Live policy lookup

**Query Spec**:
A domain-level declaration of a requested single-table query against one Managed Table, before storage-specific validation and compilation.
_Avoid_: SQL, GORM query
