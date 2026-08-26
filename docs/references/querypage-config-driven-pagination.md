# QueryPage 配置驱动分页查询技术方案

本文是一份可脱离现有源码独立实施的技术方案。它完整定义：

- 功能范围和行为契约
- RPC 请求与响应
- 依赖的四张配置表及完整 DDL
- 配置字段和 JSON 子结构
- 动态业务表的接入约束
- 模块接口和调用流程
- 动态 SQL 生成规则
- 静态/外部下拉选项机制
- 分页、错误、并发、安全和可观测性
- 测试与上线方案

表结构来自 2026-08-25 对 `pcs_ca_center` 的只读实时结构查询；DDL 中省略了会持续变化的当前 `AUTO_INCREMENT` 序列值。

---

## 1. 建设目标

建设一个由 `ModelCode` 驱动的通用运营页面查询能力。

调用方无需知道目标数据库表，只需要传入：

- 模型编码
- 查询条件
- 页码和页大小
- 查询模式

模块根据模型配置自动完成：

1. 定位目标业务表。
2. 查询页面名称和发布类型。
3. 查询前端字段定义。
4. 查询下拉框静态选项和外部表选项。
5. 校验目标表白名单。
6. 基于目标表真实结构构造 SQL。
7. 执行 Count 和分页查询。
8. 将数据库字段统一转换为字符串。
9. 返回页面元数据和分页数据。

### 1.1 功能范围

第一版明确支持：

- 单表分页查询
- 动态查询条件
- 动态排序
- 动态字段交互信息
- 静态下拉选项
- 外部表下拉选项
- `ALL` 和 `ONLY_DATA` 两种查询模式

第一版暂不支持：

- JOIN 查询
- 子查询
- 发布单信息合并
- 灰度表数据合并
- 按 `QueryStrategyName` 分发到特殊查询策略

如果模型配置包含 JOIN 或特殊查询策略，应在配置发布阶段拒绝将其接入该页面查询模块。

---

## 2. 核心术语

| 术语 | 定义 |
|---|---|
| 模型 | 一类可运营业务数据，由 `model_code` 唯一标识 |
| 业务表 | 模型对应的实际 MySQL 表 |
| 字段配置 | 字段名称、查询方式、前端控件和编辑规则 |
| 静态选项 | 直接写在选项配置表里的 code/name |
| 外部选项 | 通过配置指定另一张业务表，动态查询得到的 code/name |
| 查询模式 | `ALL` 或 `ONLY_DATA` |
| 运营白名单 | 允许被通用查询模块访问的数据库表集合 |
| 订阅键 | FinConfig 客户端查询配置时使用的组合键 |

---

## 3. 总体架构

```text
QueryPageRequest
       │
       ▼
请求校验与默认值归一化
       │
       ▼
按 ModelCode 加载模型配置
       │
       ├─────────────────────────────┐
       ▼                             ▼
分页业务数据查询                  页面元数据查询（ALL）
       │                             │
       │                             ├─ 字段配置
       │                             ├─ 发布类型
       │                             ├─ 静态选项
       │                             └─ 外部表选项
       │
       └──────────────┬──────────────┘
                      ▼
                 组装页面结果
                      │
                      ▼
              QueryPageResponse
```

运行时依赖：

```text
FinConfig
├─ channel_operation_model_config
├─ channel_operation_model_field_config
├─ channel_operation_option_config
└─ channel_operation_allow_list

Decision MySQL Read DB
├─ 模型对应的主业务表
└─ 外部下拉选项来源表
```

---

# 4. RPC 接口契约

## 4.1 请求

```thrift
struct PageRequest {
    1: optional i32 PageSize
    2: optional i32 PageNumber
}

struct SearchCondition {
    1: required string Type
    2: required string Value
}

struct QueryPageRequest {
    2: required string ModelCode
    3: optional map<string, SearchCondition> Condition
    5: optional PageRequest PageRequest
    6: optional string Desc
    7: required string QueryType
}
```

字段语义：

| 字段 | 必填 | 说明 |
|---|---:|---|
| `ModelCode` | 是 | 模型编码，对应 `channel_operation_model_config.model_code` |
| `Condition` | 否 | 查询条件；key 为业务表物理列名 |
| `PageRequest` | 否 | 分页参数 |
| `Desc` | 否 | 仅供前端区分场景，后端不消费 |
| `QueryType` | 是 | `ALL` 或 `ONLY_DATA` |

## 4.2 请求归一化规则

虽然当前 IDL 将分页字段声明为 optional，但实现必须显式补默认值：

| 参数 | 缺省值 | 合法范围 |
|---|---:|---|
| `PageNumber` | 1 | `>= 1` |
| `PageSize` | 10 | `1～200` |
| `QueryType` | 无 | 只能为 `ALL` 或 `ONLY_DATA` |

校验规则：

- 请求不得为空。
- `ModelCode` 去除首尾空格后不得为空。
- `Condition` 中的 value 不得是 nil。
- `Condition.Type` 必须是支持的枚举。
- `PageSize` 超过上限时返回参数错误，不能直接执行大页查询。
- `Desc` 不参与任何查询逻辑。

## 4.3 查询模式

### `ALL`

返回：

- 模型编码和名称
- 字段交互信息
- 下拉选项
- 发布类型
- 分页业务数据
- 分页信息

### `ONLY_DATA`

返回：

- 模型编码和名称
- 分页业务数据
- 分页信息

不读取：

- 字段配置
- 静态选项
- 外部选项
- 发布类型

该模式用于服务端内部查询，例如按唯一条件读取当前记录。

## 4.4 响应

```thrift
struct QueryPageResponse {
    2: required string ModelCode
    3: optional list<InteractionInfo> InteractionInfoList
    4: optional string Data
    5: optional list<ReleaseTypeOption> ReleaseTypeList
    6: required string ModelName
    253: optional PageResponse PageResponse
    254: required OperationBaseResponse OperationBaseResponse
}
```

`Data` 为 JSON 字符串，而非直接的 Thrift list：

```json
[
  {
    "id": "1001",
    "code": "DEMO_001",
    "name": "示例配置",
    "is_deleted": "0",
    "gmt_created": "2026-08-25 10:00:00"
  }
]
```

成功响应：

```json
{
  "ModelCode": "DemoModel",
  "ModelName": "示例模型",
  "Data": "[{\"id\":\"1001\",\"code\":\"DEMO_001\"}]",
  "PageResponse": {
    "TotalNumber": 1,
    "TotalPage": 1,
    "PageSize": 10,
    "PageNumber": 1
  },
  "OperationBaseResponse": {
    "RetCode": "CA000000",
    "RetMsg": "操作成功",
    "RetStatus": "SUCCESS"
  }
}
```

---

# 5. 配置表设计

## 5.1 `channel_operation_model_config`

### 5.1.1 用途

定义运营模型的核心信息：

- 模型编码和名称
- 目标业务表
- 默认排序
- 查询策略
- 变更策略
- 字段映射
- 发布类型
- 唯一键和描述字段

`QueryPage` 查询时以 `model_code` 作为订阅键。

### 5.1.2 现网表结构

```sql
CREATE TABLE `channel_operation_model_config` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `creator` varchar(64) DEFAULT NULL COMMENT '创建人',
  `modifier` varchar(64) DEFAULT NULL COMMENT '修改人',
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP COMMENT '修改时间',
  `model_code` varchar(100) NOT NULL COMMENT '模型编码 (业务主键)',
  `model_name` varchar(200) NOT NULL COMMENT '模型名称 (显示名)',
  `query_strategy_name` varchar(100) DEFAULT NULL COMMENT '查询策略名称',
  `change_strategy_name` varchar(100) DEFAULT NULL COMMENT '变更策略名称',
  `query_strategy_config` text COMMENT '查询策略详细配置(JSON)',
  `change_strategy_config` text COMMENT '变更策略详细配置(JSON)',
  `field_infos` text COMMENT '字段定义列表(JSON)',
  `auto_fill_infos` text COMMENT '自动填充信息',
  `extra_info` text COMMENT '扩展信息',
  `release_type_list` text COMMENT '发布类型列表(JSON)',
  `unique_key_infos` text COMMENT '唯一性约束字段(JSON)',
  `unique_desc_infos` text COMMENT '唯一性约束描述(JSON)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_model_code` (`model_code`) COMMENT '模型编码唯一索引'
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COMMENT='运营对象模型元数据配置表';
```

### 5.1.3 字段消费关系

| 字段 | QueryPage 是否消费 | 用途 |
|---|---:|---|
| `model_code` | 是 | 查询配置、返回模型编码 |
| `model_name` | 是 | 页面显示名称 |
| `query_strategy_name` | 当前否 | 当前 QueryPage 固定走单表分页 |
| `change_strategy_name` | 否 | 变更链路使用 |
| `query_strategy_config` | 是 | 提取业务表和默认排序 |
| `change_strategy_config` | 否 | 变更链路使用 |
| `field_infos` | 当前否 | 通用 Query 的字段映射使用 |
| `auto_fill_infos` | 否 | 变更链路使用 |
| `extra_info` | 否 | 扩展配置 |
| `release_type_list` | `ALL` 使用 | 页面发布方式 |
| `unique_key_infos` | 否 | 发布和唯一性校验使用 |
| `unique_desc_infos` | 否 | 发布目标描述使用 |

### 5.1.4 `query_strategy_config`

线上实际使用 PascalCase 键名：

```json
{
  "BaseTableName": "demo_business_config",
  "Joins": [],
  "OrderInfo": {
    "OrderBy": "id",
    "OrderType": "DESC"
  }
}
```

结构定义：

```go
type QueryStrategyConfig struct {
    BaseTableName string
    Joins         []JoinClause
    OrderInfo     OrderInfo
}

type OrderInfo struct {
    OrderBy   string
    OrderType string
}

type JoinClause struct {
    Table          string
    OnClause       string
    Type            string
    IsSubQuery      bool
    SubQuerySource  string
    SubQueryAlias   string
    SubQueryFields  []string
}
```

本方案第一版只消费：

- `BaseTableName`
- `OrderInfo.OrderBy`
- `OrderInfo.OrderType`

`Joins` 非空时应拒绝接入，而不能静默忽略。

### 5.1.5 `change_strategy_config`

线上键名：

```json
{
  "TableName": "demo_business_config",
  "PrimaryKeys": ["id"],
  "ChangeMatchFields": ["code"]
}
```

该字段不参与页面查询，但模型配置解析器应允许它为空：

```go
type ChangeStrategyConfig struct {
    TableName         string
    PrimaryKeys       []string
    ChangeMatchFields []string
}
```

### 5.1.6 `field_infos`

线上键名：

```json
[
  {
    "DisplayName": "code",
    "QueryCondName": "code",
    "QueryResultName": "code"
  }
]
```

结构：

```go
type FieldInfo struct {
    DisplayName       string
    QueryCondName     string
    QueryResultName   string
    ChangeContentName string
}
```

当前 QueryPage 不消费该字段，所以主查询条件和返回字段使用数据库物理列名。

### 5.1.7 `release_type_list`

```json
[
  {
    "Code": "DB_RELEASE",
    "Name": "DB发布"
  },
  {
    "Code": "EMERGENCY_RELEASE",
    "Name": "应急发布"
  }
]
```

转换为：

```json
[
  {
    "ReleaseTypeCode": "DB_RELEASE",
    "ReleaseTypeName": "DB发布"
  }
]
```

### 5.1.8 建议配置示例

```sql
INSERT INTO channel_operation_model_config (
    creator,
    modifier,
    model_code,
    model_name,
    query_strategy_name,
    query_strategy_config,
    release_type_list
) VALUES (
    'system',
    'system',
    'DemoModel',
    '示例模型',
    'DBPaginationQueryStrategy',
    '{
      "BaseTableName":"demo_business_config",
      "Joins":[],
      "OrderInfo":{"OrderBy":"id","OrderType":"DESC"}
    }',
    '[
      {"Code":"DB_RELEASE","Name":"DB发布"}
    ]'
);
```

---

## 5.2 `channel_operation_model_field_config`

### 5.2.1 用途

定义模型字段如何展示和交互：

- 字段编码和名称
- 查询匹配方式
- 前端控件
- 是否可查
- 是否可编辑
- 是否必填
- 默认值
- 显示顺序

查询订阅键：

```text
model_code,is_deleted
```

查询值：

```text
<ModelCode>,0
```

### 5.2.2 现网表结构

```sql
CREATE TABLE `channel_operation_model_field_config` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键id',
  `gmt_create` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `gmt_modified` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `creator` varchar(255) NOT NULL COMMENT '创建人',
  `modifier` varchar(255) NOT NULL COMMENT '修改人',
  `model_code` varchar(100) NOT NULL COMMENT '模型编码',
  `field_code` varchar(100) NOT NULL COMMENT '字段编码',
  `field_name` varchar(200) NOT NULL COMMENT '字段名称',
  `match_type` varchar(100) NOT NULL COMMENT '查询匹配类型',
  `is_queryable` tinyint(3) unsigned NOT NULL DEFAULT '0'
      COMMENT '是否可查询：0-否，1-是',
  `is_deleted` tinyint(3) unsigned NOT NULL DEFAULT '0'
      COMMENT '是否已删除：0-否，1-是',
  `display_order` int(11) NOT NULL DEFAULT '0' COMMENT '顺序',
  `ui_type` varchar(64) DEFAULT NULL
      COMMENT '前端控件类型，如 input, select, textarea 等',
  `is_editable` tinyint(3) unsigned NOT NULL DEFAULT '1'
      COMMENT '是否可输入：0-否，1-是',
  `default_value` varchar(1000) DEFAULT '' COMMENT '默认值',
  `is_required` tinyint(3) unsigned NOT NULL DEFAULT '0'
      COMMENT '是否必填',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_model_field` (`model_code`,`field_code`),
  KEY `idx_model_code` (`model_code`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COMMENT='运营平台模型字段表';
```

注意数据库原字段名是 `gmt_create`，不是 `gmt_created`。

### 5.2.3 字段协议

| 字段 | 说明 |
|---|---|
| `model_code` | 所属模型 |
| `field_code` | 字段编码；当前实现要求等于业务表物理列名 |
| `field_name` | 前端展示名称 |
| `match_type` | 前端生成查询条件时使用的匹配方式 |
| `is_queryable` | 是否展示为查询条件 |
| `display_order` | 升序排列 |
| `ui_type` | `input/select/textarea/time` 等 |
| `is_editable` | 是否可在变更表单中输入 |
| `default_value` | 默认值 |
| `is_required` | 是否必填 |
| `is_deleted` | 软删除标记 |

### 5.2.4 返回映射

```text
field_code    -> InteractionInfo.FieldCode
field_name    -> InteractionInfo.FieldName
ui_type       -> InteractionInfo.UiType
match_type    -> InteractionInfo.MatchType
is_editable   -> InteractionInfo.IsEditable
is_queryable  -> InteractionInfo.IsQueryable
default_value -> InteractionInfo.DefaultValue
is_required   -> InteractionInfo.IsRequired
```

布尔/数字字段通过 FinConfig 读取后以字符串形式返回，例如 `"0"`、`"1"`。

### 5.2.5 示例

```sql
INSERT INTO channel_operation_model_field_config (
    creator,
    modifier,
    model_code,
    field_code,
    field_name,
    match_type,
    is_queryable,
    is_deleted,
    display_order,
    ui_type,
    is_editable,
    default_value,
    is_required
) VALUES (
    'system',
    'system',
    'DemoModel',
    'status',
    '状态',
    'exact',
    1,
    0,
    10,
    'select',
    1,
    '',
    1
);
```

---

## 5.3 `channel_operation_option_config`

### 5.3.1 用途

为 `ui_type=select` 的字段配置选项。

关联关系：

```text
option_config.config_group = model_field_config.model_code
option_config.config_key   = model_field_config.field_code
```

查询订阅键：

```text
config_group,value_type,is_deleted
```

### 5.3.2 现网表结构

```sql
CREATE TABLE `channel_operation_option_config` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键id',
  `gmt_create` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `gmt_modified` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `creator` varchar(255) NOT NULL COMMENT '创建人',
  `modifier` varchar(255) NOT NULL COMMENT '修改人',
  `config_group` varchar(100) NOT NULL COMMENT '选项配置分组',
  `config_key` varchar(100) NOT NULL COMMENT '选项配置key',
  `value_type` varchar(100) NOT NULL COMMENT '选项配置value类型',
  `value` varchar(1000) NOT NULL COMMENT '选项配置value值',
  `is_deleted` tinyint(3) unsigned NOT NULL DEFAULT '0'
      COMMENT '是否已删除：0-否，1-是',
  `value_name` varchar(1000) DEFAULT NULL COMMENT '值名称',
  `uk_group_key_value_sha256` varchar(64)
      GENERATED ALWAYS AS (
        sha2(
          concat(
            char_length(`config_group`), ':', `config_group`, '|',
            char_length(`config_key`), ':', `config_key`, '|',
            char_length(`value`), ':', `value`
          ),
          256
        )
      ) STORED
      COMMENT 'config_group+config_key+value 唯一性哈希',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_group_key_value` (`uk_group_key_value_sha256`),
  KEY `idx_group` (`config_group`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COMMENT='运营平台选项配置';
```

### 5.3.3 静态选项

`value_type`：

```text
OptionSourceFromConfig
```

字段映射：

```text
config_group -> ModelCode
config_key   -> FieldCode
value        -> OptionCode
value_name   -> OptionName
```

示例：

```sql
INSERT INTO channel_operation_option_config (
    creator,
    modifier,
    config_group,
    config_key,
    value_type,
    value,
    value_name,
    is_deleted
) VALUES
(
    'system',
    'system',
    'DemoModel',
    'status',
    'OptionSourceFromConfig',
    'ENABLED',
    '启用',
    0
),
(
    'system',
    'system',
    'DemoModel',
    'status',
    'OptionSourceFromConfig',
    'DISABLED',
    '停用',
    0
);
```

### 5.3.4 外部表选项

代码协议支持：

```text
OptionSourceFromExternalTable
```

其 `value` 是 JSON：

```json
{
  "table_name": "demo_status_dictionary",
  "query_option": {
    "is_deleted": {
      "Type": "exact",
      "Value": "0"
    }
  },
  "field_key": "status_code",
  "field_name": "status_name"
}
```

含义：

| JSON 字段 | 说明 |
|---|---|
| `table_name` | 外部选项来源表 |
| `query_option` | 查询条件 |
| `field_key` | 作为 `OptionCode` 的列 |
| `field_name` | 作为 `OptionName` 的列 |

示例配置：

```sql
INSERT INTO channel_operation_option_config (
    creator,
    modifier,
    config_group,
    config_key,
    value_type,
    value,
    value_name,
    is_deleted
) VALUES (
    'system',
    'system',
    'DemoModel',
    'status',
    'OptionSourceFromExternalTable',
    '{
      "table_name":"demo_status_dictionary",
      "query_option":{
        "is_deleted":{"Type":"exact","Value":"0"}
      },
      "field_key":"status_code",
      "field_name":"status_name"
    }',
    '状态字典',
    0
);
```

现网当前活跃数据中未观察到 `OptionSourceFromExternalTable` 类型记录，但运行时代码已经实现该协议。因此它属于“代码支持、当前配置未启用”的能力。

### 5.3.5 当前唯一键的限制

唯一键只包含：

```text
config_group + config_key + value
```

不包含：

- `value_type`
- `is_deleted`

因此：

- 同一个 group/key/value 不能同时配置成不同来源类型。
- 软删除后不能插入完全相同的记录，只能恢复或更新原记录。
- 外部 JSON 内容的任何变化都会生成新的 hash。

实施时必须按照这个约束管理数据。

---

## 5.4 `channel_operation_allow_list`

### 5.4.1 用途

限制通用数据库模块可以访问哪些表。

查询 `QueryPage` 主业务表和外部选项表前，都必须命中：

```text
allow_type = operation_allow_list
allow_key  = <table_name>
```

### 5.4.2 现网表结构

```sql
CREATE TABLE `channel_operation_allow_list` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `creator` varchar(64) DEFAULT 'system' COMMENT '创建人',
  `modifier` varchar(64) DEFAULT 'system' COMMENT '修改人',
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP COMMENT '修改时间',
  `allow_type` varchar(100) NOT NULL COMMENT '允许名单类型',
  `allow_key` varchar(500) NOT NULL COMMENT '允许名单',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_type_key` (`allow_type`,`allow_key`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COMMENT='运营允许表';
```

### 5.4.3 白名单类型

现网存在两种：

| `allow_type` | 用途 |
|---|---|
| `operation_allow_list` | 允许通用查询或变更访问该表 |
| `whole_table_query_allow_list` | 允许无条件全表查询 |

主业务表接入示例：

```sql
INSERT INTO channel_operation_allow_list (
    creator,
    modifier,
    allow_type,
    allow_key
) VALUES (
    'system',
    'system',
    'operation_allow_list',
    'demo_business_config'
);
```

外部选项表如果允许空条件查询，除了 `operation_allow_list`，还应加入：

```sql
INSERT INTO channel_operation_allow_list (
    creator,
    modifier,
    allow_type,
    allow_key
) VALUES (
    'system',
    'system',
    'whole_table_query_allow_list',
    'demo_status_dictionary'
);
```

---

# 6. 动态业务表接入规范

由于 QueryPage 可以查询任意业务表，不可能提供一份统一业务表 DDL，但所有接入表必须满足以下契约。

## 6.1 存储位置

- 表位于 Decision MySQL 集群。
- 查询使用只读实例。
- 表名必须出现在 `channel_operation_allow_list` 中。

## 6.2 字段要求

- `field_code` 必须对应真实物理列。
- `OrderInfo.OrderBy` 必须对应真实物理列。
- 外部选项的 `field_key` 和 `field_name` 必须对应真实物理列。
- 建议包含稳定主键。
- 建议默认排序字段具有索引。
- 高频查询字段应建立索引。
- 不允许把敏感列无条件暴露给通用页面查询。

## 6.3 示例业务表

```sql
CREATE TABLE `demo_business_config` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(100) NOT NULL COMMENT '配置编码',
  `name` varchar(200) NOT NULL COMMENT '配置名称',
  `status` varchar(32) NOT NULL COMMENT '状态',
  `is_deleted` tinyint unsigned NOT NULL DEFAULT 0,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
      ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_status_deleted` (`status`,`is_deleted`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COMMENT='示例运营业务表';
```

---

# 7. 模块设计

## 7.1 对外模块

```go
type PageQueryModule interface {
    Query(
        ctx context.Context,
        req PageQueryRequest,
    ) (*PageQueryResult, error)
}
```

## 7.2 配置目录接口

```go
type PageConfigCatalog interface {
    GetModel(
        ctx context.Context,
        modelCode string,
    ) (*ModelMeta, error)

    ListFields(
        ctx context.Context,
        modelCode string,
    ) ([]FieldMeta, error)

    ListOptions(
        ctx context.Context,
        modelCode string,
        source OptionSource,
    ) ([]OptionMeta, error)

    IsTableAllowed(
        ctx context.Context,
        allowType string,
        tableName string,
    ) (bool, error)
}
```

生产适配器通过 FinConfig subscription client 实现；测试使用内存 fake。

## 7.3 表读取接口

```go
type TableReader interface {
    Describe(
        ctx context.Context,
        tableName string,
    ) (TableSchema, error)

    Page(
        ctx context.Context,
        req TablePageQuery,
    ) (*PageRows, error)

    List(
        ctx context.Context,
        req TableListQuery,
    ) ([]Row, error)
}
```

生产适配器使用 Decision Read DB；测试使用内存适配器或 SQL mock。

---

# 8. 查询执行流程

## 8.1 主流程伪代码

```go
func (m *pageQueryModule) Query(
    ctx context.Context,
    input PageQueryRequest,
) (*PageQueryResult, error) {
    req, err := normalizeAndValidate(input)
    if err != nil {
        return nil, parameterError(err)
    }

    model, err := m.catalog.GetModel(ctx, req.ModelCode)
    if err != nil {
        return nil, configError(err)
    }

    if err := validateModel(model); err != nil {
        return nil, configError(err)
    }

    var (
        pageData   *PageRows
        pageMeta   *PageMeta
        dataErr    error
        metadataErr error
    )

    // 数据查询与页面元数据可在模型配置加载完成后并行。
    runParallel(
        func() {
            pageData, dataErr = m.queryPageData(ctx, req, model)
        },
        func() {
            if req.QueryType == QueryTypeAll {
                pageMeta, metadataErr = m.queryPageMeta(ctx, model)
            }
        },
    )

    if dataErr != nil {
        return nil, dataErr
    }
    if metadataErr != nil {
        return nil, metadataErr
    }

    return assembleResult(model, pageMeta, pageData), nil
}
```

## 8.2 模型校验

必须校验：

- 模型存在。
- 配置中的 `model_code` 与请求一致。
- `BaseTableName` 非空。
- `Joins` 为空。
- 目标表命中 `operation_allow_list`。
- `OrderType` 为 `ASC` 或 `DESC`。
- `OrderBy` 存在于目标表结构。
- `release_type_list` 能正确解析。

---

# 9. SQL 生成规则

## 9.1 支持的条件

| Type | SQL |
|---|---|
| `exact` | `` `field` = ? `` |
| `like` | `` `field` LIKE ? ``，参数为 `%value%` |
| `close_range` | `` `field` >= ? AND `field` <= ? `` |
| `open_range` | `` `field` > ? AND `field` < ? `` |
| `in` | `` `field` IN (?) `` |
| `not_in` | `` `field` NOT IN (?) `` |

未知 Type 必须返回参数错误，不能静默忽略。

## 9.2 字段安全

对每个条件：

1. 从真实表结构查找字段。
2. 不存在则返回参数错误。
3. 字段名只能由表结构提供，不接受任意 SQL 表达式。
4. 使用反引号引用字段名。
5. 所有值使用占位参数绑定。

## 9.3 值类型转换

根据 MySQL 列类型转换：

| MySQL 类型 | 转换 |
|---|---|
| tinyint/smallint/int/bigint | `int64` |
| tinyint(1) | bool 或整数布尔 |
| float/double/decimal | `float64` 或 decimal string |
| char/varchar/text/enum/set | string |
| date/time/datetime/timestamp/year | time |
| 其他 | 原字符串 |

`close_range/open_range` 的值：

```json
{
  "from": "2026-01-01 00:00:00",
  "to": "2026-01-31 23:59:59"
}
```

`in/not_in` 的值：

```json
["A", "B", "C"]
```

## 9.4 分页算法

正确顺序：

```text
Count
  → 计算 TotalPage
  → 归一化 PageNumber
  → 计算 Offset
  → 执行分页 Select
```

计算公式：

```go
totalPage := (total + pageSize - 1) / pageSize

if total == 0 {
    pageNumber = 1
    totalPage = 0
} else if pageNumber > totalPage {
    pageNumber = totalPage
}

offset := (pageNumber - 1) * pageSize
```

必须使用归一化后的页码计算 SQL offset，避免响应页码与数据不一致。

## 9.5 排序

```sql
ORDER BY `configured_order_field` ASC|DESC
```

要求：

- `OrderBy` 必须存在于表结构。
- `OrderType` 转为大写。
- 只能是 `ASC` 或 `DESC`。
- 未配置排序时，建议回退到主键升序，保证分页稳定。
- 不允许直接拼接未校验字符串。

---

# 10. 返回值转换

数据库行统一转换成 `map[string]string`。

| 原值类型 | 返回值 |
|---|---|
| NULL | `""` |
| `time.Time` | `2006-01-02 15:04:05` |
| bool | `true/false` |
| 有符号/无符号整数 | 十进制字符串 |
| 浮点数 | 建议保留原始有效精度 |
| string | 原值 |
| `[]byte` | `string(v)` |
| `fmt.Stringer` | `v.String()` |
| 其他 | `fmt.Sprintf("%v")` |

为了兼容现有调用方，第一阶段保留“NULL 转空字符串”的行为。

应修正两个现有表现：

- 不再用 `%f` 强制浮点数补六位小数。
- `[]byte` 应转换成字符串，不能返回 `[49 50 51]`。

---

# 11. 下拉选项处理

## 11.1 静态选项

处理顺序：

1. 查询 `OptionSourceFromConfig`。
2. 按 `config_key` 分组。
3. `value` 转成 `OptionCode`。
4. `value_name` 转成 `OptionName`。
5. 空 code 和空 name 的记录忽略。

## 11.2 外部选项

处理流程：

1. 查询 `OptionSourceFromExternalTable` 配置。
2. 解析 JSON。
3. 校验目标表白名单。
4. 校验 `field_key/field_name` 存在。
5. 只查询这两个字段，避免读取整表所有列。
6. 应用 `query_option`。
7. 限制最大返回行数，例如 1000。
8. 将值统一转成字符串。
9. 按 `(OptionCode, OptionName)` 去重。
10. 按稳定规则排序。

并发约束：

```text
最大外部表并发查询数 = 2
```

## 11.3 失败策略

| 失败 | 策略 |
|---|---|
| 选项配置表整体查询失败 | 页面失败 |
| 单条外部配置 JSON 非法 | 软降级，记录错误 |
| 外部表不在白名单 | 软降级或配置错误告警 |
| 单个外部表查询失败 | 软降级 |
| 字段不存在 | 软降级 |
| 返回行数超过上限 | 截断并告警 |
| goroutine panic | 恢复、记录、继续其他配置 |

静态选项先加入，外部选项后加入，然后统一去重和稳定排序。

---

# 12. 错误契约

| 类别 | 错误码 | 示例 |
|---|---|---|
| 成功 | `CA000000` | 正常返回 |
| 参数错误 | `CA020002` | 页大小非法、未知 QueryType、非法条件 |
| 内部错误 | `CA010001` | FinConfig 或数据库不可用 |
| 配置错误 | 初期可映射 `CA010001` | 模型不存在、JSON 非法、表未进白名单 |

建议内部错误类型：

```go
type ErrorKind string

const (
    ErrorParameter      ErrorKind = "PARAMETER"
    ErrorModelNotFound  ErrorKind = "MODEL_NOT_FOUND"
    ErrorConfigInvalid  ErrorKind = "CONFIG_INVALID"
    ErrorTableForbidden ErrorKind = "TABLE_FORBIDDEN"
    ErrorSchema         ErrorKind = "SCHEMA_ERROR"
    ErrorDatabase       ErrorKind = "DATABASE_ERROR"
    ErrorDependency     ErrorKind = "DEPENDENCY_ERROR"
)
```

对外错误码可以保持兼容，内部必须保留明确分类。

---

# 13. 性能设计

## 13.1 调用数量

一次 `ALL` 查询包含：

- 模型配置查询：1 次
- 字段配置查询：1 次
- 静态选项查询：1 次
- 外部选项配置查询：1 次
- 主表结构查询：1 次
- 主表 Count：1 次
- 主表分页 Select：1 次
- 每条外部配置：
  - 表结构查询：1 次
  - Select：1 次

## 13.2 优化措施

- 模型加载完成后，并行查询主数据和页面元数据。
- 外部选项最大并发度 2。
- 表结构可按 `database + table` 做短时缓存。
- Schema 缓存必须支持 DDL 后失效或短 TTL。
- PageSize 设置硬上限。
- 外部选项设置最大行数。
- Count 和 Select 使用同一套条件。
- 排序字段建立索引。
- 高频条件字段建立组合索引。

## 13.3 配置表索引建议

现网结构可以运行，但与订阅键并不完全一致。数据量增大后建议补充：

```sql
ALTER TABLE channel_operation_model_field_config
ADD KEY idx_model_deleted (`model_code`, `is_deleted`);

ALTER TABLE channel_operation_option_config
ADD KEY idx_group_type_deleted (
    `config_group`,
    `value_type`,
    `is_deleted`
);
```

这两项属于优化建议，不是当前现网已有索引。

---

# 14. 安全约束

必须同时落实：

1. 业务表必须命中 `operation_allow_list`。
2. 无条件外部选项查询必须命中 `whole_table_query_allow_list`。
3. 条件字段必须存在于真实 Schema。
4. 推荐进一步限制为 `is_queryable=1` 的字段。
5. 返回字段应由字段配置白名单控制。
6. 排序字段必须存在于 Schema。
7. 排序方向只能为 `ASC/DESC`。
8. 查询值全部使用参数绑定。
9. 外部选项配置不能携带原始 SQL。
10. `OnClause`、SQL 片段类配置在本单表方案中一律拒绝。
11. 日志不得打印敏感字段完整值。
12. 新增业务表列时，应审查它是否会被通用页面返回。

---

# 15. 可观测性

建议指标：

```text
query_page_request_total{model_code,query_type,status}
query_page_latency_ms{model_code,query_type}
query_page_config_latency_ms{config_type}
query_page_db_count_latency_ms{table}
query_page_db_select_latency_ms{table}
query_page_rows_returned{model_code}
query_page_external_option_latency_ms{table,field_code}
query_page_external_option_error_total{stage}
query_page_invalid_condition_total{model_code,type}
query_page_table_forbidden_total{table}
```

结构化日志字段：

```text
request_id
model_code
query_type
base_table
page_number
page_size
condition_fields
total_rows
returned_rows
external_option_count
degraded_option_count
error_kind
duration_ms
```

禁止记录：

- 完整上下文对象
- 敏感查询值
- 整行业务数据
- 未脱敏的外部选项配置

---

# 16. 测试方案

## 16.1 请求测试

- nil 请求
- ModelCode 为空
- QueryType 为空或非法
- PageRequest 为空
- PageSize/PageNumber 单独为空
- PageSize 为 0、负数、超过上限
- Condition value 为 nil
- 未知条件 Type

## 16.2 模型配置测试

- 模型存在
- 模型不存在
- `query_strategy_config` 非法
- BaseTableName 为空
- Joins 非空
- OrderBy 不存在
- OrderType 非法
- `release_type_list` 非法
- 无发布类型

## 16.3 条件测试

覆盖：

- exact
- like
- close_range
- open_range
- in
- not_in
- 空 Value
- 非法 range JSON
- 非法数组 JSON
- 不存在的字段
- 数字、布尔、时间转换

## 16.4 分页测试

- 无数据
- 一页数据
- 多页数据
- 请求页小于 1
- 请求页超过总页数
- 总数大于 `int32`
- Count 失败
- Select 失败
- Count 和 Select 条件完全一致

## 16.5 页面元数据测试

- 字段按 `display_order` 排序
- 非法 `display_order`
- default_value 为空和非空
- is_required 为 0/1
- `ONLY_DATA` 不查询字段配置
- `ALL` 返回发布类型

## 16.6 选项测试

- 静态选项
- 外部选项
- 静态和外部合并
- 重复 code/name
- 外部 JSON 非法
- 外部表不在白名单
- 字段不存在
- 数字型 code/name
- 外部查询超过最大行数
- 多个外部查询并发度不超过 2
- 外部查询完成顺序不同但最终结果稳定
- goroutine panic 后 WaitGroup 正常结束

## 16.7 安全测试

- 非白名单表
- 非法条件字段
- 非法排序字段
- OrderType 注入字符串
- 表名注入字符串
- 未授权字段查询
- 新增敏感列后返回字段白名单仍生效

---

# 17. 实施步骤

## 阶段一：行为固化

- 为当前 QueryPage 增加特征测试。
- 固化 `ALL/ONLY_DATA`、数据字符串化和选项软降级语义。
- 补齐分页 nil/零值测试。

## 阶段二：模块化替换

- 引入 `PageQueryModule`。
- 引入 `PageConfigCatalog` 和 `TableReader`。
- 保持 RPC 方法和 IDL 不变。
- 用新模块替换 Builder 编排。
- 删除只剩一个实现的 Builder 接口和遗留函数。

## 阶段三：风险治理

- 增加分页默认值和上限。
- SQL 前修正页码。
- 增加条件 Type 校验。
- 增加排序字段/方向校验。
- 引入可查询字段和返回字段白名单。
- 外部选项增加行数上限、稳定排序和去重。

## 阶段四：灰度上线

- 对相同请求同时运行新旧实现。
- 只返回旧结果，异步比较：
  - Data
  - TotalNumber
  - PageNumber
  - InteractionInfoList
  - ReleaseTypeList
  - Options
- 按 ModelCode 灰度切换。
- 观察错误率、延迟和差异率。
- 全量后再清理旧实现。

---

# 18. 验收标准

上线必须同时满足：

- 不提供分页参数时不会 panic，默认查询第 1 页 10 条。
- `ALL` 能返回模型、字段、选项、发布类型和数据。
- `ONLY_DATA` 不访问字段和选项配置。
- 非白名单表不可查询。
- 非法字段、条件类型和排序配置不可进入 SQL。
- SQL 使用归一化后的页码。
- 空数据返回 `PageNumber=1、TotalPage=0、Data=[]`。
- 外部选项单点失败不影响主数据，但有明确监控。
- 外部查询并发度不超过 2。
- 返回结果顺序稳定。
- 新增业务表字段不会未经配置自动暴露。
- 现有内部调用方使用 `ONLY_DATA + PageSize=1` 的行为保持兼容。
