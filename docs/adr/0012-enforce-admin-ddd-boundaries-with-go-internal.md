# Enforce Admin DDD boundaries with Go internal packages

Admin 的实现位于 `admin/internal`，按 `interfaces/http`、`application`、`domain`、`infrastructure/mysql` 和 `platform` 分区；`cmd/admin` 只作为 Composition Root。Go 的 `internal` 机制阻止 Server、Client 和 Shared 导入 Admin 领域实现，Domain 不依赖 Gin、GORM、database/sql、HTTP DTO 或 MySQL 类型。

Policy Aggregate 的 Repository contract 属于 Domain；实时元数据读取、Query 执行和 Mutation 执行端口属于 Application。Infrastructure 实现这些端口，Interfaces 只依赖 Application，所有具体实现都在 Composition Root 组装。
