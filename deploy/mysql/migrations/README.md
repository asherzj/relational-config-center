# Policy Catalog migrations

The reusable Policy model uses an expand-contract rollout. Apply migrations
001 through 005 in order, deploy the transactional Policy Snapshot Admin, then
run the contraction command below. A fresh installation uses the already-final
`mysql/init/001-schema.sql` and does not replay these legacy migrations.

## Table Policy Code expansion

After `005-expand-table-policy-code-references.sql`, run the set-wide preflight
and backfill from the `admin` module:

```bash
POLICY_MIGRATION_OPERATOR=historical-maintainer go run ./cmd/policy-migrate
```

The command uses `MYSQL_*` and requires the separate historical attribution
setting `POLICY_MIGRATION_OPERATOR`. It opens a maintenance connection without
normal Admin HTTP configuration or account-schema readiness. It first locks and backfills the legacy set only when every row is
representable. It then repeats a contraction preflight that requires both
non-null, valid technology-neutral Codes, existing executable
Active/Deprecated definitions of registered Types, valid relational scalar
rules, and representable legacy configuration. A failed preflight prints sorted
table-scoped diagnostics and drops no column. On success it applies the final
single Table Policy ALTER.

Representable legacy behavior is normalized as follows:

- omitted page-query values become `id DESC`, default page size `20`, and max
  page size `200`, matching the legacy constructor;
- explicit valid page-query scalars are preserved;
- `allow_add`, `allow_modify`, and `allow_delete` move into the reusable
  Mutation Policy;
- an ADD-only `operator`/`now` target maps to a Create operator/time slot;
- a MODIFY `operator`/`now` target must also appear identically in ADD and maps
  to a Modify operator/time slot, preserving ADD and MODIFY behavior.

Policy Codes are deterministic, technology-neutral hashes of normalized
semantics and end in `_v1`, so identical legacy rules reuse one Active Policy.

The preflight rejects unknown strategy/configuration fields, invalid query
scalars, literal or unknown Auto Fill sources, multiple targets for one fixed
slot, non-mirrored MODIFY rules, and partial Code backfills. Diagnostics include
the affected `table_name`. On failure the transaction rolls back every Policy
insert and Table Policy update; the nullable expand columns and indexes remain
available for a corrected rerun.

## Standalone contract migration

[`006-contract-legacy-table-policy-columns.sql`](./006-contract-legacy-table-policy-columns.sql)
is provided for DBA-controlled migration runners. Its stored-procedure guard
rechecks Codes, definition existence/lifecycle/registered Types, relational
scalars, and legacy Query/Mutation/Auto Fill representability before reaching
its only destructive `ALTER TABLE`. Any assertion raises `SQLSTATE 45000`; all
legacy columns remain intact. Prefer the Go command because its diagnostics are
more precise, but direct execution of 006 is independently fail closed.

The final `rcc_table_policies` columns are exactly `id`, `table_name`, both
Policy Codes, `enabled`, `creator`, `modifier`, `gmt_created`, and
`gmt_modified`. No foreign keys or optimistic-lock columns are added.

## Local Account control tables

After completing the existing Policy Catalog migration, apply
[`007-local-accounts.sql`](./007-local-accounts.sql) once with database maintenance
permissions. Fresh installations already include the same tables in
`init/001-schema.sql`; do not replay 007 there. These are explicit InnoDB tables,
not GORM AutoMigrate output. Never expose any `rcc_` table through generic policies.

Business requests now require local-account sessions and TMP-01 is removed by #37.
Stop old Admin entry points before changing authentication; do not run Token-based
instances in parallel with the new entry. Normal startup/readiness verifies the required account schema and rejects missing
structures with migration guidance. The complete maintenance-window sequence and
proxy/script replacement are documented below.
See [the account interface and development setup](../../../docs/admin-local-accounts.md).

## Global account roles

After 007, stop old Admin instances and apply
[`008-account-roles.sql`](./008-account-roles.sql). It is restartable and gives
existing accounts VIEWER without resetting previously granted roles on reruns.
Fresh installations already contain the same columns and history table.
Startup/readiness requires the role schema, but deliberately does not require an
existing administrator: registration and read-only login must work before the
maintainer explicitly runs `account-maintain grant-admin` for a selected account.
See [role bootstrap, recovery and HTTP contracts](../../../docs/admin-account-roles.md).

## 记录版本（009）

停止全部旧版本和外部写入后执行 `009-record-versions.sql`。该迁移只建立受保护控制表，不改业务表；重跑保留所有版本。新 Admin 启动及就绪检查要求其列、唯一键及 InnoDB 引擎完整。Admin/Web 必须一起切换为版本必填调用方，不能并行运行旧写入者。

存量基线为 0，删除保留版本条目。外部 SQL、表重建、排序规则、数据库或身份算法升级必须按 [记录版本维护流程](../../../docs/admin-record-versions.md) 提高整表维护基线并重新确认；不允许清空版本控制表或降级继续写入。

## 发布草稿与在途目标（010 / 011）

完成 009 后顺序执行 `010-release-drafts.sql` 和 `011-release-targets.sql`。
010 持久保存草稿/历史及按账号、动作、请求标识的成功结果；011 建立提交时取得的已知记录唯一目标。
两者均可重跑，不能清空旧请求、历史或占用来恢复服务。Ready 检查控制结构和 InnoDB，
完整前不恢复业务服务；新安装的 001 已包含相同定义。

提交审批还要求可证明的目标表/Schema TRIGGER 元数据权限，否则明确拒绝冻结。
没有占用过期清理任务。T5 已删除旧记录直写路由，继续应用下面的 012 后使用正式执行入口。详见 [审批、冻结与恢复契约](../../../docs/admin-release-approvals.md)。

## 原子发布结果（012）

停写维护窗口内，在 011 后执行 `012-publication.sql`。该幂等迁移仅建立 `rcc_table_publications`、`rcc_publication_commands`、`rcc_refresh_notifications`，不改变业务表；新安装 001 包含完全相同定义。Ready 要求精确列/主键与 InnoDB，不允许清空记录或重置游标。

部署维护账号需按实际 Admin 登录身份显式授予 `GRANT PROCESS ON *.* TO '<admin-user>'@'<host>'`；目标表的 TRIGGER 元数据授权仍必须可证明。PROCESS 用于读取隐藏跨 schema 外键的完整 InnoDB 字典，无法读取时发布在业务写入前明确拒绝。该全局授权不在迁移中自动执行，不能仅靠目标 schema 的 SELECT 推断没有外部级联。详见[能力边界与持久结果](../../../docs/design-notes/publication-contract.md)。

成功只表示数据库提交，通知状态 NOT_CONNECTED；不运行投递器。全部 Admin/Web 应一同切换，旧 rows 客户端会明确拒绝，不提供兼容开关。后续批量和反向发布按工单计划继续交付。


T5 同时修正 FLOAT 主键的有损短文本权重与 FLOAT/DOUBLE 的正负零等价。应用新二进制前，须取消受影响表的旧在途单并停写，按 [记录版本维护门禁](../../../docs/admin-record-versions.md#t5-浮点身份修订的升级门禁) 为全部 FLOAT/DOUBLE 主键表推进维护基线、保留旧 key。012 不自动完成这项维护，也不能据其可重跑而跳过代际切换。
