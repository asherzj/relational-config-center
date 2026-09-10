# Admin V1 技术基线

本文是 Admin 第一迭代的冻结设计。它汇总可直接指导实现和验收的边界；取舍理由见 [ADR](./adr/)，领域语言见 [Admin Context](../admin/CONTEXT.md)。

当前实现说明：本文冻结的是 Admin 后端第一迭代的历史范围。当前账号角色、同表混合发布、独立审批与审批回滚见[发布单升级与操作指南](admin-release-upgrade.md)；旧记录直写 HTTP 路由已删除。正式 Web 管理台已经在 [`web/README.md`](../web/README.md) 和 [`web/DESIGN.md`](../web/DESIGN.md) 所述入口交付；Server、Client 仍未提供运行时配置服务或账户功能。

当前并发语义已由 [ADR-0021](./adr/0021-store-record-versions-outside-business-tables.md) 及 [记录版本契约](./admin-record-versions.md) 替代下文受管记录的 last-write-wins 约定；规则目录并发语义不变。当前数据库版本与身份维护要求也以该契约为准。

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
| 初始化 | 独立 `schema-migrate` Goose 任务 + Docker Compose |
| Migration | 独立 Goose 迁移/接管/恢复；Admin 只读门禁；历史 SQL/Policy 维护保留旧库职责 |
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
- Domain Catalog ports 持久化 Query、Mutation 与 Table Policy。
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

每个物理表至多一条 Table Policy，以不可修改的 `table_name` 为领域和 HTTP 标识。它只保存 `query_policy_code`、`mutation_policy_code`、`enabled` 与审计信息；ADD/MODIFY/DELETE 授权和标准 Auto Fill 都属于被引用的 Mutation Policy，不能按表覆盖。

新规则创建为 disabled；完整替换保持当前状态；enable 重新验证表与规则；disable 立即停止数据授权。没有 delete、draft、回滚或历史版本。规则变更采用 last-write-wins。

## 5. Policy Catalog

`rcc_` 是系统控制表保留前缀，代码中的受保护表集合是最终边界。

```sql
CREATE TABLE `rcc_table_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `table_name` varchar(64) NOT NULL,
  `query_policy_code` varchar(100) NOT NULL,
  `mutation_policy_code` varchar(100) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_table_name` (`table_name`),
  KEY `idx_table_policy_query_code` (`query_policy_code`),
  KEY `idx_table_policy_mutation_code` (`mutation_policy_code`)
);
```

完整定义见[嵌入 Goose 迁移](../admin/internal/infrastructure/mysql/migrations/)及其目标结构清单。`rcc_query_policies` 关系化保存 `page_query` 的默认排序和分页标量；`rcc_mutation_policies` 关系化保存操作授权及四个可空审计目标槽。三个表都使用 `created_at` / `updated_at`，具有唯一、查询索引和标量 CHECK，不使用外键或乐观锁。

Policy API 同样返回 `created_at` / `updated_at`。旧库按 [013 升级说明](../deploy/mysql/migrations/README.md#policy-审计时间统一013) 重命名原 `gmt_created` / `gmt_modified` 列，保留时间值，并同步升级 Admin/Web。`rcc_table_field_policies` 采用相同审计命名，由 Goose 00003 增加；未接管旧库使用[历史 014](../deploy/mysql/migrations/014-table-field-policies.sql) 后显式接管；管理员字段交互配置、实时读取与回退契约见[字段规则管理](admin-field-policies.md)。该表独立于Query/Mutation执行授权，不新增显示配置快照。

该关系化取舍由 [ADR-0016](./adr/0016-separate-policy-definitions-from-table-assignments.md) 冻结，并取代 ADR-0010 的内联 JSON 模型。Policy Code 是不可修改、版本化、技术无关的业务标识；定义遵循 `DRAFT -> ACTIVE -> DEPRECATED`，新绑定只能选择 Active，现有 Deprecated 绑定仍可执行。

新部署直接使用最终结构。旧 Catalog 按 [`migrations/README.md`](../deploy/mysql/migrations/README.md) 执行 expand/backfill/contract：Go 命令先对全表做可表示性与引用预检，006 SQL 自身也会在任何 destructive ALTER 前 fail closed。升级完成后 fresh 与 upgrade 的列顺序、类型、NULL/default/collation、索引顺序和 CHECK 等价。

## 6. 策略注册与执行

Application 持有两个启动后只读的 Policy Type registry：

```go
type QueryPolicyTypeRegistry map[string]QueryTypeBuilder
type MutationPolicyTypeRegistry map[string]MutationTypeContract
```

- Registry 由代码显式固定；不使用 `init`、反射、动态插件或运行时注册。
- 数据库定义引用未知 Type 时 fail closed。
- 每次请求从关系化定义创建独立执行器实例。
- `context.Context` 和请求仅传给 `Execute`，不使用会保存请求状态的 `Build`。
- Strategy 依赖 Application ports，不直接导入 MySQL Infrastructure。
- 每个请求在一个 `REPEATABLE READ` 事务内分别读取 Table、Query、Mutation Policy，不 JOIN、不使用进程缓存或 Redis。

第一迭代只注册技术无关的 `page_query` 和 `single_table_mutation` Type。

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

Query Policy 关系字段：

```text
default_order_field = "id"
default_order_direction = "DESC"
default_page_size = 20
max_page_size = 200
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
- 最多 256 个 AND 条件（范围计一条）、集合最多 100 个值、Page Size 最大 200、Offset 最大 10,000。
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

计算列在 `columns` 中额外返回 `generated: true`（普通列省略），用于修改表单排除数据库生成的只读字段。
修改表单以查询到的原记录初始化全部可编辑字段，直接编辑并提交完整值；主键、计算列和 Auto Fill 字段不进入修改内容，NULL 与空字符串保持原义。

## 9. 单表变更策略

Mutation Policy 的关系化授权与标准 Auto Fill：

```text
allow_add = true
allow_modify = true
allow_delete = false
create_operator_field = "creator"
create_time_field = "created_at"
modify_operator_field = "modifier"
modify_time_field = "updated_at"
```

- ADD 可写非生成列；仅自增 `id` 可省略，非自增 `id` 即使有数据库默认值也须显式提交，否则写入前返回 `missing_required_field`。返回 `{"id":"42"}`，完整保留 uint64 主键；显式自增零值按当前 MySQL SQL 模式返回实际保存或生成的 ID。
- MODIFY 按 `id` 更新一行、使用 PATCH 语义、禁止修改 `id`，返回 `{"affected":1}`。
- DELETE 默认禁止；显式允许后按 `id` 硬删除，返回 `{"affected":1}`。
- MySQL 唯一索引是唯一性的最终裁决，重复键映射为 409。
- MODIFY 采用 last-write-wins，不使用 ETag、版本列或原值比较。
- Driver 启用 ClientFoundRows；写入原值仍返回匹配行，零行表示 `id` 不存在。
- 每次 Mutation 在 MySQL Adapter 内的独立事务中执行。

Auto Fill 只支持上述四个固定槽：ADD 填 Create 与 Modify 槽，MODIFY 只填 Modify 槽，DELETE 不填。Operator 槽取当前请求已认证账号的永久 Account ID，Time 槽取同一事务的数据库时间。客户端提交任一服务端管理字段会被拒绝，而不是覆盖；literal 与任意规则不在最终模型中。

## 10. HTTP API

### Database Table Discovery

```text
GET /api/v1/database-tables
GET /api/v1/database-tables/{table_name}
```

实时读取当前 database 的普通基表，排除 `rcc_*`，按表名排序，不分页。返回 Table Comment、Policy 是否存在及启用、结构兼容性和稳定原因；不返回 DSN、database 名、索引详情或 Catalog ID。发现不构成授权。

### Policy Catalog

```text
GET    /api/v1/query-policy-types
GET    /api/v1/query-policies
POST   /api/v1/query-policies
GET    /api/v1/query-policies/{code}
PUT    /api/v1/query-policies/{code}
PATCH  /api/v1/query-policies/{code}/metadata
POST   /api/v1/query-policies/{code}/activate
POST   /api/v1/query-policies/{code}/deprecate
DELETE /api/v1/query-policies/{code}

GET    /api/v1/mutation-policy-types
GET    /api/v1/mutation-policies
POST   /api/v1/mutation-policies
GET    /api/v1/mutation-policies/{code}
PUT    /api/v1/mutation-policies/{code}
PATCH  /api/v1/mutation-policies/{code}/metadata
POST   /api/v1/mutation-policies/{code}/activate
POST   /api/v1/mutation-policies/{code}/deprecate
DELETE /api/v1/mutation-policies/{code}

GET  /api/v1/table-policies
POST /api/v1/table-policies
GET  /api/v1/table-policies/{table_name}
PUT  /api/v1/table-policies/{table_name}
POST /api/v1/table-policies/{table_name}/enable
POST /api/v1/table-policies/{table_name}/disable
GET  /api/v1/table-policies/{table_name}/release-templates
PUT  /api/v1/table-policies/{table_name}/release-templates/{release_type}
```

Query/Mutation Draft 可完整替换或删除，Active/Deprecated 仅允许更新显示元数据。Table Policy POST 创建 disabled assignment；PUT 完整替换并保持状态。Table Policy 没有 DELETE、PATCH、批量或历史 API。所有 Table Policy 写入必须携带 `Idempotency-Key`，PUT 和启停正文必须携带读取到的十进制字符串 `expected_version`；响应提供 `version`。同键原操作重推返回持久原结果，同键异内容返回 `idempotency_conflict`；陈旧版本返回 `table_policy_conflict`。新建仍为 disabled，并在同一事务创建默认应急关联；启用与有效应急关联一起成功。完整关联契约见[表发布流程设置](admin-table-release-templates.md)。

### Managed Data

```text
POST   /api/v1/tables/{table_name}/query
POST   /api/v1/release-orders
POST   /api/v1/release-orders/{id}/submit
POST   /api/v1/release-orders/{id}/approve
POST   /api/v1/release-orders/{id}/execute
```

数据确认只保存草稿；独立审批后的正式执行才写配置，旧三条 rows 写路由已删除。完整控制事务、最终行与恢复见 [发布结果契约](design-notes/publication-contract.md)。

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
| 401 | 会话失效或登录失败 |
| 403 | `rcc_*` 或禁止目标 |
| 404 | Policy、物理表或目标行不存在 |
| 409 | Policy 已存在、唯一键冲突 |
| 422 | 规则或实时 Schema 不兼容 |
| 500 | 未分类内部错误 |
| 503 | 数据库或 Catalog 不可用 |
| 504 | 查询超时 |

响应不包含 SQL、DSN、Driver 原文或调用栈。Domain/Application 返回稳定分类错误，HTTP 层负责状态码映射，Infrastructure 负责转换底层错误。

## 12. MySQL 与运行配置

配置只来自环境变量：

```text
ADMIN_HTTP_ADDR
ADMIN_PUBLIC_ORIGIN
ADMIN_ALLOW_LOCAL_HTTP
ADMIN_TRUSTED_PROXIES
ADMIN_REGISTER_LIMIT
ADMIN_LOGIN_IP_LIMIT
ADMIN_LOGIN_FAILURE_LIMIT

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
- 业务 API 始终需要当前账号会话；非 GET/HEAD 请求需要会话 CSRF 与配置同源 Origin/Referer。旧 Token、免认证和正常固定 Operator 配置明确拒绝。
- `/api/v1/**` 使用 Cookie 会话、Request ID、1 MiB Body Limit、Recovery 和结构化访问日志；注册、登录与登录前 CSRF 准备入口遵循[账号契约](admin-local-accounts.md)。不提供跨源 CORS 访问。
- `/health/live` 与 `/health/ready` 无认证；Readiness 检查 MySQL 与 Catalog，不受单个业务表影响。
- 日志为 stdout JSON，只记录 Request ID、方法、稳定路由模板、状态码和耗时；不记录 Token、DSN、动态 table/row path 值、查询值、Mutation Content、返回数据、完整 SQL 或绑定参数。未知路由使用固定安全占位。
- 配置、注册表、MySQL 或 Catalog 错误会阻止启动；不在启动时遍历全部 Policy。
- SIGINT/SIGTERM 触发最长 10 秒优雅停机，最后关闭连接池。
- 第一迭代不提供 Metrics，也不建立持久化审计表。

## 14. 初始化与 Migration

当前新安装和后续升级统一执行 `schema-migrate up`，迁移 SQL 与目标结构清单嵌入独立维护二进制。Compose 依次运行迁移任务、独立开发 fixture、Admin；任何一步失败阻止后续启动。存量库先完成适用的历史 SQL/Policy 维护，再显式 `baseline`；未确认迁移须核查后显式 `recover`，禁止自动重试或跳过版本。

Admin 启动和运行中 `/health/ready` 只读核查版本前缀、尝试状态、发行摘要和完整必要结构。维护工具通过 `OpenMaintenance` 独立工作，正常业务连接不需要 DDL 或迁移登记权限。生产数据库账号仍由部署系统或 DBA 管理。当前初始化旧入口已删除；`testdata/pre-goose-8b5cd859.sql` 是冻结旧库测试材料，不能用于当前安装。详见[迁移手册](schema-migrations.md)和[历史升级说明](../deploy/mysql/migrations/README.md)，仍禁止使用 AutoMigrate 修改 Schema。

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

- Web UI：历史范围曾留待另行选择框架；当前正式实现位于 `web/`，按本文 HTTP 契约接入并由 [`web/README.md`](../web/README.md) 说明运行与验收边界。
- Server/Client：另行设计各自领域模型和 gRPC/Protobuf 契约。
- PostgreSQL：新增独立 Adapter 与 Compiler。
- 控制表 Schema 升级：通过独立 `schema-migrate` 初始化、向前升级、严格接管与显式恢复，见[迁移维护手册](schema-migrations.md)。Compose 先迁移、再独立 fixture、再 Admin；存量库先完成[适用历史步骤](../deploy/mysql/migrations/README.md)后显式接管，不自动登记基线。后续新增控制结构只追加 Goose 迁移和目标结构清单。
- 多租户、公网访问或真实用户审计：重新设计身份、授权与隔离。
- 受管记录并发保护、发布和审批已由上述当前契约取代历史规划；规则目录并发控制、缓存、关系查询和 Secret 管理仍待后续设计。


当前发布草稿 API、权限、字段差异及幂等恢复见[发布草稿契约](admin-release-drafts.md)。本文保留第一迭代的历史路由设计；旧记录写入口已由正式发布流程取代，不再提供写入能力。
