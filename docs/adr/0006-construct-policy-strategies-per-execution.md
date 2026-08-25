---
status: accepted
---

# Construct policy strategies from explicit registries per execution

> ADR-0016 supersedes this ADR's persistence boundary in which each Table Policy directly carries strategy identifiers and JSON configuration. This ADR remains authoritative for fresh per-execution construction, explicit code-owned Type registries, and application-level execution ports.

最终模型由 Query/Mutation Policy 的稳定 `type_code` 选择代码内显式注册的执行契约，具体规则来自关系化 Policy 定义。每次数据操作都从事务内读取的完整 Policy Snapshot 创建新执行器，实例不跨请求复用。

## Consequences

- 持久化标识不能直接使用 Go 类型名，代码重命名不得改变已有 Policy 语义。
- 第一迭代不使用反射、动态插件或运行时注册；未知标识一律拒绝执行。
- 构造函数接收依赖和策略配置，返回新实例；请求与 `context.Context` 只传给 `Execute`，不通过可变 `Build` 方法保存在对象中。
- 策略依赖 application 定义的 Repository port，不能直接依赖 `infrastructure/mysql`。
- 每个 Type 使用关系化强类型字段并复用同一条激活、绑定和执行校验路径。
- 第一迭代不缓存 Table Policy，每次请求直接读取 Catalog，使原子替换和启停在提交后的下一个请求生效。
- `ChangeMatchFields` 和 `field_infos` 不进入新模型；表结构与字段类型来自实时 MySQL 元数据。

第一迭代只注册 `page_query` 和 `single_table_mutation`。前者使用 Query Policy 的默认排序与页大小，后者使用 Mutation Policy 的三个授权值与四个标准 Auto Fill 槽；Table Policy 不包含执行覆盖。
