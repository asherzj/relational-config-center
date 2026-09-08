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
The single database available to one Admin deployment for management. It cannot be selected or changed by runtime policy.
_Avoid_: Database instance, arbitrary database

**Managed Table**:
An existing base table governed by an enabled Table Policy, whose schema is maintained outside Admin.
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
The identity attributed to the person executing a configuration or policy change. For a change made under a Local Account, it is that account's permanent Account ID; for a publication or rollback it identifies the publisher, separately from the applicant and approver.
_Avoid_: Display name, deployment identity, auditor

**Policy Catalog（规则目录）**:
The authoritative collection of Query Policies, Mutation Policies, and enabled or disabled Table Policies. It is governed independently from managed tables, cannot manage itself through the generic table-management capability, and denies generic access when no complete valid Policy assignment can be obtained.
_Avoid_: Managed table, policy table, 策略目录

**Policy Snapshot（规则快照）**:
A Table Policy assignment together with the complete Query Policy and Mutation Policy it references, read consistently for one data request and used for that request's validation and execution.
_Avoid_: Live policy lookup, 策略快照

**Query Spec**:
A declaration of filters, sorting, and pagination requested against one Managed Table, subject to its assigned Query Policy.
_Avoid_: SQL, GORM query

**Change Set**:
A complete field-by-field comparison of one proposed ADD, MODIFY, or DELETE against a Managed Table row, used to review the intended change before authorizing it for publication.
_Avoid_: Release, revision, audit record

**Release Order（发布单）**:
The authoritative record of a proposed configuration change and its progression through approval, publication, cancellation, or rollback, associating the requested content with its applicant and operation history.
_Avoid_: Change Set, deployment, notification task

**Release Order Title（发布单标题）**:
The applicant-provided short description of a Release Order's intent, fixed when the order is submitted. It does not replace the order's identifier or the name of its Managed Table.
_Avoid_: Release Order ID, table name, record title

**Release Approval（发布审批）**:
The decision of an authorized person other than the applicant to approve or reject the frozen content of a submitted Release Order. It applies only to that submitted content, not to later edits or another order.
_Avoid_: Login, publication, self-confirmation

**Record Version（记录并发版本）**:
The monotonically advancing concurrency identity of one configuration record, used to reject changes based on an older record state, including after deletion and recreation of the same record identity.
_Avoid_: Release Order Version, Table Version, historical revision

**Release Order Version（发布单版本）**:
The revision of a Release Order's editable content or workflow state, used to ensure that an action applies to the order state its caller observed.
_Avoid_: Record Version, Table Version

**Table Version（表发布版本）**:
The publication progress of one Managed Table, advanced when a change set is committed for that table. It identifies published table progress rather than the concurrency identity of an individual record.
_Avoid_: Record Version, Release Order Version, cache refresh time

**Rollback Release Order（回滚发布单）**:
A new Release Order requesting the reversal of a previous publication, linked to that publication and subject to fresh approval and checks that no later change would be overwritten.
_Avoid_: History deletion, forced restore, cancellation

**Reprepared Release Order（重新准备发布单）**:
A new editable Release Order that replaces an approved but unpublished ordinary Release Order after its original applicant or an administrator reviews the current configuration. The replacement belongs to the person who performs the operation and must receive a fresh independent approval; cancelling the source, releasing its Active Targets, creating the replacement and linking both histories form one atomic change.
_Avoid_: Editing an approval, approval reuse, quick rollback

**Active Target（在途目标）**:
A known configuration record identity reserved by a submitted, unfinished Release Order so that another order cannot simultaneously submit a conflicting change to that identity.
_Avoid_: Draft editing lock, business-field similarity

**Account Role（账号角色）**:
A global, composable grant governing the actions a Local Account may perform across this deployment's Managed Tables and Release Orders. Every role includes viewing; editing, approval and publication do not imply one another, and no role permits approval of one's own order.
_Avoid_: Account Status, Table Policy, per-table permission

**Publication Command（发布变更记录）**:
An immutable record of a configuration change actually committed by a publication, retaining the final configuration state or the fact of deletion and linking it to its originating Release Order.
_Avoid_: Approval request, editable draft, SQL command

**Refresh Notification Record（刷新通知记录）**:
A durable record that a committed publication requires downstream consumers to refresh. Its existence does not mean that a consumer has received or applied the change.
_Avoid_: Published configuration, delivery receipt, cache version

## Related documents

- [Admin technical baseline](../docs/admin-v1-technical-baseline.md): database selection and Managed Table schema requirements.
- [Record Versions](../docs/admin-record-versions.md): database identity equivalence and maintenance generations.
- [Account roles](../docs/admin-account-roles.md): default roles, administrator appointment and recovery.
