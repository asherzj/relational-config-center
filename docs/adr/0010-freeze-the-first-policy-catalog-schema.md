# Freeze the first Policy Catalog schema

第一迭代使用受保护的 `rcc_table_policies` 表保存每个物理表唯一的一份 Table Policy，只包含自增内部 ID、`table_name`、两个策略标识及 MySQL JSON 配置、`enabled`、创建修改人和时间。新 Policy 默认 disabled，`table_name` 创建后不可修改；Catalog 不包含 `code`、`name`、Allowlist、字段信息、Schema Fingerprint、发布信息或 revision。
