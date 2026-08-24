# Admin governs existing tables without owning their schemas

Admin 第一迭代只管理 Table Policy 和既有 Managed Table 中的行数据；表的创建、修改、删除与迁移由部署者在系统外负责，Admin 只读取元数据进行校验。这样可以在不引入通用 Schema Migration 能力的情况下提供受控的数据管理，并避免把“关系型配置管理”扩张成数据库设计器。

## Consequences

- `deploy/mysql/init/001-schema.sql` 只负责系统自身的控制面结构，不负责创建或升级使用者的业务表。
- 初始化脚本不创建数据库账号或保存凭据；Compose 的开发配置创建测试账号，生产账号和授权由部署系统或 DBA 管理。
- Admin 不保存表结构快照或 Schema Fingerprint；每次操作以实时 MySQL 元数据为准，并且永远不会自动修改表结构。
- 普通新增列不会使 Policy 失效：Query 读取最新全量字段，ADD/MODIFY 校验本次提交与自动填充字段，DELETE 只依赖 `id`。只有当前操作无法满足实时结构约束时才拒绝该请求。
- ADD 还会检查所有非空、无默认值且非生成的字段是否已经由请求或 Auto Fill 提供；MODIFY 只检查本次涉及字段，DELETE 只检查表与 `id` 不变量。
