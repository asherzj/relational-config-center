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
A runtime-configured assignment of one predefined Query Policy and one predefined Mutation Policy to an existing base table by stable code, together with the table's Approval Role assignment and optional Concurrency Control Key. Enabling the assignment makes that table a Managed Table; the Table Policy does not override either assigned Policy, duplicate database field metadata, or contain connection information.
_Avoid_: Table configuration, database configuration, schema migration, 表策略

**Concurrency Control Key（并发管控键）**:
An optional single field or field combination defined by a Table Policy whose values, compared using the fields' database equality rules with NULL treated as a value, identify additional targets protected against other Release Orders. It supplements primary-key protection and cannot be added, changed, or removed while any unfinished Release Order contains a change for that table.
_Avoid_: Primary key, unique constraint, Release Order Version

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

**Table Field Policy（表字段规则）**:
A current, table-and-field-specific interaction rule guiding Web display, query controls and record entry against a real field. It may be disabled without losing its configuration. It does not grant execution authority, override Query or Mutation Policies, or become a publication display snapshot.
_Avoid_: Field permission, Schema definition, enum authorization, 字段执行规则

**Policy Catalog（规则目录）**:
The authoritative collection of Query Policies, Mutation Policies, Table Field Policies, and enabled or disabled Table Policies. It is governed independently from managed tables, cannot manage itself through the generic table-management capability, and denies generic access when no complete valid Policy assignment can be obtained.
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

**Release Type（发布方式）**:
A predefined category of release workflow shared by one or more Release Templates. Standard database publication includes approval, while emergency publication omits approval; the category does not identify one particular template.
_Avoid_: Release Template Code, deployment environment, rollback order

**Release Template（发布流程模板）**:
A reusable, named configuration of an ordered release workflow, identified by a stable unique code and one immutable Release Type. It contains Release Node Definitions rather than the progress or results of a particular Release Order. An emergency template must remain available; a standard template may be disabled.
_Avoid_: Release Order, Release Execution, individual node, SQL template

**Table Release Template（表发布模板关联）**:
The single selection of a Release Template for one Table Policy and one Release Type. Standard associations may be disabled; an emergency association remains valid while its table is managed and can be replaced without an unavailable interval.
_Avoid_: Release Node Definition, Release Order instance, per-node configuration

**Release Node Definition（发布节点定义）**:
One named step within a Release Template, using a predefined workflow capability and its allowed authority parameters. Its code is unique within that template, and its position follows the sequence allowed by the template's Release Type.
_Avoid_: Release Execution, workflow progress, arbitrary script, database row change

**Release Table Flow（逐表流程实例）**:
The workflow acknowledged for one involved Managed Table when a Release Order draft is saved, retaining its selected Release Type and ordered node definitions. Later template changes or table reassignment do not replace that saved workflow.
_Avoid_: Release Template, Approval Assignment Snapshot, whole-order template

**Release Node Instance（发布节点实例）**:
One step in a Release Table Flow, retaining the acknowledged definition and the progress recorded through actual table decisions or whole-order publication and completion. Any attributed person and time belong to the action that produced that progress.
_Avoid_: Release Node Definition, current approval eligibility, inferred completion

**Release Order（发布单）**:
The authoritative record of an ordered collection of proposed changes across Managed Tables in one Managed Data Source and its progression through approval, publication, completion, cancellation, or rollback. It associates the frozen application with its applicant, per-table flows, change details, execution results, and operation history.
_Avoid_: Change Set, deployment, notification task

**Release Change Detail（发布单变更明细）**:
One ordered ADD, MODIFY, or DELETE intent for a specific Managed Table within a Release Order, retaining its original application separately from its actual publication and rollback results. Once submitted, its approved intent and order cannot be overwritten by execution results.
_Avoid_: Publication Command, execution result detail, Release Order

**Release Execution（发布执行记录）**:
The record of one successfully committed publication or rollback of an entire Release Order, attributing its type, actor, time, versions, and overall outcome while referring to the actual results retained by its change details. An order has at most one successful publication and one successful rollback; execution records have no independent approval or completion lifecycle.
_Avoid_: Rollback Release Order, failed attempt, approval, notification receipt

**Release Order Title（发布单标题）**:
The applicant-provided short description of a Release Order's intent, fixed when the order is submitted. It does not replace the order's identifier or the name of its Managed Table.
_Avoid_: Release Order ID, table name, record title

**Release Approval（发布审批）**:
The review of a submitted Release Order's frozen content through decisions made by authorized people other than the applicant for its involved Managed Tables. The order is approved only when every involved table is approved; any valid rejection rejects the whole order.
_Avoid_: Login, publication, self-confirmation

**Table Approval（表审批）**:
An authorized person's approval or rejection covering one Managed Table's changes within a submitted Release Order, retaining the person, qualification source, names, opinion and time of the decision. Approval by one eligible person completes that table's requirement; one action covers all tables still pending for that person.
_Avoid_: Approval of an individual record, publication, approval of another table

**Approval Assignment Snapshot（审批分配快照）**:
The role identities and names assigned to review each involved Managed Table when a Release Order is submitted, including an explicitly empty assignment. Membership and eligibility remain current, while later table reassignment does not replace the snapshot or invalidate decisions already completed lawfully.
_Avoid_: Policy Snapshot, Release Table Flow, frozen membership, current table assignment

**Record Version（记录并发版本）**:
The monotonically advancing concurrency identity of one configuration record, used to reject changes based on an older record state, including after deletion and recreation of the same record identity.
_Avoid_: Release Order Version, Table Version, historical revision

**Release Order Version（发布单版本）**:
The revision of a Release Order's editable content or workflow state, used to ensure that an action applies to the order state its caller observed.
_Avoid_: Record Version, Table Version

**Table Version（表发布版本）**:
The publication progress of one Managed Table, advanced when a change set is committed for that table. It identifies published table progress rather than the concurrency identity of an individual record.
_Avoid_: Record Version, Release Order Version, cache refresh time

**Quick Rollback（快速回滚）**:
The whole-order emergency reversal of a successful ordinary publication before Release Completion, authorized by a current publisher after reviewing the restoration intent, without new approval. It retains the original Active Targets throughout restoration and ends the original order on success, with the actual reversal recorded as its rollback Release Execution.
_Avoid_: Rollback Release Order, partial restore, history deletion, forced overwrite

**Rollback Reason（回滚原因）**:
An optional explanation associated with a successful rollback Release Execution, which its executor or an administrator can supply or revise afterward with each change attributed in history. It has no completion deadline and does not gate subsequent publications.
_Avoid_: Release Approval, prerequisite for rollback

**Release Completion（发布完结）**:
The explicit end of a successful ordinary publication's protected recovery period, authorized by a current publisher. It releases the publication's targets and permanently closes rollback for that order without changing configuration values or claiming downstream delivery.
_Avoid_: Publication, cancellation, delivery confirmation

**Reprepared Release Order（重新准备发布单）**:
A new editable Release Order that replaces an approved but unpublished ordinary Release Order after its original applicant or an administrator reviews the current configuration. The replacement belongs to the person who performs the operation and must receive a fresh independent approval; cancelling the source, creating the replacement, transferring still-needed Active Targets and acquiring any additional targets, and linking both histories form one atomic change.
_Avoid_: Editing an approval, approval reuse, quick rollback

**Active Target（在途目标）**:
A primary-key identity or Concurrency Control Key value reserved by a Release Order when its change details are saved, protecting the old and proposed identities needed for publication and rollback against other orders. Reservations continue through publication and end when no draft detail needs them, or the order is cancelled, rejected, completed, or successfully rolled back.
_Avoid_: Database transaction lock, Release Order Version, unique constraint

**Account Role（账号角色）**:
A global, composable grant governing a Local Account's viewing, editing, publication or administration capabilities across this deployment. Table approval eligibility follows Approval Roles and the Default Approver rule, and no account role permits approval of one's own order.
_Avoid_: Account Status, Table Policy, Approval Role, per-table permission

**Approval Role（审批角色）**:
A named, administrator-managed group of Local Accounts that may approve changes to Managed Tables assigned to that role; an account may belong to several such groups. An enabled account's membership in any enabled role in a table's applicable approval assignment establishes eligibility to review that table, subject to the prohibition on approving one's own order.
_Avoid_: Account Role, department, publisher

**Default Approver（默认审批人）**:
An enabled administrator eligible to review a table when its Approval Assignment Snapshot has no enabled role with an enabled member other than the applicant. This eligibility ends for pending approvals when eligible role members become available, and never permits reviewing one's own order or automatic approval.
_Avoid_: Automatic approval, universal approval authority, applicant confirmation

**Notification Center（通知中心）**:
The workspace for finding submitted Release Orders by their relationship to the current account or across the deployment, including pending approvals, the account's applications and the account's completed review actions. Visibility of an order does not itself grant approval authority.
_Avoid_: Refresh Notification Record, downstream delivery, publication execution

**Approval Notification（审批通知）**:
A recipient-and-order indication of pending review or a relevant approval or release outcome, with unread awareness independent of whether review work remains pending. Acknowledging observed progress does not complete approval or consume later changes; losing pending responsibility preserves unread outcomes and completed review history.
_Avoid_: Refresh Notification Record, Table Approval, publication result

**Publication Command（发布变更记录）**:
An immutable record of a configuration change actually committed by a publication, retaining the final configuration state or the fact of deletion and linking it to its originating Release Order and Release Execution.
_Avoid_: Approval request, editable draft, SQL command

**Refresh Notification Record（刷新通知记录）**:
A durable record that a committed publication requires downstream consumers to refresh. Its existence does not mean that a consumer has received or applied the change.
_Avoid_: Published configuration, delivery receipt, cache version

## Related documents

- [Release Template decision](../docs/adr/0027-configure-release-workflows-with-stable-templates.md): stable template identity, constrained nodes, and lifecycle boundaries.
- [Multi-table draft targets and execution decision](../docs/adr/0025-multitable-drafts-reserve-targets-and-record-executions.md): the accepted model for multi-table orders, draft reservations, and original-order rollback.
- [Table approval roles and default reviewers](../docs/adr/0026-authorize-approval-by-table-roles.md): the accepted approval responsibility and eligibility model.
- [Admin technical baseline](../docs/admin-v1-technical-baseline.md): database selection and Managed Table schema requirements.
- [Record Versions](../docs/admin-record-versions.md): database identity equivalence and maintenance generations.
- [Account roles](../docs/admin-account-roles.md): default roles, administrator appointment and recovery.
