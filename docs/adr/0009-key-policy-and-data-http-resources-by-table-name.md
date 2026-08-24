# Key policy and data HTTP resources by table name

Table Policy 不再维护独立的 `code` 或 `name`，`table_name` 是领域和 HTTP API 中的唯一稳定标识，Catalog 的自增 `id` 只在 Repository 内部使用。Policy Catalog 使用 `/api/v1/table-policies/{table_name}`，Managed Data 使用 `/api/v1/tables/{table_name}` 下的 query 和 row 资源；显示名称从实时 MySQL Table Comment 获取。

## Consequences

Policy 创建后不能修改 `table_name`，物理表重命名等同于旧 Policy 失效并为新表创建 Policy。所有数据请求必须先取得对应的有效 Table Policy，且 `rcc_*` 系统表永远不能通过通用数据 API 访问。

Policy Catalog 提供 list、create、get、完整 replace、enable 和 disable，不提供 delete、patch、历史版本或批量操作。另提供只读的 `/api/v1/database-tables` 资源，从 `information_schema` 展示可用于创建 Policy 的普通基表；发现不等于授权。

物理表发现结果按表名排序，返回 Table Comment、Policy 是否存在及启用、结构兼容性和稳定的不兼容原因；它不返回 DSN、database 名、索引详情或 Catalog 内部 ID，第一迭代不分页。
