# Admin V1 技术基线

本文是 Admin 第一迭代的冻结设计。它汇总可直接指导实现和验收的边界；取舍理由见 [ADR](./adr/)，领域语言见 [Admin Context](../admin/CONTEXT.md)。

## 1. 目标与范围

Admin 是 Web 的管理端后端，治理部署配置指定的一个 MySQL database 中的既有配置表。Admin 不创建、修改、删除或迁移业务表结构，只通过实时元数据验证并管理表中的行数据。

第一迭代只实现 Admin 后端：

- Gin HTTP/JSON API；不启动 gRPC Server。
- Table Policy 专用管理 API。
- 单表分页查询和按 `id` 的单行变更。
- 物理表发现、健康检查、认证和生产形态的进程管理。
- MySQL 8.4、真实数据库集成测试和 Docker Compose 开发环境。

本迭代不实现 Web UI、Server、Client、发布流程、审批、Schema 管理、多租户、多数据源、缓存、持久化审计或秘密管理。

## 2. 技术选择

| 领域 | 选择 |
|---|---|
| HTTP | Gin + HTTP/JSON |
| 数据库 | MySQL 8.4 LTS；兼容要求为 MySQL 8.0+ |
| ORM | GORM，唯一主要 DAL |
| Dialect | `gorm.io/driver/mysql` |
| Driver | `go-sql-driver/mysql` |
| 连接池 | `database/sql` |
| 动态查询 | Query Spec → Table Policy → MySQL Query Compiler → GORM Clauses |
| 普通 CRUD | GORM Repository |
| 特殊 SQL | 仅 Repository 内使用参数化 Raw SQL |
| 初始化 | `deploy/mysql/init/001-schema.sql` + Docker Compose |
| Migration | 暂不引入；出现已部署实例升级需求后再引入 Goose |
| 测试 | Go `testing`、`httptest`、Testcontainers、真实 MySQL 8.4 |

不采用 Hertz、Kitex、sqlc、GORM AutoMigrate、自研 Gateway、Redis、消息队列或 PostgreSQL 兼容分支。

## 3. 架构与依赖方向

```text
admin/
├── cmd/admin/main.go                 Composition Root
└── internal/
    ├── interfaces/http/              Gin Handler、DTO、中间件、Router
    ├── application/                  Policy、Discovery、Query、Mutation 用例
    ├── domain/                       Policy、Table、Query、Mutation 模型
    ├── infrastructure/mysql/         Catalog、Metadata、Compiler、Executor
    └── platform/                     Config、Logging、HTTP Server
```

依赖规则：

```text
interfaces/http → application → domain
                         ↑
infrastructure/mysql ────┘
```

- Domain 不依赖 Gin、GORM、`database/sql`、HTTP DTO 或 MySQL 类型。
- `domain/policy.Repository` 持久化 Table Policy Aggregate。
- Application 定义 `TableMetadataReader`、`QueryExecutor`、`MutationExecutor` 端口。
- Infrastructure 实现端口并隐藏 GORM Session 与事务。
- Admin 不依赖 `shared`、Server、Client、grpc-go 或 Protobuf。

## 4. 核心领域模型

### Managed Data Source

部署环境指定的唯一 MySQL database。Policy 不能保存 DSN、切换 database 或建立额外连接。

### Managed Table

满足以下条件并拥有 enabled Table Policy 的普通基表：

- 位于 Managed Data Source。
- 不是 `rcc_*` 控制面表、视图或系统表。
- `id` 是字面名称且唯一的单列主键。

### Table Policy

每个物理表至多一条 Policy，以 `table_name` 为领域和 HTTP 标识。Policy 只包含 Query/Mutation 策略绑定和 `enabled|disabled` 状态，不包含 `code`、`name`、字段列表、Allowlist、Schema Fingerprint、发布信息、revision 或连接信息。

新 Policy 创建为 disabled；完整替换保持当前状态；enable 重新验证表与策略；disable 立即停止数据授权。没有 delete、draft、回滚或历史版本。Policy 变更采用 last-write-wins。

## 5. Policy Catalog

`rcc_` 是系统控制表保留前缀，代码中的受保护表集合是最终边界。

```sql
CREATE TABLE `rcc_table_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `table_name` varchar(64) NOT NULL,
  `query_policy` varchar(100) NOT NULL,
  `query_policy_config` json NOT NULL,
  `mutation_policy` varchar(100) NOT NULL,
  `mutation_policy_config` json NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_table_name` (`table_name`)
);
```

JSON 配置必须是 object；空配置使用 `{}`，不能使用 SQL NULL。每个策略使用强类型配置并拒绝未知字段。创建、替换、启用和执行复用相同的 Constructor 校验路径。

## 6. 策略注册与执行

Application 持有两个启动后只读的 Constructor map：

```go
type QueryRegistry map[string]QueryConstructor
type MutationRegistry map[string]MutationConstructor
```

- Composition Root 显式注册策略；不使用 `init`、反射、动态插件或运行时注册。
- 重复策略名阻止启动，未知策略名 fail closed。
- 每次请求调用 Constructor 创建独立 Strategy 实例。
- `context.Context` 和请求仅传给 `Execute`，不使用会保存请求状态的 `Build`。
- Strategy 依赖 Application ports，不直接导入 MySQL Infrastructure。
- Table Policy 每次请求从 Catalog 读取，不使用进程缓存或 Redis。

第一迭代只注册 `mysql_page_query_v1` 和 `mysql_single_table_mutation_v1`。

## 7. 实时 Schema 与动态值

Admin 不保存 `field_infos` 或 Schema Fingerprint，每次操作读取实时 MySQL 元数据：

- Query 返回当前全量字段，因此任一返回字段类型不支持时拒绝请求。
- ADD 校验提交与 Auto Fill 字段，并检查所有非空、无默认值、非生成字段已提供。
- MODIFY 只校验本次提交与 Auto Fill 字段。
- DELETE 只验证目标表与 `id` 不变量。
- 新增可空或有默认值的字段自动兼容。

Domain 使用 `TableName`、`ColumnName`、`JSONString`、`Cell` 和 `Row` 表示动态数据。HTTP 值使用 JSON String；SQL NULL 使用 JSON `null`，缺少字段表示 ADD 使用数据库默认行为或 MODIFY 保持原值，空字符串始终是真实空字符串。

支持整数、`DECIMAL`、浮点、字符与文本、`ENUM`、`TINYINT(1)`、日期时间和 `JSON`。不支持 BINARY、VARBINARY、BLOB、BIT、SET 与空间类型。

时间格式：

- `DATE`：`YYYY-MM-DD`
- `TIME`：`HH:mm:ss[.ffffff]`
- `DATETIME`：`YYYY-MM-DD HH:mm:ss[.ffffff]`
- `TIMESTAMP`：RFC 3339 UTC

MySQL Session 使用 UTC。

## 8. 分页查询策略

配置：

```json
{
  "default_order": {"field": "id", "direction": "DESC"},
  "default_page_size": 20,
  "max_page_size": 200
}
```

Query Spec：

```json
{
  "conditions": [
    {"field": "status", "operator": "exact", "value": "AVAIL"},
    {"field": "id", "operator": "closed_range", "from": "10", "to": "100"}
  ],
  "order": {"field": "id", "direction": "DESC"},
  "page_number": 1,
  "page_size": 20
}
```

规则：

- 条件全部以 AND 连接。
- 支持 `exact`、`contains`、`open_range`、`closed_range`、`in`、`not_in`、`is_null`、`is_not_null`。
- `contains` 只用于字符串，且用户输入中的 `%`、`_` 和转义符按字面量处理。
- 空字符串不能被忽略；NULL 只能使用 NULL 操作符。
- Range 至少包含一个边界；IN/NOT IN 必须是非空字符串数组。
- 单字段排序，方向仅允许 ASC/DESC；请求可覆盖默认排序。
- 最多 20 个条件、集合最多 100 个值、Page Size 最大 200、Offset 最大 10,000。
- Count 与 Scan 在同一个只读一致性事务中执行，各受 3 秒超时限制。

响应：

```json
{
  "columns": [
    {"name": "id", "type": "uint64", "nullable": false},
    {"name": "status", "type": "string", "nullable": false}
  ],
  "rows": [{"id": "42", "status": "AVAIL"}],
  "page": {
    "page_number": 1,
    "page_size": 20,
    "total_count": 1,
    "total_pages": 1
  }
}
```

空结果返回 `rows: []`、零总数和零总页数，并保留合法请求页码。

## 9. 单表变更策略

配置：

```json
{
  "allow_add": true,
  "allow_modify": true,
  "allow_delete": false,
  "auto_fill": {
    "add": {
      "creator": {"source": "operator"},
      "modifier": {"source": "operator"},
      "gmt_created": {"source": "now"},
      "gmt_modified": {"source": "now"}
    },
    "modify": {
      "modifier": {"source": "operator"},
      "gmt_modified": {"source": "now"}
    }
  }
}
```

- ADD 可写非生成列；自增 `id` 可省略，返回 `{"id":"42"}`。
- MODIFY 按 `id` 更新一行、使用 PATCH 语义、禁止修改 `id`，返回 `{"affected":1}`。
- DELETE 默认禁止；显式允许后按 `id` 硬删除，返回 `{"affected":1}`。
- MySQL 唯一索引是唯一性的最终裁决，重复键映射为 409。
- MODIFY 采用 last-write-wins，不使用 ETag、版本列或原值比较。
- Driver 启用 ClientFoundRows；写入原值仍返回匹配行，零行表示 `id` 不存在。
- 每次 Mutation 在 MySQL Adapter 内的独立事务中执行。

Auto Fill 使用结构化规则，只支持 `operator`、`now` 和 `literal`。服务端值覆盖客户端值；任何解析、字段或类型错误都使操作失败。OperatorProvider 第一迭代从 `ADMIN_OPERATOR` 返回固定值，不代表已认证用户。

## 10. HTTP API

### Database Table Discovery

```text
GET /api/v1/database-tables
GET /api/v1/database-tables/{table_name}
```

实时读取当前 database 的普通基表，排除 `rcc_*`，按表名排序，不分页。返回 Table Comment、Policy 是否存在及启用、结构兼容性和稳定原因；不返回 DSN、database 名、索引详情或 Catalog ID。发现不构成授权。

### Policy Catalog

```text
GET  /api/v1/table-policies
POST /api/v1/table-policies
GET  /api/v1/table-policies/{table_name}
PUT  /api/v1/table-policies/{table_name}
POST /api/v1/table-policies/{table_name}/enable
POST /api/v1/table-policies/{table_name}/disable
```

POST 创建 disabled Policy；PUT 完整替换并保持状态。没有 DELETE、PATCH、批量或历史 API。

### Managed Data

```text
POST   /api/v1/tables/{table_name}/query
POST   /api/v1/tables/{table_name}/rows
PATCH  /api/v1/tables/{table_name}/rows/{id}
DELETE /api/v1/tables/{table_name}/rows/{id}
```

所有数据 API 必须先加载 enabled Policy。Compiler 的表名来自 Policy，字段来自实时 Schema，方向和操作符是封闭枚举，所有值通过参数绑定；Handler 不能传入 SQL 片段。

## 11. 错误模型

```json
{
  "error": {
    "code": "invalid_query_condition",
    "message": "field id does not support contains",
    "request_id": "..."
  }
}
```

| HTTP | 类别 |
|---|---|
| 400 | JSON、分页、条件和值格式错误 |
| 401 | Bearer Token 错误 |
| 403 | `rcc_*` 或禁止目标 |
| 404 | Policy、物理表或目标行不存在 |
| 409 | Policy 已存在、唯一键冲突 |
| 422 | 策略或实时 Schema 不兼容 |
| 500 | 未分类内部错误 |
| 503 | 数据库或 Catalog 不可用 |
| 504 | 查询超时 |

响应不包含 SQL、DSN、Driver 原文或调用栈。Domain/Application 返回稳定分类错误，HTTP 层负责状态码映射，Infrastructure 负责转换底层错误。

## 12. MySQL 与运行配置

配置只来自环境变量：

```text
ADMIN_HTTP_ADDR
ADMIN_API_TOKEN
ADMIN_AUTH_DISABLED
ADMIN_OPERATOR
ADMIN_CORS_ORIGINS

MYSQL_HOST
MYSQL_PORT
MYSQL_DATABASE
MYSQL_USER
MYSQL_PASSWORD
MYSQL_TLS_MODE
MYSQL_MAX_OPEN_CONNS
MYSQL_MAX_IDLE_CONNS
MYSQL_CONN_MAX_LIFETIME
MYSQL_CONN_MAX_IDLE_TIME
MYSQL_CONNECT_TIMEOUT
MYSQL_READ_TIMEOUT
MYSQL_WRITE_TIMEOUT
```

使用 `go-sql-driver/mysql.Config` 构造 DSN。固定启用 ParseTime、UTC、ClientFoundRows 与 utf8mb4；固定关闭 MultiStatements、InterpolateParams 与 AllowAllFiles。Compose 显式关闭 TLS，生产默认要求 TLS。

连接池默认值：MaxOpenConns 10、MaxIdleConns 10、ConnMaxLifetime 3 分钟、ConnMaxIdleTime 1 分钟。全进程只有一个连接池，启动时必须 Ping 成功。

## 13. HTTP 安全与进程基线

- 默认监听 `127.0.0.1:8080`。
- 只有绑定 loopback 且显式设置 `ADMIN_AUTH_DISABLED=true` 时才能关闭 Token。
- `/api/v1/**` 使用 Bearer Token、Request ID、1 MiB Body Limit、Recovery、精确 CORS Allowlist 和结构化访问日志。
- `/health/live` 与 `/health/ready` 无认证；Readiness 检查 MySQL 与 Catalog，不受单个业务表影响。
- 日志为 stdout JSON，只记录 Request ID、方法、稳定路由模板、状态码和耗时；不记录 Token、DSN、动态 table/row path 值、查询值、Mutation Content、返回数据、完整 SQL 或绑定参数。未知路由使用固定安全占位。
- 配置、注册表、MySQL 或 Catalog 错误会阻止启动；不在启动时遍历全部 Policy。
- SIGINT/SIGTERM 触发最长 10 秒优雅停机，最后关闭连接池。
- 第一迭代不提供 Metrics，也不建立持久化审计表。

## 14. 初始化与 Migration

```text
deploy/
├── docker-compose.yml
├── .env.example
└── mysql/init/
    └── 001-schema.sql
```

`001-schema.sql` 只创建控制面表；`002-dev-users.sh` 使用环境变量创建本地 Admin 读写账号和预留 Server 只读账号。生产账号由部署系统或 DBA 创建。Testcontainers 复用同一 Schema。

Docker 初始化脚本只在空数据目录执行。第一迭代没有 Migration；出现已部署实例升级需求时引入 Goose，禁止用 AutoMigrate 修改 Schema。

## 15. 测试与验收

测试分层：

- Domain 纯单元测试。
- Application Use Case、Registry 和 Factory 的 Fake Port 测试。
- Query Compiler 表驱动测试。
- `httptest` 覆盖 DTO、认证、中间件、路由和错误映射。
- Testcontainers `mysql:8.4` 覆盖 Catalog、实时元数据、分页、Mutation、事务和 Driver 行为。
- 关键路径覆盖 HTTP → Application → 真实 MySQL；不 Mock GORM。

`make test` 运行单元测试，`make test-integration` 运行 Docker 集成测试。测试使用独立 database 或事务回滚隔离。

Admin V1 的 Definition of Done：

- 运行配置、健康检查、认证、中间件与优雅停机可用。
- 物理表发现、Policy 创建/替换/启停可用。
- 单表分页、全部已定操作符、排序和精确 Count 可用。
- ADD、PATCH、可选硬 DELETE 与 Auto Fill 可用。
- `rcc_*` 永久拒绝，动态 SQL 只使用验证标识和绑定参数。
- 主要失败路径具有稳定错误码。
- MySQL 8.4 集成测试覆盖上述主路径。

## 16. 后续触发条件

- Web UI：另行选择框架并按本文 HTTP 契约接入。
- Server/Client：另行设计各自领域模型和 gRPC/Protobuf 契约。
- PostgreSQL：新增独立 Adapter 与 Compiler。
- 已部署 Schema 升级：引入 Goose。
- 多租户、公网访问或真实用户审计：重新设计身份、授权与隔离。
- Policy/Data 并发控制、缓存、发布、审批、关系查询和 Secret 管理：作为独立能力设计，不隐式扩展当前策略。
