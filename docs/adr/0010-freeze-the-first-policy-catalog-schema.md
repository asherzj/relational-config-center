---
status: superseded by ADR-0016
---

# Freeze the first Policy Catalog schema

> 本文仅记录已被替代的第一版内联 JSON 取舍。当前实现与验收以
> [ADR-0016](./0016-separate-policy-definitions-from-table-assignments.md) 为准：
> Query/Mutation 定义已关系化，Table Policy 只保存两个 Policy Code，
> 不再保存 JSON 或每表 `allow_*`。

第一迭代使用受保护的 `rcc_table_policies` 表保存每个物理表唯一的一份 Table Policy，只包含自增内部 ID、`table_name`、两个策略标识及 MySQL JSON 配置、`allow_add`、`allow_modify`、`allow_delete`、`enabled`、创建修改人和时间。新 Policy 的三个 Mutation 能力和状态都默认 disabled，`table_name` 创建后不可修改；Catalog 不包含 `code`、`name`、Allowlist、字段信息、Schema Fingerprint、发布信息或 revision。

## Considered Options

我们评估过按关系型数据库范式拆分策略配置：`default_order_field`、`default_order_direction`、`default_page_size`、`max_page_size`、`allow_add`、`allow_modify` 和 `allow_delete` 都是单值标量，可以成为普通列；重复出现的 Auto Fill 规则可以拆为以 Policy、操作和字段为键的一对多子表。也评估过为每种 Query Policy 和 Mutation Policy 建立一对一的策略专属配置表。

第一迭代仍选择两个 JSON 配置列保存策略专属参数。这不是忽略关系型建模规范，而是把一份策略配置视为由对应策略 Constructor 强类型解析、整体校验和原子替换的值对象。数据请求总是读取完整 Policy Snapshot，当前没有按配置成员跨 Policy 筛选、关联、局部更新或独立审计的需求。完全关系化会为每种策略增加配置表、读取联接、跨表替换事务，以及“策略标识必须匹配且只能存在一份对应配置”的条件完整性；这些复杂度在当前访问模式下没有带来相称收益。

ADD、MODIFY、DELETE 已被定义为所有 Mutation Policy 共享且语义稳定的 Table Policy 能力，因此 `allow_add`、`allow_modify` 和 `allow_delete` 直接进入 `rcc_table_policies`，不建立一对一配置表。默认排序和分页参数仍属于 `mysql_page_query_v1`；Auto Fill 仍属于 `mysql_single_table_mutation_v1`，不进入 Catalog 主表。只有需要按 Auto Fill 规则查询、局部修改或独立审计时，才把规则拆为子表。

## Consequences

- 三个 `allow_*` 列是 Mutation 操作授权的唯一数据源；`mutation_policy_config` 不得再接受同名字段，避免双写不一致。
- JSON 是其余策略专属配置的唯一数据源；不能为其中的成员维护一组可写的影子列。
- 既有 Catalog 必须先把 JSON 中的三个权限值回填到一级列，再删除 JSON 同名成员；不能直接依赖新列默认值。
- JSON 必须是 object，并由策略的强类型 Constructor 拒绝未知字段；创建、替换、启用和执行复用同一校验路径。
- 管理端可以把 JSON 渲染为结构化表单，但表单结构不改变 Catalog 的持久化边界。
- 当出现按配置成员过滤或索引、规则级审计或局部更新、配置跨 Policy 复用、数据库级成员约束，或稳定的跨策略能力时，必须重新评估关系化；不能仅为减少 JSON 外观而拆表。
