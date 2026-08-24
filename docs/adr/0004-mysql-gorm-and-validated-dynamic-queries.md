# Use MySQL and GORM behind a validated dynamic-query pipeline

第一迭代以 MySQL 8.4 LTS 为默认开发、测试和部署数据库，并以 GORM、`gorm.io/driver/mysql`、`go-sql-driver/mysql` 和 `database/sql` 作为主要持久化栈。普通 CRUD 使用 GORM Repository；动态查询必须依次经过领域 Query Spec、Table Policy 校验和 MySQL Query Compiler，再生成 GORM Clauses；特殊参数化 SQL 只能封装在 Repository 内，以兼顾开发效率、运行时表访问能力与 SQL 边界控制。

## Considered Options

暂不采用 PostgreSQL、sqlc、GORM AutoMigrate 或让 HTTP 请求直接表达 SQL。PostgreSQL 若在后续加入，将使用独立 Adapter 和 Compiler，不在当前实现中预埋兼容分支。

## Consequences

第一迭代的 Query Spec 只能读取一个 Managed Table，不支持 Join、多跳关系、子查询或聚合。关系型查询属于后续独立扩展，不能通过 Raw SQL 绕过当前边界。

第一种 Query Policy 采用分页单表查询，行为以既有 `DBPaginationQueryStrategy` 为参考重新设计。第一迭代的 Mutation 成功后直接提交到 Managed Table，不保存 `release_type_list`，也不提供 ChangeSet 或发布流程。

该策略使用稳定标识 `mysql_page_query_v1`，支持 AND 连接的 `exact`、`contains`、`open_range`、`closed_range`、`in`、`not_in`、`is_null` 和 `is_not_null` 条件、单字段排序、精确总数及页码分页。默认页大小为 20、最大为 200；最多 20 个条件、每个集合最多 100 个值、最大 Offset 为 10,000，非法参数直接失败，不进行静默修正。

Count 与分页数据在同一个只读事务和一致性快照中完成，并分别受 3 秒查询超时限制。空字符串是可查询的真实值，JSON `null` 不会被静默忽略。

动态 SQL 的表名只能来自当前 Policy，字段必须先由实时 Schema 解析，方向和操作符使用封闭枚举，所有值使用绑定参数。`contains` 把用户输入的通配符字符视为字面量后再添加两侧通配符；Handler 不能向 Repository 传递 SQL 片段。

Query 响应包含一次性的列元数据、JSON String 行数据和页码信息。所有计数使用 64 位语义；空结果保留请求页码并返回空行、零总数与零总页数。
