# Govern one configured MySQL database and deny access by default

每个部署只连接配置中指定的一个 MySQL database，Table Policy 只能选择其中的普通基表，不能携带 DSN、跨 database 或指向视图和系统表。通用数据接口仅在取得有效且启用的 Table Policy 后才允许访问；Policy 缺失、失效或 Catalog 不可用时一律拒绝，以把动态数据访问限制在显式授权面内。

## Consequences

- 第一迭代不支持多数据源、跨库查询、视图或运行时连接配置。
- Managed Table 必须以字面名称 `id` 作为唯一主键；复合主键、其他名称的主键和无主键表不能创建有效 Policy。
- Policy Catalog 使用保留的 `rcc_` 表名前缀；这些表由代码永久排除，不能成为 Policy 目标。前缀用于命名隔离，真正的授权边界仍由代码中的受保护表集合保证。
- Policy 只做表级治理，不保存 `field_infos`：启用一张表会暴露其实时 Schema 中的全部字段，并允许策略按实时字段类型处理查询和变更。新增字段会自动进入该边界。
- 每个请求以开始处理时取得的 Policy Snapshot 完成校验和执行。
