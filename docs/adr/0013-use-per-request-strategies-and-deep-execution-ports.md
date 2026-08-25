# Use per-request strategies and deep execution ports

> ADR-0016 extends both data paths to a complete relational Policy Snapshot. Query keeps the separate Table, Query, and Mutation Policy reads, live Schema validation, Count, and Scan in one read-only `REPEATABLE READ` transaction. Mutation keeps those separate Catalog/Schema reads, database-time Auto Fill production, authorization, and the governed row change in one read-write `REPEATABLE READ` transaction. Both transactions are owned by the MySQL Adapter; this ADR remains authoritative for application ports and keeping GORM/transaction details out of Application.

Application 持有启动后只读的 Query 与 Mutation Policy Type registry；不使用 `init`、可变全局注册或运行时插件。每次请求依据事务内解析的关系化 Policy 定义创建独立执行器，通过 Application port 调用深层 Query/Mutation session，不接触 `*gorm.DB`。

MySQL Adapter 在深执行端口内部拥有事务：分页 Query 在一个只读一致性事务内完成 Policy Snapshot、Count 与 Scan；每次 Mutation 在一个读写事务内完成 Policy Snapshot、数据库时间读取与行变更，事务内的行执行器不会再开启嵌套事务。事务、GORM Session、隔离和回滚不会泄漏到 Application。动态行使用 Domain 的 TableName、ColumnName、JSONString、Cell 与 Row 表达，HTTP 和 MySQL 分别在边界完成转换。
