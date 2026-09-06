# Admin

Admin governs access to rows in existing relational configuration tables without owning those tables' schemas.

## Language

**Local Account（本地账号）**:
An end-user account belonging to the organization served by one Admin deployment, whose credentials are managed by this system. Its permanent identity is independent of its login name and profile details.
_Avoid_: Database user, deployment token, Operator

**Account ID（账号标识）**:
The permanent identifier of one Local Account, unchanged by updates to the account's displayed name or other profile details. It identifies the author of changes attributed to that account.
_Avoid_: Username, email address, display name

**Account Email（账号邮箱）**:
The required, unique email address declared by a Local Account. It is unverified account information, not proof of mailbox ownership or a login or recovery credential.
_Avoid_: Verified email, login name, identity proof

**Account Status（账号状态）**:
Whether a Local Account is enabled or disabled for authenticated access to Admin.
_Avoid_: Policy Status, email verification status, user role

**Login Session（登录会话）**:
A time-bounded authenticated period of access to Admin associated with one Local Account. One account may have several concurrent Login Sessions, which can be ended individually or together.
_Avoid_: Database session, Policy Snapshot, deployment token

**Managed Data Source**:
The single MySQL database selected by deployment configuration and available to one deployment for management. It cannot be selected or changed by runtime policy.
_Avoid_: MySQL instance, arbitrary database, external DSN

**Managed Table**:
An existing base table governed by an enabled Table Policy. Its sole primary-key column is named `id`, and its schema is maintained outside Admin.
_Avoid_: View, system table, remote table, arbitrary table

**Table Policy（表规则）**:
A runtime-configured assignment of one predefined Query Policy and one predefined Mutation Policy to an existing base table by stable code. Enabling the assignment makes that table a Managed Table; the Table Policy does not override either assigned Policy, duplicate database field metadata, or contain connection information.
_Avoid_: Table configuration, database configuration, schema migration, 表策略

**Query Policy（查询规则）**:
A predefined, versioned rule set governing single-table queries against a Managed Table. A Table Policy references it by stable code and cannot override it per table.
_Avoid_: Query Spec, SQL template, 查询策略

**Mutation Policy（变更规则）**:
A predefined, versioned rule set governing changes to rows in a Managed Table, including whether ADD, MODIFY, and DELETE are allowed and which standard audit fields are server-managed. A Table Policy references it by stable code and cannot override it per table.
_Avoid_: Repository, database trigger, 变更策略

**Policy Code（规则编码）**:
The immutable, versioned business identifier of one Query Policy or Mutation Policy. It remains stable for the lifetime of the Policy and does not expose the storage technology used to execute it.
_Avoid_: Database ID, Go type name, Strategy Code, 策略编码

**Policy Type（规则类型）**:
The stable category that defines the shape and execution contract shared by one or more Query Policies or Mutation Policies. A Policy supplies the rules for its Type; a Table Policy never selects a Type directly.
_Avoid_: Policy Code, runtime plugin, 策略类型

**Policy Status（规则状态）**:
The lifecycle state of a Query Policy or Mutation Policy. A Draft may be edited but not assigned, an Active Policy is immutable and assignable, and a Deprecated Policy remains valid for existing assignments but cannot receive new ones.
_Avoid_: Table Policy enabled state, deletion flag, 策略状态

**Operator**:
The identity attributed to the author of a configuration or policy change. For a change made under a Local Account, it is that account's permanent Account ID rather than its login name or display name.
_Avoid_: Display name, deployment identity, auditor

**Policy Catalog（规则目录）**:
The authoritative collection of Query Policies, Mutation Policies, and enabled or disabled Table Policies. It is governed independently from managed tables, cannot manage itself through the generic table-management capability, and denies generic access when no complete valid Policy assignment can be obtained.
_Avoid_: Managed table, policy table, 策略目录

**Policy Snapshot（规则快照）**:
A Table Policy assignment together with the complete Query Policy and Mutation Policy it references, read consistently for one data request and used for that request's validation and execution.
_Avoid_: Live policy lookup, 策略快照

**Query Spec**:
A domain-level declaration of a requested single-table query against one Managed Table, before storage-specific validation and compilation.
_Avoid_: SQL, GORM query

**Change Set**:
A complete field-by-field comparison of one pending ADD, MODIFY, or DELETE against a Managed Table row, used for final confirmation before the change is executed.
_Avoid_: Release, revision, audit record
