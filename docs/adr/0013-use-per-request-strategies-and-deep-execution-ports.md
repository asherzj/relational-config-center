# Use per-request strategies and deep execution ports

Application 持有启动后只读的 Query 与 Mutation Constructor map，Composition Root 显式注册策略并在重复标识时启动失败；不使用 `init`、可变全局注册或运行时插件。每次请求创建独立 Strategy，Strategy 通过 Application port 调用 QueryExecutor 或 MutationExecutor，不接触 `*gorm.DB`。

MySQL Adapter 在深执行端口内部拥有事务：分页 Query 在一个只读一致性事务内完成 Count 与 Scan，每次 Mutation 在独立写事务内完成。事务、GORM Session、隔离和回滚不会泄漏到 Application。动态行使用 Domain 的 TableName、ColumnName、JSONString、Cell 与 Row 表达，HTTP 和 MySQL 分别在边界完成转换。
