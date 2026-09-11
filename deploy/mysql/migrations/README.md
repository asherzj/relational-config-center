# Policy Catalog migrations

These scripts retain their historical upgrade responsibilities. After reaching
the current structure, including applicable historical steps through 017, an existing database can be explicitly
adopted with `schema-migrate baseline`; the command verifies the complete control
schema before registering versions and never replays these scripts. See the
[Goose maintenance runbook](../../../docs/schema-migrations.md). Goose `up`
refuses unadopted control tables. Compose runs Goose before its independent development fixture and Admin.
Admin startup/readiness require a current, confirmed, structurally complete release.

The reusable Policy model uses an expand-contract rollout. Apply migrations
001 through 005 in order, then 013 before running the current contraction
command below. A fresh installation uses the already-final
`schema-migrate up` and does not replay these legacy migrations.

## Table Policy Code expansion

After `005-expand-table-policy-code-references.sql`, apply
`013-policy-audit-timestamps.sql` to align all three catalogs with the current
maintenance command's audit mappings. Then run the set-wide preflight and
backfill from the `admin` module:

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
The historical 006 script renames Table Policy audit columns back to `gmt_*`;
if using that script, apply 013 again afterwards. The current Go contraction
command keeps `created_at` / `updated_at` throughout.

The final `rcc_table_policies` columns are exactly `id`, `table_name`, both
Policy Codes, `enabled`, `creator`, `modifier`, `created_at`, and
`updated_at` at this historical stage; 016 adds `concurrency_key`. No foreign keys or optimistic-lock columns are added.

## Local Account control tables

After completing the existing Policy Catalog migration, apply
[`007-local-accounts.sql`](./007-local-accounts.sql) once with database maintenance
permissions. Fresh installations already include the same tables in
`schema-migrate up`; do not replay 007 there. These are explicit InnoDB tables,
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
010 持久保存草稿/历史及按账号、动作、请求标识的成功结果；011 建立记录唯一目标，016 起提前到保存草稿取得。
两者均可重跑，不能清空旧请求、历史或占用来恢复服务。Ready 检查控制结构和 InnoDB，
完整前不恢复业务服务；新安装使用嵌入 Goose 迁移。

提交审批还要求可证明的目标表/Schema TRIGGER 元数据权限，否则明确拒绝冻结。
没有占用过期清理任务。T5 已删除旧记录直写路由，继续应用下面的 012 后使用正式执行入口。详见 [审批、冻结与恢复契约](../../../docs/admin-release-approvals.md)。

## 原子发布结果（012）

停写维护窗口内，在 011 后执行 `012-publication.sql`。该幂等迁移仅建立 `rcc_table_publications`、`rcc_publication_commands`、`rcc_refresh_notifications`，不改变业务表；新安装使用嵌入 Goose 迁移。Ready 要求精确列/主键与 InnoDB，不允许清空记录或重置游标。

部署维护账号需按实际 Admin 登录身份显式授予 `GRANT PROCESS ON *.* TO '<admin-user>'@'<host>'`；目标表的 TRIGGER 元数据授权仍必须可证明。PROCESS 用于读取隐藏跨 schema 外键的完整 InnoDB 字典，无法读取时发布在业务写入前明确拒绝。该全局授权不在迁移中自动执行，不能仅靠目标 schema 的 SELECT 推断没有外部级联。详见[能力边界与持久结果](../../../docs/design-notes/publication-contract.md)。

成功只表示数据库提交，通知状态 NOT_CONNECTED；不运行投递器。全部 Admin/Web 应一同切换，旧 rows 客户端会明确拒绝，不提供兼容开关。同源多表合计 1～1,000 项发布与原单回滚共用这些控制结构，无额外临时兼容表；完整维护窗口和恢复步骤见[发布单升级指南](../../../docs/admin-release-upgrade.md)。


T5 同时修正 FLOAT 主键的有损短文本权重与 FLOAT/DOUBLE 的正负零等价。应用新二进制前，须取消受影响表的旧在途单并停写，按 [记录版本维护门禁](../../../docs/admin-record-versions.md#t5-浮点身份修订的升级门禁) 为全部 FLOAT/DOUBLE 主键表推进维护基线、保留旧 key。012 不自动完成这项维护，也不能据其可重跑而跳过代际切换。

## Policy 审计时间统一（013）

`rcc_query_policies`、`rcc_mutation_policies`、`rcc_table_policies` 的审计时间统一为
`created_at` / `updated_at`，HTTP 返回字段同步改名。Web 与 Admin 必须一起升级。

1. 备份数据库，停止旧 Admin 和其他 Policy 写入者。
2. 对已完成 012 的数据库执行 `013-policy-audit-timestamps.sql`。
3. 确认所有适用历史控制步骤（包括 014～017）和当前 `policy-migrate` 收缩均已完成，按[接管手册](../../../docs/schema-migrations.md#校验并接管现有库)显式运行 `schema-migrate baseline`，随后 `schema-migrate up` 升级到当前构建，再以 `schema-migrate status` 确认 `state=current`、没有未确认操作。
4. 部署新版 Admin/Web，确认就绪检查及三类 Policy 的查询、创建和修改均正常。

迁移只重命名列，保留现有时间值、数据类型、默认值和自动更新时间行为；不会修改业务表，
也不会改写 Mutation Policy 中配置的审计目标字段。新安装使用 `schema-migrate up` 即可。
迁移可重跑，支持表间中断和单个时间列已经改名的状态；若旧名和新名同时存在或同时缺失，
会在任何表发生变更前拒绝执行，并指出异常表。修正异常后可重跑。
旧版 Admin 不能使用迁移后的列名。登记 Goose 后禁止在账本之外反向改名或跳版本；需恢复旧部署时，使用一致的已验证备份及匹配二进制，保持停写窗口。Goose 只支持经验证的向前修复。

`rcc_table_field_policies` 同样采用 `created_at` / `updated_at`，由014创建；013本身不创建该表。

## 表字段规则（014）

通过 mysql 客户端执行包含中文列注释的历史脚本时，必须使用 `--default-character-set=utf8mb4`；不能让客户端把 UTF-8 文件按其他字符集导入。

014 仅供尚未接管且已完成 013 的历史数据库：停写时执行适用步骤及 `014-table-field-policies.sql`，并完成 015～017，再按[接管手册](../../../docs/schema-migrations.md)显式 `baseline` 后执行 `up` 升级后续版本，确认 current 后部署 Admin/Web。新库和已使用 Goose 的库运行 `schema-migrate up`，由 00003 创建同一字段策略结构。两条路径均保留已有规则、发布单、幂等结果、版本、占用、通知和业务行。Admin 只读完整验证字段表的列、默认值、索引、CHECK、引擎以及版本状态；同名不兼容表不会被 IF NOT EXISTS 修正，应在维护窗口核对，不能删表绕过校验。

交互控件默认text，无auto；JSON NULL语义、原子保存及实时读取见[字段规则契约](../../../docs/admin-field-policies.md)。旧发布单清理属于独立且需明确指定隔离环境的维护工作，绝非014升级前提。

## 原单成功执行（015）

完成 014 后，在停写维护窗口应用
[`015-original-order-executions.sql`](./015-original-order-executions.sql)。它建立
`rcc_release_details` 和 `rcc_release_executions`，为 Command 与通知增加
`execution_id`，将通知主键改为 `(execution_id, table_name)`。原单发布及回滚
分别保存一次成功执行；申请、发布实际结果和回滚实际结果按项独立保存于明细。

迁移可重跑，不删除业务数据、控制历史、旧请求或通知，不提供旧发布单 JSON
转换或新旧双写。旧 Command 和通知仅补 `legacy:<order_id>` 技术身份，保留
原内容和状态；同表多条旧通知仍独立，重跑不覆盖已存在的执行身份。
已有旧发布单数据时，保持停写并确认留存及环境切换方案。新安装使用嵌入 Goose `up`。启动就绪检查要求完整新结构，详情拒绝旧整单格式。
详见[升级与原单操作指南](../../../docs/admin-release-upgrade.md)。

## 草稿目标与并发管控键（016）

完成 015 后，在停写窗口执行 [`016-draft-target-reservations.sql`](./016-draft-target-reservations.sql)。它为 Table Policy 增加非空 JSON 字段 `concurrency_key`（默认 `[]`），建立 `rcc_release_table_references(table_name, order_id)`，保护没有具体主键的未结束明细。目标仍使用 `rcc_release_targets`，主键身份算法不变，附加键使用独立编码命名空间。

迁移可重跑，保留业务数据、规则、历史、原请求和已有目标；不会回填、转换或删除旧发布单数据。新旧 Admin/Web 不能并行运行。已有旧发布单时继续保持停写并明确环境切换与留存方案；不借新引用表为空来接受旧单。Ready 必须看见新字段及完整 InnoDB 引用表结构，新安装 Goose 当前结构与完成 017 后的结构一致。

## 删除主单默认表（017）

完成 016 后执行 [`017-release-main-order.sql`](./017-release-main-order.sql)，在一个原子 ALTER 中删除 `release_table` 索引和 `table_name` 列。每项申请始终明确自身表；业务 JSON 不转换或清理。完成当前全部结构后才显式 baseline。已接管库使用 Goose 00004/00005，不重放此历史链。
