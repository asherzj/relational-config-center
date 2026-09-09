# QueryPage 配置驱动分页查询开源技术方案

> 规格版本：1.0.0
> 状态：Implementation Candidate
> 所属上下文：Admin
> 依赖：MySQL 8.0+、现有 Table Policy、Gin HTTP/JSON
> 约束：本文不使用公司内部 FinConfig、Thrift、PSM、Stage 或 Region。

本文定义开源版 Relational Configuration Center 的配置驱动页面查询能力。调用方以 `model_code` 请求页面，Admin 将页面模型解析为一个启用的 Managed Table 和 Table Policy，再完成分页数据、字段交互信息、选项和发布类型组装。本文是未来 Admin 能力，不替代现有按 `table_name` 暴露的 Managed Data 接口。

## 0. 环境隔离新增要求

开源版必须支持环境隔离。环境以稳定 `environment` 编码标识，并分为 `TEST` 和 `PRODUCTION` 两类：`TEST` 环境允许存在多个，`PRODUCTION` 环境全系统只允许一个。系统至少配置一个测试环境和一个生产环境。

QueryPage 请求必须显式携带具体环境编码；Admin 只允许在该环境内解析 Managed Table、执行查询和加载选项，禁止默认环境、跨环境查询或查询失败后的跨环境回退。测试环境和生产环境采用独立数据库还是同库逻辑隔离，需要另行确认。

## 1. 建设目标

调用方只需要提供：

- 具体环境编码；
- 页面模型编码；
- 查询条件；
- 页码和页大小；
- `ALL` 或 `ONLY_DATA` 查询模式。

Admin 必须完成：

1. 将 `model_code` 解析为一个 Managed Table。
2. 加载当前 Policy Snapshot 和实时 MySQL Schema。
3. 使用 Query Spec 执行 Count 与分页查询。
4. 在 `ALL` 模式加载字段、选项和发布类型。
5. 只返回页面字段白名单允许的列。
6. 将数据库值转换为既有 JSON String 表示。

### 1.1 功能范围

第一版支持：

- 单表 AND 条件查询；
- 单字段排序和页码分页；
- 字段展示、查询和编辑元数据；
- 静态选项；
- 受 Table Policy 保护的外部表选项；
- `ALL` 与 `ONLY_DATA`；
- 可选的发布类型展示。

第一版不支持 JOIN、子查询、聚合、任意 SQL、跨数据库查询、页面内直接推进发布单或在 QueryPage 内计算百分比灰度。

## 2. 核心术语

| 术语 | 定义 |
|---|---|
| Page Model | 以 `model_code` 标识的页面配置，引用一个 Managed Table |
| Managed Table | 拥有启用 Table Policy 的现有业务表 |
| Page Field | 页面对一个真实字段的展示、查询和编辑声明 |
| Static Option | 直接存储在页面选项表中的 value/label |
| Table Option | 从另一个 Managed Table 查询得到的 value/label |
| Query Mode | `ALL` 或 `ONLY_DATA` |
| Policy Snapshot | 一次请求使用的 Table、Query、Mutation Policy 一致快照 |
| Environment | 客户端选择的隔离空间；类型为 `TEST` 或 `PRODUCTION`，多个测试环境共享类型但使用不同编码 |

`Model` 只在页面和发布能力中使用；底层数据治理仍以 `Managed Table` 和 `Table Policy` 为权威语言。

## 3. 总体架构

```text
HTTP QueryPage Request
        |
        v
QueryPage Module
        |
        +--> Page Catalog --------> Page Field / Option / Release Type
        |
        +--> Policy Catalog ------> Policy Snapshot
        |
        +--> Schema Reader -------> Live Table Schema
        |
        +--> Managed Query -------> Count + Page Rows
        |
        v
HTTP QueryPage Response
```

QueryPage 是深模块。它的外部接口只表达页面查询；配置解析、并发加载、SQL 安全和软降级位于实现内部。

## 4. HTTP 接口契约

### 4.1 请求

```http
POST /api/v1/pages/{model_code}:query
Content-Type: application/json
```

```json
{
  "environment": "test-a",
  "mode": "ALL",
  "query": {
    "conditions": [
      {"field": "status", "operator": "exact", "value": "ACTIVE"}
    ],
    "order": {"field": "id", "direction": "DESC"},
    "page_number": 1,
    "page_size": 20
  }
}
```

### 4.2 请求归一化

- `environment` 必填，不提供默认值，且必须对应已启用的环境；
- `mode` 为空时使用 `ALL`。
- `query` 为空时构造空条件 Query Spec。
- 页码为空时使用 1。
- 页大小为空时使用 Query Policy 的默认值。
- 非法枚举、零值、负值或超过 Policy 上限的值直接失败。
- 空字符串是有效查询值，不能按“未提供”处理。

### 4.3 查询模式

`ALL` 返回页面元数据、选项、发布类型和分页数据。`ONLY_DATA` 只解析 Page Model、Policy Snapshot 和实时 Schema，不访问字段、选项或发布配置。

### 4.4 响应

```json
{
  "model": {
    "code": "merchant_channel",
    "name": "Merchant Channel",
    "table_name": "merchant_channel_config"
  },
  "fields": [
    {
      "name": "status",
      "label": "Status",
      "control": "select",
      "query_operator": "exact",
      "required": true,
      "editable": true,
      "options": [
        {"value": "ACTIVE", "label": "Active"}
      ]
    }
  ],
  "release_types": [],
  "data": {
    "columns": [{"name": "id", "type": "uint64", "nullable": false}],
    "rows": [{"id": "42", "status": "ACTIVE"}],
    "page": {
      "page_number": 1,
      "page_size": 20,
      "total_count": 1,
      "total_pages": 1
    }
  },
  "warnings": []
}
```

所有计数使用 64 位语义。空结果保留合法请求页码，并返回 `rows: []`、`total_count: 0`、`total_pages: 0`。

## 5. 配置表设计

### 5.1 `rcc_page_models`

```sql
CREATE TABLE `rcc_page_models` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_code` varchar(100) NOT NULL,
  `name` varchar(100) NOT NULL,
  `description` varchar(500) NOT NULL DEFAULT '',
  `table_name` varchar(64) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_page_model_code` (`model_code`),
  UNIQUE KEY `uk_page_model_table` (`table_name`),
  CONSTRAINT `chk_page_model_code` CHECK (`model_code` REGEXP '^[a-z][a-z0-9_]*$'),
  CONSTRAINT `chk_page_model_enabled` CHECK (`enabled` IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

启用 Page Model 时必须确认目标表存在、不是 `rcc_*` 控制表，并拥有启用的 Table Policy。

### 5.2 `rcc_page_fields`

```sql
CREATE TABLE `rcc_page_fields` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_code` varchar(100) NOT NULL,
  `field_name` varchar(64) NOT NULL,
  `label` varchar(100) NOT NULL,
  `control_type` varchar(32) NOT NULL,
  `query_operator` varchar(32) NULL,
  `visible` tinyint(1) NOT NULL DEFAULT 1,
  `queryable` tinyint(1) NOT NULL DEFAULT 0,
  `editable` tinyint(1) NOT NULL DEFAULT 0,
  `required` tinyint(1) NOT NULL DEFAULT 0,
  `display_order` int NOT NULL DEFAULT 0,
  `default_value` text NULL,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_page_field` (`model_code`, `field_name`),
  KEY `idx_page_field_order` (`model_code`, `display_order`, `id`),
  CONSTRAINT `chk_page_field_flags` CHECK (
    `visible` IN (0,1) AND `queryable` IN (0,1) AND
    `editable` IN (0,1) AND `required` IN (0,1)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

配置发布时逐字段对照实时 Schema；不存在、生成列不可编辑、Query Policy 不支持的操作符均拒绝。

### 5.3 `rcc_page_options`

```sql
CREATE TABLE `rcc_page_options` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_code` varchar(100) NOT NULL,
  `field_name` varchar(64) NOT NULL,
  `source_type` varchar(16) NOT NULL,
  `option_value` varchar(500) NULL,
  `option_label` varchar(500) NULL,
  `source_table` varchar(64) NULL,
  `source_value_field` varchar(64) NULL,
  `source_label_field` varchar(64) NULL,
  `source_filter_field` varchar(64) NULL,
  `source_filter_value` varchar(500) NULL,
  `display_order` int NOT NULL DEFAULT 0,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_page_options` (`model_code`, `field_name`, `enabled`, `display_order`, `id`),
  CONSTRAINT `chk_page_option_source` CHECK (`source_type` IN ('STATIC', 'TABLE')),
  CONSTRAINT `chk_page_option_enabled` CHECK (`enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

`STATIC` 必须提供 value/label，且所有 source 字段为空。`TABLE` 必须提供 source table/value/label，且静态 value/label 为空。应用层在保存时执行该互斥校验。

### 5.4 Table Policy 与发布类型

QueryPage 不再创建独立 allow-list。`rcc_table_policies.enabled=1` 是主表和外部选项表的访问边界。若安装发布模块，发布类型来自 `rcc_release_types`；未安装时返回空数组。

## 6. 动态业务表接入规范

### 6.1 存储位置

业务表必须位于部署配置指定的唯一 MySQL database。Page Model、Table Option 和请求均不得指定 DSN、database 或跨 schema 标识符。

### 6.2 字段要求

- 表必须是普通 base table。
- `id` 必须是唯一的单列主键。
- 不支持 `BLOB`、空间类型等当前 Schema Codec 无法表达的列。
- 新增字段只有在 Page Field 中显式配置后才对 QueryPage 可见。
- Table Option 的 value/label 字段必须存在并可转换为字符串。

### 6.3 示例业务表

```sql
CREATE TABLE `merchant_channel_config` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_id` varchar(64) NOT NULL,
  `channel_code` varchar(64) NOT NULL,
  `status` varchar(32) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_merchant_channel` (`merchant_id`, `channel_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 7. 模块设计

### 7.1 QueryPage 模块

```go
type QueryPageModule interface {
    QueryPage(ctx context.Context, request QueryPageRequest) (QueryPageResponse, error)
}
```

### 7.2 Page Catalog 接缝

```go
type PageCatalog interface {
    ResolveModel(ctx context.Context, modelCode string) (PageModel, error)
    LoadPresentation(ctx context.Context, modelCode string) (PagePresentation, error)
}
```

生产使用 MySQL 适配器，测试使用内存适配器。

### 7.3 Managed Query 接缝

```go
type ManagedQuery interface {
    Query(ctx context.Context, tableName string, spec QuerySpec) (Page, error)
}
```

调用方不接触 SQL、GORM Clause 或实时 Schema 细节。

## 8. 查询执行流程

### 8.1 主流程

```text
normalize request
resolve enabled Page Model
load Policy Snapshot and live Schema
validate visible/queryable fields
run Managed Query
if ONLY_DATA: return data
load Page Presentation
load options with bounded concurrency
load optional release types
assemble response and warnings
```

`ALL` 模式可以并行执行主数据和页面展示读取；同一依赖的调用必须受请求取消和超时控制。

### 8.2 模型校验

模型不存在、未启用、目标 Table Policy 未启用、配置字段与实时 Schema 不一致时均失败。页面配置错误不能绕过 Policy；Policy 拒绝始终优先。

## 9. SQL 生成规则

### 9.1 条件

沿用 `page_query` Policy Type：`exact`、`contains`、`open_range`、`closed_range`、`in`、`not_in`、`is_null`、`is_not_null`。

### 9.2 字段安全

表名只来自启用 Page Model；字段必须同时存在于实时 Schema、Page Field 和 Query Policy 允许集合；值全部使用绑定参数。

### 9.3 类型转换

HTTP 值沿用 JSON String 语义。SQL NULL 使用 JSON `null`，空字符串保持为空字符串。类型错误在执行 SQL 前返回。

### 9.4 分页

页码从 1 开始，Page Size 不超过 Query Policy 的 `max_page_size`，Offset 不超过 10,000。Count 与 Scan 使用同一只读一致性事务和相同条件。

### 9.5 排序

只支持一个真实字段和 `ASC`/`DESC`。未提供时使用 Query Policy 默认排序。为保证稳定结果，非唯一排序字段必须追加 `id` 作为内部次序。

## 10. 返回值转换

只返回 `visible=1` 的字段。每个值按实时 Schema 转为 JSON String；任何不可表达值使整个请求失败，禁止漏列或部分行成功。

## 11. 下拉选项

### 11.1 静态选项

按 `display_order,id` 排序；重复 value 在配置保存时拒绝。

### 11.2 外部表选项

来源表必须拥有启用 Table Policy。查询使用独立只读 Query Spec，不接受 SQL 片段；默认最多 1,000 行、并发度 2、超时 1 秒，并按 value/label 稳定排序。

### 11.3 失败策略

主数据失败时整个请求失败。单个外部选项失败时 `ALL` 请求仍可成功，但对应字段 options 为空，并在 `warnings` 中返回稳定错误种类；静态配置错误在保存或启用阶段拒绝，不允许运行时软降级。

## 12. 错误契约

| HTTP | code | 语义 |
|---:|---|---|
| 400 | `INVALID_ARGUMENT` | 非法模式、分页、条件或字段 |
| 404 | `PAGE_MODEL_NOT_FOUND` | 模型不存在 |
| 409 | `PAGE_MODEL_DISABLED` | 模型或 Table Policy 未启用 |
| 422 | `PAGE_CONFIG_INVALID` | 页面配置与 Schema/Policy 不一致 |
| 500 | `INTERNAL` | 未分类内部错误 |
| 503 | `DEPENDENCY_UNAVAILABLE` | MySQL 或可选依赖不可用 |

错误响应包含 `code`、`message`、`request_id`，不得暴露 SQL、DSN 或数据库错误原文。

## 13. 性能设计

- Page Model 与 Page Presentation 可按修改时间做短 TTL 缓存；Policy Snapshot 仍按当前 Admin 规则读取。
- 实时 Schema 缓存必须有短 TTL 和显式失效能力。
- 外部选项限制行数、并发和响应字节数。
- 高频条件和排序字段由运维为业务表建立索引，Admin 不自动执行 DDL。
- 指标标签禁止直接使用任意 table/model 值而不做基数治理。

## 14. 安全约束

1. 所有表必须命中启用 Table Policy。
2. `rcc_*` 控制表永远不能成为 Page Model 或 Table Option 来源。
3. 页面字段是返回白名单，不使用 `SELECT *` 后再过滤。
4. 配置字段名和表名通过实时 Schema 验证。
5. 禁止原始 SQL、表达式、任意 URL 和跨 schema 标识符。
6. Admin 使用反向代理注入的可信身份和内置 RBAC。
7. 日志不得记录查询值、整行业务数据或敏感选项。

## 15. 可观测性

至少提供：

```text
rcc_query_page_requests_total{mode,result}
rcc_query_page_duration_seconds{mode}
rcc_query_page_rows_returned
rcc_query_page_option_requests_total{source,result}
rcc_query_page_option_degraded_total{reason}
rcc_query_page_policy_rejected_total{reason}
```

日志包含 request_id、principal、model_code、table_name、page_size、condition_count、returned_rows、warning_count、duration_ms 和 error_code。

## 16. 测试方案

### 16.1 请求

覆盖 environment 缺失、禁用、未知和跨环境访问，以及 nil/空 JSON、非法 mode、默认分页、上限、空字符串、NULL 和未知操作符。

### 16.2 模型配置

覆盖模型不存在/禁用、Table Policy 禁用、字段漂移、受保护表、重复字段和非法控制类型。

### 16.3 条件

覆盖所有操作符、通配符转义、集合上限、范围边界、类型转换和 SQL 注入输入。

### 16.4 分页

覆盖空数据、超出总页数、64 位总数、稳定排序、Count/Scan 一致快照和 Offset 上限。

### 16.5 页面元数据

覆盖字段顺序、可见/可查询差异、默认值和发布模块未安装。

### 16.6 选项

覆盖静态/外部、重复值、超时、行数上限、并发度、来源 Policy 禁用和稳定顺序。

### 16.7 安全

覆盖受保护表、非法标识符、未授权字段、敏感日志和反向代理身份缺失。

## 17. 实施步骤

### 阶段一：目录与只读能力

创建页面三张控制表、管理接口和校验器；接入现有 Managed Query。

### 阶段二：QueryPage

实现 `ONLY_DATA`，再实现 `ALL`、静态选项和外部选项。

### 阶段三：安全与性能

补齐字段白名单、缓存、超时、指标、日志和容量限制。

### 阶段四：发布集成

发布模块可用后只增加 Release Type 展示，不让 QueryPage 承担发布状态机。

## 18. 验收标准

- QueryPage 不接受物理表名作为请求参数。
- `ONLY_DATA` 不读取页面展示和选项配置。
- 所有数据访问经过 Policy Snapshot 和实时 Schema。
- 新增业务字段不会自动暴露。
- Count 与分页数据来自同一一致性事务。
- 外部选项失败不会污染主数据结果。
- 相同 `model_code` 在不同测试环境中的查询结果相互隔离，任何请求都不能回退到生产环境。
- 任意配置错误、非法字段或 SQL 注入输入均不能进入 SQL 标识符。
- 本文测试矩阵、MySQL 8.4 集成测试和 race test 全部通过。
