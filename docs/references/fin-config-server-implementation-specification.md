# FinConfig Server 独立实现规格

> 规格版本：1.0.0  
> 状态：Implementation Candidate  
> 适用对象：架构、后端、客户端、DBA、SRE、QA  
> 实施前提：实现团队无需访问任何既有 FinConfig 源码；本文是规范性输入。  
> 兼容目标：保留现有 FinConfig 客户端的查询、版本比较、压缩数据和失效通知语义。

## 0. 规范约定

本文使用以下强度词：

- **必须（MUST）**：违反即视为实现不合格。
- **禁止（MUST NOT）**：任何实现都不得出现。
- **应该（SHOULD）**：默认必须实现；只有经过 ADR 记录的理由才可偏离。
- **可以（MAY）**：实现可选，不影响兼容性。

除“非规范性附录”外，正文均为实施规格。示例用于解释规范；示例与正文冲突时以正文为准。

## 1. 目标、范围与非目标

### 1.1 建设目标

FinConfig Server 是配置数据面的中心读取模块。它必须完成：

1. 从关系数据库加载配置元数据、正式数据、灰度覆盖和增量记录。
2. 构建不可变的进程内一致快照。
3. 按订阅方提供全量、单表、多表、元数据和版本差异查询。
4. 接收消息、动态控制和定时轮询触发，低成本刷新快照。
5. 通过长连接向在线客户端发送失效提示。
6. 在消息丢失、增量失败、客户端慢或连接中断时，依靠版本轮询最终收敛。
7. 运行期失败继续提供最后一次完整快照，禁止发布部分构建或无法验证的状态。

### 1.2 范围

本文定义以下可独立实现内容：

- 领域模型和术语。
- MySQL 控制表、业务表接入约束和索引。
- RPC 逻辑契约和兼容错误码。
- MQ/TCC 触发消息格式。
- 快照结构、稳定编码、压缩和摘要规则。
- 查询、六类刷新、版本轮询和通知算法。
- 事务、游标、并发、幂等和失败语义。
- 配置项、生命周期、SLO、指标、部署、灰度、回滚和验收。

### 1.3 非目标

- 本模块不提供配置写入、审批或发布单工作流。
- 本模块不保证失效通知必达；通知用于降低延迟，轮询负责最终一致性。
- 本模块不向业务调用方暴露数据库查询能力。
- 本模块不执行跨表业务事务。
- 本模块不解释配置字段的业务含义；所有业务列按字符串读取和传输。

## 2. 系统上下文

```text
配置管理端
  │ ① 同事务写业务表、版本、增量记录
  │ ② 提交后发送刷新事件
  ▼
MySQL ──────────────► FinConfig Server ◄──────── MQ / 动态控制
                           │
                           ├── 原子快照 ──► 查询 RPC
                           │
                           ├── 失效提示 ──► Client Stream
                           │
                           └── 版本轮询 ──► 丢事件自愈
                                              │
                                              ▼
                                       FinConfig Client
```

### 2.1 权威关系

- 正式业务表是正式配置内容的最终事实源。
- 灰度控制表是灰度覆盖的最终事实源。
- 版本表是“是否需要重新比较/刷新”的信号源；版本字符串本身不用于排序。
- Command/Binlog 表是变更索引，不是 ADD/MODIFY 内容的最终事实源。
- 当前已发布快照是本进程查询的唯一事实源。
- 客户端必须以服务端查询结果为准，Stream 消息只代表“可能发生变化”。

### 2.2 一致性目标

系统提供以下保证：

- 单次 RPC 响应内强一致：数据、版本、定义和订阅必须来自同一快照代际。
- 单进程发布原子：读者只能看到完整旧快照或完整新快照。
- DB 到服务端最终一致：事件优先，15 秒轮询兜底。
- 服务端到客户端最终一致：Stream 优先，客户端轮询兜底。
- 不承诺跨机房线性一致，不承诺所有实例在同一毫秒切换代际。

## 3. 术语

| 术语 | 定义 |
|---|---|
| 配置表（Table） | 一张被纳入 FinConfig 的业务表 |
| 订阅方（Consumer/PSM） | 声明需要读取某些配置表的应用标识 |
| 正式数据（Baseline） | 业务表中已正式生效的全量数据 |
| 环境（Environment） | 配置作用域，如 `prod`、`boe_xxx`、`ppe_xxx` |
| 部署阶段（Stage） | 生产环境内的细分阶段，如 `canary`、`single_dc`、`all_dc` |
| 灰度覆盖（Overlay） | 在正式数据之上按 env/stage 应用的 ADD/MODIFY/DELETE |
| 行（Row） | `map<string,string>` 形式的一条配置记录 |
| 行键（RowKey） | 按表定义的唯一键字段稳定编码得到的缓存键 |
| 表定义（TableDefinition） | 表名、唯一键、加载策略和增量策略 |
| 版本（Revision） | 表 + 环境对应的不透明变更标识 |
| 摘要（Digest） | 正式压缩体或灰度稳定编码的 MD5 十六进制字符串 |
| 快照（Snapshot） | 某一代全部元数据、数据、派生索引和游标的不可变集合 |
| 代际（Generation） | 本实例每次成功发布快照时递增的 `uint64` |
| Command | 发布方显式写入的增量记录，以 RowKey 标识目标行 |
| Binlog Record | CDC 链路写入的增量记录，以业务表 `id` 标识目标行 |
| 游标（Cursor） | 某表已完整消费的最大增量记录 ID |
| 失效提示（Invalidation Hint） | 只通知客户端重新比较版本的轻量消息 |
| last-known-good | 最近一次通过完整校验并成功发布的快照 |

## 4. 技术基线

默认实施基线如下；替换技术栈时必须保持本文全部行为：

| 类别 | 默认选择 |
|---|---|
| 语言 | Go 1.19 或更高 |
| RPC | Kitex + Thrift，兼容现有方法和字段 |
| 数据库 | MySQL 8.0，InnoDB |
| DB 访问 | `database/sql`、GORM 或等价实现；事务语义以本文为准 |
| 压缩 | LZ4 Frame，`github.com/pierrec/lz4/v4` 4.1.22 |
| 摘要 | MD5，小写 32 位十六进制；空输入使用字符串 `0` |
| 消息 | RocketMQ 广播消费或等价广播消息系统 |
| 动态控制 | TCC 或支持 key-value 读取与轮询的等价系统 |
| 进程内发布 | `atomic.Pointer[Snapshot]` 或等价原子引用 |

服务端是纯读取方。生产数据库账号必须只有 `SELECT` 权限；配置管理端使用独立写账号。

## 5. 功能需求

| 编号 | 需求 |
|---|---|
| FR-001 | 启动时必须先构建并发布完整快照，之后才能进入 Ready |
| FR-002 | 必须支持按订阅方查询全部配置 |
| FR-003 | 必须支持查询单表和指定多表 |
| FR-004 | 必须支持只查询订阅、版本和表定义 |
| FR-005 | 必须支持基于版本、摘要和订阅的 ADD/MODIFY/DELETE 比较 |
| FR-006 | 必须支持未压缩和 LZ4 压缩两套读取形态 |
| FR-007 | 必须支持全量、指定表全量、灰度、Command、Binlog 五类显式刷新，以及版本轮询触发的单表全量刷新 |
| FR-008 | 所有刷新必须采用 Build → Validate → Publish 协议 |
| FR-009 | 增量刷新必须按表隔离失败并保证失败表游标不推进 |
| FR-010 | 必须支持双向流注册、心跳、通知、重连替换和失活清理 |
| FR-011 | 通知队列满时不得阻塞刷新或其他客户端 |
| FR-012 | MQ/TCC/Stream 丢失时必须能靠版本轮询收敛 |
| FR-013 | 运行期任意失败必须继续提供 last-known-good |
| FR-014 | 必须提供快照年龄、刷新结果、游标滞后、通知丢弃和连接数量的可观测数据 |
| FR-015 | 必须支持在 `full_only` 与 `incremental` 两种刷新引擎间动态回切 |

### 5.1 必需模块与接口

实现可以调整包名，但必须保留以下职责和依赖方向：

| 模块 | 对调用者的接口 | 隐藏的实现复杂性 |
|---|---|---|
| `SnapshotCatalog` | `View()`、`Publish(candidate)` | 原子指针、代际、不变量校验、last-known-good |
| `RefreshEngine` | `Refresh(ctx, plan)` | 六类算法、COW、摘要、游标、部分失败 |
| `SourceStore` | `WithinReadView(ctx, consistency, fn)` | DB 选择、事务、分页、schema、批量回查 |
| `QueryModule` | 八个 RPC 对应的逻辑方法 | 订阅、作用域合并、版本比较、响应复制 |
| `StreamHub` | `Serve(stream)`、`Notify(audiences,hint)` | registry、收发循环、心跳、背压、清理 |
| `TriggerRouter` | `HandleMQ/HandleTCC/PollVersions` | 消息校验、RefreshPlan 生成、ACK、通知编排 |
| `Lifecycle` | `Start(ctx)`、`Ready()`、`Close(ctx)` | 依赖装配、后台任务、健康状态、关闭顺序 |

依赖规则：

- 领域模块只依赖本节定义的端口和纯领域模型。
- MySQL、MQ、动态控制、Kitex 都是适配器，不得被领域模型直接 import。
- 传输 DTO 和数据库记录必须在适配器内转换成领域对象。
- `SnapshotCatalog` 只有一种原子内存实现时保持为具体模块，不为测试额外制造空接口；测试通过它的真实接口验证。
- `SourceStore` 必须有 MySQL 生产适配器和内存/SQLite 测试适配器，因此是实际接缝。

`SourceStore` 的最小端口：

```text
SourceStore.WithinReadView(ctx, consistency, fn(SourceReader)) error

SourceReader:
  LoadTableDefinitions(optional table set) -> definitions
  LoadSubscriptions(optional table set) -> subscriptions
  LoadVersions(optional table set) -> revisions
  LoadGrayOverlays(optional table set) -> overlays
  LoadWholeTable(table, page size) -> rows
  LoadRowsByConfigIDs(table, ids) -> rows
  LoadRowsByUniqueTuples(table, fields, tuples) -> rows
  LoadCommandMaxIDs(optional table set) -> map<table,id>
  LoadCommands(table, after, through) -> ordered commands
  LoadBinlogMaxIDs(optional table set) -> map<table,id>
  LoadBinlogRecords(table, after, through) -> ordered records
  LoadIncrementWatermarks(source, optional table set) -> map<table,purged_through_id>
```

所有 `SourceReader` 方法必须使用 `WithinReadView` 提供的同一 DB handle；禁止方法内部绕过 ReadView 重新选择读库或写库。

`WithinReadView` 的 MySQL 适配器必须按以下顺序工作：

1. 根据 `ConsistentPrimary` 或 `ConsistentReplica` 选择一个固定 endpoint；进入 ReadView 后禁止切换。
2. 在任何数据读取前设置隔离级别 `REPEATABLE READ`，并开启只读事务的一致性快照。
3. 把同一个事务对象传给本次回调内的全部 `SourceReader` 方法，包括分页、版本读取、增量窗口和业务行回查。
4. 回调成功时提交只读事务；回调失败、超时或 panic 时回滚。
5. 若所选副本不支持一致性快照或一次事务内的全部表读取，`ConsistentReplica` 必须失败并由调用方改用 primary，禁止退化成多次独立查询。

## 6. 领域数据模型

以下定义是语言无关的逻辑模型。

### 6.1 `Row`

```text
Row = map<ColumnName, StringValue>
```

数据库值转字符串必须遵守：

| 数据库值 | 字符串格式 |
|---|---|
| `NULL` | 空字符串 `""` |
| CHAR/VARCHAR/TEXT/ENUM | 原字符串 |
| 有符号/无符号整数 | 十进制，无前导零（数值 0 除外） |
| BOOLEAN | `true` 或 `false` |
| FLOAT/DOUBLE | 固定 6 位小数；不允许 NaN/Inf |
| DECIMAL | 数据库返回的十进制定点文本，不使用科学计数法 |
| DATE | `YYYY-MM-DD` |
| DATETIME/TIMESTAMP | `YYYY-MM-DD HH:mm:ss`；数据库连接时区必须固定 |
| BINARY/BLOB/JSON/空间类型 | v1 不支持，加载时失败 |

每行必须包含表定义声明的全部唯一键字段。实现不得使用 `%v` 一类不稳定的通用格式处理未知类型。

### 6.2 `RowKey`

v1 编码规则：

1. 按 `unique_keys_def` 中声明的字段顺序取值。
2. 使用单字节 `0x1F`（ASCII Unit Separator）连接。
3. 任一唯一键值包含 `0x1F` 时必须拒绝加载，禁止产生有歧义的键。
4. 缺字段或同表出现重复 RowKey 时，整张表加载失败。

示例：唯一键字段为 `channel_code,merchant_id`，值为 `wxpay` 和 `10001`，RowKey 为：

```text
wxpay\x1f10001
```

变更唯一键必须表达为“DELETE 旧 RowKey + ADD 新 RowKey”；Command 的单条 MODIFY 不支持唯一键变化。

### 6.3 `TableDefinition`

```text
TableDefinition {
  table_name: string
  unique_key_fields: ordered list<string>
  load_to_cache: bool
  incremental_mode: FULL_ONLY | COMMAND | BINLOG
  schema_version: uint64
}
```

约束：

- `table_name` 和列名必须匹配 `^[A-Za-z_][A-Za-z0-9_]{0,63}$`。
- `unique_key_fields` 非空、无重复、全部存在于真实表结构。
- 持久化/传输字段 `unique_keys_def` 必须编码为英文逗号连接且不含空格，例如 `channel_code,merchant_id`；解析时不得静默 trim 或跳过空项。
- 唯一键字段禁止使用 FLOAT/DOUBLE/BLOB/JSON/空间类型；字符串字段必须使用数据库唯一索引相同的排序规则。
- `BINLOG` 模式要求业务表存在名为 `id` 的非负 `BIGINT` 主键。
- 服务端只加载 `load_to_cache=true` 的定义。
- 表定义变化必须触发指定表全量刷新；唯一键变化禁止走增量。

### 6.4 `Subscription`

```text
Subscription {
  consumer: string
  table_name: string
  relation: OneToOne | OneToMany
  subscribe_keys: ordered list<string>
}
```

`subscribe_keys` 必须是业务表真实列；持久化/传输时同样使用英文逗号连接且不含空格。同一订阅方可对同一表声明多组订阅。订阅变化影响查询结果和 `PollNeedRefresh`，不改变正式数据内容。

订阅的持久化身份摘要按以下方式计算：依次对 `consumer/table_name/relation/join(subscribe_keys, ",")` 编码为 `uint32 big-endian 字节长度 + UTF-8 字节`，拼接四段后计算 SHA-256，保存完整 32 字节。配置管理端写入时计算，服务端读取时必须重算校验。使用长度前缀是为了避免字段分隔歧义。

### 6.5 `TableRevision`

```text
TableRevision {
  table_name: string
  environment: string
  revision: string
  prod_digest: optional string
  gray_digest: optional string
  release_number: string
}
```

- `revision` 是不透明非空字符串，只做相等比较。
- 数据库只持久化 `revision/release_number`；两个 digest 由服务端快照构建时计算。
- 请求环境不存在时，查询回退 `prod`；仍不存在时返回合成 revision `0`、两个 digest `0`。
- 版本按环境存储，不按 stage 存储。

### 6.6 `GrayConfig`

```text
GrayConfig {
  id: int64
  table_name: string
  environment: string
  deploy_stage: optional string
  action: ADD | MODIFY | DELETE
  row_key: string
  row_content: Row
}
```

合并顺序固定为 `id` 升序；同一 RowKey 后出现的覆盖先前结果。DELETE 的 `row_content` 可以为空，ADD/MODIFY 必须提供完整行。

作用域匹配规则：

- `environment` 必须与请求环境相等。
- 请求环境不是 `prod` 时，匹配该环境的全部覆盖，忽略 `deploy_stage`。
- 请求环境是 `prod` 时，空 stage 覆盖所有生产阶段；非空 stage 只匹配同名请求 stage。

### 6.7 `IncrementCommand`

```text
IncrementCommand {
  id: int64
  table_name: string
  action: ADD | MODIFY | DELETE
  revision: string
  row_key: string
  row_payload: optional Row
  committed_at: timestamp
}
```

`row_payload` 只可用于诊断和兼容；ADD/MODIFY 必须回查业务主表最终行。Command 必须是追加写，自增 ID 不得复用或修改。

### 6.8 `BinlogRecord`

```text
BinlogRecord {
  id: int64
  table_name: string
  config_id: int64
  action: ADD | MODIFY | DELETE
  before_row: optional Row
  after_row: optional Row
  committed_at: timestamp
}
```

Binlog Record 也是追加写。刷新以业务主表按 `config_id` 回查的最终存在性和内容为准，`action/before_row/after_row` 仅用于定位旧 RowKey 和诊断。

## 7. MySQL 存储规格

### 7.1 通用规则

- 字符集必须为 `utf8mb4`，排序规则必须在所有实例一致。
- 所有控制表必须使用 InnoDB。
- 时间统一存储为 UTC；连接必须显式设置 `time_zone='+00:00'`。
- 表名、字段名只允许来自已校验的 `TableDefinition`，禁止把 RPC 或 MQ 的任意字符串直接作为 SQL identifier。
- 所有值必须参数化；禁止拼接用户值到 SQL。
- 全量读取每页默认 1000 行；回查每批默认 500 个 ID/RowKey。

### 7.2 控制表 DDL

下面 DDL 是 v1 最小可运行集合。审计字段可以增加，不得改变唯一键和查询语义。

```sql
CREATE TABLE fin_config_table_definition (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name VARCHAR(64) NOT NULL,
  unique_keys_def VARCHAR(512) NOT NULL,
  load_to_cache TINYINT(1) NOT NULL DEFAULT 1,
  incremental_mode ENUM('FULL_ONLY','COMMAND','BINLOG') NOT NULL DEFAULT 'COMMAND',
  schema_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
  extra_info JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_table_name (table_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_subscription (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  consumer VARCHAR(255) NOT NULL,
  table_name VARCHAR(64) NOT NULL,
  relation ENUM('OneToOne','OneToMany') NOT NULL,
  subscribe_keys VARCHAR(1024) NOT NULL,
  identity_digest BINARY(32) NOT NULL COMMENT 'SHA-256 of canonical subscription identity',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_subscription_identity (identity_digest),
  KEY idx_consumer_table (consumer, table_name),
  KEY idx_table_name (table_name),
  KEY idx_consumer (consumer)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_version (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name VARCHAR(64) NOT NULL,
  environment VARCHAR(255) NOT NULL,
  revision VARCHAR(255) NOT NULL,
  release_number VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_table_environment (table_name, environment),
  KEY idx_release_number (release_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_gray_overlay (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name VARCHAR(64) NOT NULL,
  environment VARCHAR(255) NOT NULL,
  deploy_stage VARCHAR(255) NULL,
  action ENUM('ADD','MODIFY','DELETE') NOT NULL,
  row_key VARCHAR(2000) NOT NULL,
  row_content JSON NULL,
  release_number VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_table_id (table_name, id),
  KEY idx_scope (table_name, environment, deploy_stage)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_increment_command (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name VARCHAR(64) NOT NULL,
  action ENUM('ADD','MODIFY','DELETE') NOT NULL,
  revision VARCHAR(255) NOT NULL,
  row_key VARCHAR(2000) NOT NULL,
  row_payload JSON NULL,
  release_number VARCHAR(255) NOT NULL DEFAULT '',
  committed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_table_id (table_name, id),
  KEY idx_committed_at (committed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_binlog_record (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name VARCHAR(64) NOT NULL,
  config_id BIGINT UNSIGNED NOT NULL,
  action ENUM('ADD','MODIFY','DELETE') NOT NULL,
  before_row JSON NULL,
  after_row JSON NULL,
  committed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_table_id (table_name, id),
  KEY idx_table_config_id (table_name, config_id),
  KEY idx_committed_at (committed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE fin_config_increment_watermark (
  source ENUM('COMMAND','BINLOG') NOT NULL,
  table_name VARCHAR(64) NOT NULL,
  purged_through_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (source, table_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

兼容既有库时，物理表名可以通过 adapter 配置映射；逻辑字段和唯一性语义必须保持不变。

### 7.3 业务表要求

- 表必须位于配置允许的 schema 内。
- 每个唯一键字段必须有确定、稳定的字符串表示。
- 表中按 `unique_keys_def` 组合必须实际唯一；应该建立数据库唯一索引。
- BINLOG 模式必须有 `id BIGINT UNSIGNED PRIMARY KEY`。
- 单表不应包含 BLOB/二进制大对象；单行编码后上限默认 1 MiB。
- 单表行数、未压缩字节和压缩字节必须受监控；默认软上限分别为 100 万行、1 GiB、256 MiB。

全量读取必须在同一 ReadView 内稳定分页：

- 表有单列非负整数主键 `id` 时，使用 `ORDER BY id ASC LIMIT page_size` 和 `id > last_id` 的 keyset pagination。
- 其他表使用 `ORDER BY unique_key_fields ASC`；可以使用复合 keyset pagination，或在一致性快照内使用 LIMIT/OFFSET。禁止无 ORDER BY 分页。
- 每页只选择已通过 schema 校验的列；列名来自 TableDefinition/schema registry，值仍必须参数化。
- 每一页都必须执行类型规范化、必需字段校验和 RowKey 去重；任一页失败时整表失败。
- 唯一键元组或 config ID 回查默认每批 500 个；返回行必须再按完整 RowKey/config ID 分组，0 行、多行或请求外行均按算法规定处理。

控制表中的 `row_content/row_payload/before_row/after_row` JSON 若存在，必须是对象，且每个 value 必须是 JSON string；数字、布尔、数组、对象或 null value 均视为非法记录。DELETE 允许使用空对象。

### 7.4 发布方事务契约

COMMAND 模式下，一次正式变更必须在同一个数据库事务中按以下逻辑提交：

1. INSERT/UPDATE/DELETE 业务表。
2. 追加一条或多条 `fin_config_increment_command`。
3. 更新 `fin_config_version` 的 revision 和 release number。
4. 提交事务。
5. 提交成功后发送 MQ 刷新事件。

事务内的逻辑顺序不影响提交原子性，但命令必须完整表达该 revision 的全部 RowKey 变化。唯一键变化必须写 DELETE + ADD。

BINLOG 模式允许 CDC 异步产生记录，但必须满足：

- 同一表的 Binlog Record ID 与提交顺序一致，或服务端能够通过最终态回查实现幂等。
- 版本不得早于业务事务提交。
- 增量记录保留期必须不小于 7 天，并大于最大允许服务中断时间。

消息投递失败不得回滚已提交配置；版本轮询必须能发现并修复该情况。

表定义、订阅、灰度的任何语义变化也必须在同一事务中更新所有受影响表的 revision。这个约束保证动态控制回调丢失后，版本轮询仍能发现元数据或灰度变化。

增量记录清理必须由单一保留任务执行。对每个 `source + table_name`，任务必须在同一事务中：

1. 计算本次将删除记录的最大 ID；没有记录时不操作。
2. 将 `fin_config_increment_watermark.purged_through_id` 单调更新到该 ID。
3. 删除 ID 小于等于该值且超过保留期的增量记录。

禁止先删除记录再提交 watermark。刷新发现自身 cursor 小于 `purged_through_id` 时，必须判定增量窗口已断档并执行指定表全量恢复。

## 8. RPC 逻辑契约

### 8.1 公共类型

```text
Status {
  ret_status: SUCCESS | FAIL | UNKNOWN
  ret_code: string
  ret_msg: string
}

TableVersion {
  table_name: string
  version: string
  prod_md5: optional string
  gray_md5: optional string
}

TableDefine {
  table_name: string
  unique_keys_def: string       # 逗号分隔，顺序有意义
}

Subscription {
  psm: string
  table_name: string
  subs_type: OneToOne | OneToMany
  subs_keys: string             # 逗号分隔，顺序有意义
}

GrayConfig {
  id: int64
  table_name: string
  env: string
  deploy_stage: optional string
  action: ADD | MODIFY | DELETE
  gray_key: string
  gray_content: map<string,string>
}
```

所有 map 在传输层可以无序；业务相等性不得依赖 map 遍历顺序。

### 8.2 公共校验

- `psm`、`env` 必须非空。
- `env=prod` 时 `deploy_stage` 必须非空；非生产环境允许空 stage。
- 表名必须非空且存在于订阅中。
- 多表请求不得包含 nil、空表名或重复表；服务端可以先去重，但必须记录重复计数。
- 服务端未 Ready 时返回系统错误，不得读取空快照。
- 请求声明的 PSM 应该与传输身份一致；兼容期可以启用 `trust_request_psm=true`。

### 8.3 `QuerySingleTableConfig`

请求：

```text
{ psm, env, deploy_stage, table_info: TableVersion }
```

成功响应：

```text
{
  status,
  subscription_list,
  table_version,
  table_define,
  table_data: map<RowKey, Row>
}
```

算法：

1. 捕获一次快照 View。
2. 校验订阅。
3. 获取请求 env 的版本。
4. 请求 `table_info.version` 与服务端版本不同，返回 `CI010409`，不返回数据。
5. 复制正式数据并合并请求作用域的灰度。
6. 返回同一 View 中的订阅、版本和表定义。

### 8.4 `QueryAllTableConfig`

请求：`{ psm, env, deploy_stage }`。

响应包含该 PSM 全部订阅表的：

```text
subscription_map: map<table, list<Subscription>>
table_version_map: map<table, TableVersion>
table_define_map: map<table, TableDefine>
table_data_map: map<table, map<RowKey, Row>>
```

任何一张订阅表缺定义、数据或版本时，整次请求失败；禁止返回部分表。

### 8.5 `QueryMultiTableConfig`

请求：`{ psm, env, deploy_stage, table_infos: list<TableVersion> }`。

服务端必须基于一次 View 处理全部表。任一请求版本不匹配时整次返回 `CI010409`，禁止返回部分表。成功响应字段与全量接口相同，但只包含请求表。

### 8.6 `QueryMetaInfo`

请求：`{ psm, env, deploy_stage }`。

响应只包含：

- `subscription_map`
- `table_version_map`
- `table_define_map`

所有字段必须来自同一 View。

### 8.7 `PollNeedRefresh`

请求：

```text
{
  psm,
  env,
  deploy_stage,
  table_version_map: map<table, TableVersion>,
  subscription_map: map<table, list<Subscription>>
}
```

响应：

```text
need_refresh_table_map: {
  ADD: list<TableVersion>,
  MODIFY: list<TableVersion>,
  DELETE: list<TableVersion>
}
```

分类算法：

- 服务端有订阅且客户端无该表版本：`ADD`。
- 双方都有表，且 revision 不同、双方四个 digest 均非空时任一 digest 不同、或订阅集合不同：`MODIFY`。
- 客户端有表但服务端已无订阅：`DELETE`，返回客户端传入版本。
- digest 任一方为空时只比较 revision，兼容旧客户端。
- 订阅列表按集合比较，忽略顺序，保留重复次数语义。

响应必须始终包含三个 key；没有变化时 value 为空列表。

### 8.8 压缩查询

`QueryAllCompressedTableConfig` 请求 `{ psm, env, deploy_stage }`。

`QueryMultiCompressedTableConfig` 额外携带 `table_infos`，并执行与未压缩多表相同的版本校验。

响应：

```text
subscription_map
table_version_map
table_define_map
compressed_table_data_map: map<table, bytes>
gray_config_map: map<table, list<GrayConfig>>
```

压缩响应返回正式数据压缩体，不在服务端合并灰度。为兼容现有客户端，`gray_config_map[table]` 返回该表全部环境的灰度项；客户端按自身 env/stage 过滤。

严格模式开启时，多表压缩请求除 revision 外，还必须校验客户端与服务端的 prod/gray digest；不一致返回 `CI010409`。

### 8.9 兼容错误码

| 场景 | RetStatus | RetCode | RetMsg | 客户端动作 |
|---|---|---|---|---|
| 成功 | SUCCESS | `CI000000` | 操作成功 | 使用结果 |
| 通用失败 | FAIL | `CI002000` | 操作失败 | 记录并退避 |
| 鉴权失败 | FAIL | `CI003000` | 鉴权错误 | 不重试，修复身份/订阅 |
| 参数错误 | FAIL | `CI004000` | 参数错误 | 不重试，修复请求 |
| 系统错误/未 Ready | FAIL | `CI006000` | 系统错误 | 退避重试 |
| 表/数据不存在 | FAIL | `CI010404` | 数据未找到 | 重新拉元数据 |
| 版本不匹配 | FAIL | `CI010409` | 版本不匹配 | 重新 Poll 后重试 |
| 快照查询失败 | FAIL | `CI010410` | 查询缓存失败 | 退避重试 |
| 快照校验失败 | FAIL | `CI010411` | 校验缓存失败 | 告警并退避 |

响应消息禁止包含 SQL、堆栈、配置内容或内部凭据。

### 8.10 `StreamNotify`

客户端消息：

```text
NotifyRequest { psm, env, deploy_stage, client_id }
```

服务端消息：

```text
NotifyResponse {
  status,
  release_number: optional string,
  log_id: optional string,
  need_refresh_table_map: required map<string,list<TableVersion>>
}
```

首条客户端消息同时完成注册和心跳；后续消息只更新心跳。服务端不得在不同模块中对同一 stream 各自调用 `Recv`。

v1 的 Stream 消息是纯失效提示：`NeedRefreshTableMap` 必须发送空但非 nil 的 map，客户端收到任一消息后调用 `PollNeedRefresh` 获取精确 ADD/MODIFY/DELETE。`releaseNumber/logId` 应分别携带触发发布号和刷新 trace ID，无法取得时可以不填。

兼容 Thrift adapter 在发送通知时必须填充：

- `RetStatus=SUCCESS`
- `RetCode=CI000000`
- `RetMsg=操作成功`
- `NeedRefreshTableMap` 为空但非 nil 的 map（若旧 IDL 将其声明为 required）

### 8.11 RPC 方法总表与超时

实现必须暴露以下方法名；大小写是传输契约的一部分：

```text
StreamNotify(NotifyRequest) <-> stream<NotifyResponse>
QuerySingleTableConfig(QuerySingleTableConfigRequest) -> QuerySingleTableConfigResponse
QueryAllTableConfig(QueryAllTableConfigRequest) -> QueryAllTableConfigResponse
QueryMultiTableConfig(QueryMultiTableConfigRequest) -> QueryMultiTableConfigResponse
PollNeedRefresh(PollNeedRefreshRequest) -> PollNeedRefreshResponse
QueryMetaInfo(QueryMetaInfoRequest) -> QueryMetaInfoResponse
QueryAllCompressedTableConfig(QueryAllCompressedTableConfigRequest) -> QueryAllCompressedTableConfigResponse
QueryMultiCompressedTableConfig(QueryMultiCompressedTableConfigRequest) -> QueryMultiCompressedTableConfigResponse
```

默认超时和限制：

- `PollNeedRefresh/QueryMetaInfo`：1 秒。
- 单表/多表/全量查询：3 秒；大表可由调用方显式延长至 10 秒。
- Stream：无总 deadline，受心跳超时和连接 context 控制。
- 多表请求最多 100 张表。
- 单次未压缩响应默认上限 64 MiB；超过时返回 `CI010410` 并要求客户端使用压缩接口。
- 压缩响应默认上限 256 MiB。

旧 Thrift 中的 `Base/BaseResp` 等公共传输字段由 RPC adapter 按组织规范透传或填充，不参与本文领域算法，也不得影响快照选择。

### 8.12 权威 Thrift 传输契约

本节用于生成兼容客户端和服务端代码。字段编号、类型、required/optional、大小写、缺失的字段编号 8、`ChannelInfoDeamonService` 的历史拼写以及字段 255 的 `Base/BaseResp` 类型都必须原样保留。禁止为了整洁重排或修正。

`base.thrift`：

```thrift
namespace go base

struct TrafficEnv {
  1: bool Open = false,
  2: string Env = "",
}

struct Base {
  1: string LogID = "",
  2: string Caller = "",
  3: string Addr = "",
  4: string Client = "",
  5: optional TrafficEnv TrafficEnv,
  6: optional map<string, string> Extra,
}

struct BaseResp {
  1: string StatusMessage = "",
  2: i32 StatusCode = 0,
}
```

`fin_config.thrift`：

```thrift
include "base.thrift"
namespace go caijing.bytepay.channelinfo

struct TableVersion {
  1: required string TableName
  2: required string Version
  3: optional string ProdMD5
  4: optional string GrayMD5
}

struct Subscription {
  1: required string PSM
  2: required string TableName
  3: required string SubsType
  4: required string SubsKeys
}

struct TableDefine {
  1: required string TableName
  2: required string UniqueKeysDef
}

struct GrayConfig {
  1: required i64 ID
  2: required string TableName
  3: required string Env
  4: optional string DeployStage
  5: required string Action
  6: required string GrayKey
  7: required map<string,string> GrayContent
}

struct NotifyRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  4: required string ClientID
  255: base.Base Base
}

struct NotifyResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: required map<string,list<TableVersion>> NeedRefreshTableMap
  5: optional string releaseNumber
  6: optional string logId
  255: required base.BaseResp BaseResp
}

struct QuerySingleTableConfigRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  4: required TableVersion TableInfo
  255: base.Base Base
}

struct QuerySingleTableConfigResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional list<Subscription> SubscriptionList
  5: optional TableVersion TableVersion
  6: optional TableDefine TableDefine
  7: optional map<string,map<string,string>> TableData
  255: required base.BaseResp BaseResp
}

struct QueryAllTableConfigRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  255: base.Base Base
}

struct QueryAllTableConfigResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional map<string,list<Subscription>> SubscriptionMap
  5: optional map<string,TableVersion> TableVersionMap
  6: optional map<string,TableDefine> TableDefineMap
  7: optional map<string,map<string,map<string,string>>> TableDataMap
  255: required base.BaseResp BaseResp
}

struct QueryMultiTableConfigRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  4: required list<TableVersion> TableInfos
  255: base.Base Base
}

struct QueryMultiTableConfigResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional map<string,list<Subscription>> SubscriptionMap
  5: optional map<string,TableVersion> TableVersionMap
  6: optional map<string,TableDefine> TableDefineMap
  7: optional map<string,map<string,map<string,string>>> TableDataMap
  255: required base.BaseResp BaseResp
}

struct PollNeedRefreshRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  4: required map<string,TableVersion> TableVersionMap
  5: required map<string,list<Subscription>> SubscriptionMap
  255: base.Base Base
}

struct PollNeedRefreshResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: required map<string,list<TableVersion>> NeedRefreshTableMap
  255: base.Base Base
}

struct QueryMetaInfoRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  255: base.Base Base
}

struct QueryMetaInfoResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional map<string,list<Subscription>> SubscriptionMap
  5: optional map<string,TableVersion> TableVersionMap
  6: optional map<string,TableDefine> TableDefineMap
  255: base.Base Base
}

struct QueryAllCompressedTableConfigRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  255: base.Base Base
}

struct QueryAllCompressedTableConfigResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional map<string,list<Subscription>> SubscriptionMap
  5: optional map<string,TableVersion> TableVersionMap
  6: optional map<string,TableDefine> TableDefineMap
  7: optional map<string,binary> CompressedTableDataMap
  9: optional map<string,list<GrayConfig>> GrayConfigMap
  255: required base.BaseResp BaseResp
}

struct QueryMultiCompressedTableConfigRequest {
  1: required string PSM
  2: required string Env
  3: required string DeployStage
  4: required list<TableVersion> TableInfos
  255: base.Base Base
}

struct QueryMultiCompressedTableConfigResponse {
  1: required string RetStatus
  2: required string RetCode
  3: required string RetMsg
  4: optional map<string,list<Subscription>> SubscriptionMap
  5: optional map<string,TableVersion> TableVersionMap
  6: optional map<string,TableDefine> TableDefineMap
  7: optional map<string,binary> CompressedTableDataMap
  9: optional map<string,list<GrayConfig>> GrayConfigMap
  255: required base.BaseResp BaseResp
}

service ChannelInfoDeamonService {
  NotifyResponse StreamNotify(1: NotifyRequest req)
    (streaming.mode="bidirectional")
  QuerySingleTableConfigResponse QuerySingleTableConfig(
    1: QuerySingleTableConfigRequest req)
  QueryAllTableConfigResponse QueryAllTableConfig(
    1: QueryAllTableConfigRequest req)
  QueryMultiTableConfigResponse QueryMultiTableConfig(
    1: QueryMultiTableConfigRequest req)
  PollNeedRefreshResponse PollNeedRefresh(1: PollNeedRefreshRequest req)
  QueryMetaInfoResponse QueryMetaInfo(1: QueryMetaInfoRequest req)
  QueryAllCompressedTableConfigResponse QueryAllCompressedTableConfig(
    1: QueryAllCompressedTableConfigRequest req)
  QueryMultiCompressedTableConfigResponse QueryMultiCompressedTableConfig(
    1: QueryMultiCompressedTableConfigRequest req)
}
```

## 9. 刷新事件契约

### 9.1 MQ 消息

消息体必须是 JSON 数组：

```json
[
  {
    "refresh_type": "INCREMENT_REFRESH",
    "table_name": "channel_config",
    "env": "prod",
    "stage": "all_dc",
    "content": {},
    "release_number": "REL-20260825-001"
  }
]
```

字段约束：

| 字段 | 必填 | 约束 |
|---|---|---|
| `refresh_type` | 否 | 空值等价 `INCREMENT_REFRESH`；允许 `INCREMENT_REFRESH/FULL_REFRESH/BINLOG_REFRESH` |
| `table_name` | 是 | 业务表或受支持的灰度控制表标识 |
| `env` | 是 | 非空 |
| `stage` | 条件 | prod 正式发布为 `all_dc` 或空；灰度按实际 stage |
| `content` | 否 | 控制表事件可携带 `table_name` 指向目标业务表 |
| `release_number` | 是 | 同批次应该一致；不一致时拒绝整批 |

整批最大 100 条，解码后最大 1 MiB。任一条非法时拒绝整批，不得跳过非法项后继续。

每条事件同时产生一个通知作用域 `{environment: env, deploy_stage: stage}`。同表、同作用域必须去重；控制表事件必须把作用域归属到 `content.table_name` 指向的目标业务表。`env=prod, stage=""` 表示全部生产 stage。

路由规则：

1. 批次含任一 `FULL_REFRESH`：指定表全量刷新。
2. `BINLOG_REFRESH` 必须整批全部为 Binlog；否则拒绝。
3. 全部事件均不代表 prod 正式数据：只刷新灰度。
4. 批次含任一 prod 正式数据：Command 增量刷新。
5. 未知类型：拒绝。

只有成功发布快照后才能 ACK 成功。系统级失败返回错误交给 MQ 重试；单表部分失败的 ACK 策略见第 13 章。

### 9.2 人工全量动态控制

值格式：

```json
{ "release_number": "MANUAL-20260825-001" }
```

值与上次成功处理值不同时触发全量刷新。处理失败不得更新 last-good 值，下次轮询必须重试。

默认轮询间隔：5 分钟。

### 9.3 人工多表动态控制

值格式：

```json
{
  "table_names": ["table_a", "table_b"],
  "release_number": "META-20260825-001"
}
```

表名必须去重、非空且不超过 100 个。处理失败不得更新 last-good 值。默认轮询间隔：10 秒。

### 9.4 刷新引擎动态控制

值格式：

```json
{ "engine_mode": "full_only", "change_id": "MODE-20260825-001" }
```

`engine_mode` 只允许 `full_only/shadow/incremental`。控制值必须先完整校验，再原子替换进程内 last-good 模式；非法值保留上一模式并告警。没有 last-good 时必须使用本地配置值，本地也未配置时使用 `full_only`。模式变化不修改当前 Snapshot，只影响之后创建的 RefreshPlan。默认轮询间隔：10 秒。

## 10. 快照规格

### 10.1 逻辑结构

```text
Snapshot {
  generation: uint64
  built_at: timestamp
  trigger: string
  release_number: string

  consumer_to_subscriptions:
    map<Consumer, map<TableName, list<Subscription>>>

  table_to_consumers:
    map<TableName, set<Consumer>>

  table_versions:
    map<TableName, map<Environment, TableRevision>>

  table_definitions:
    map<TableName, TableDefinition>

  table_rows:
    map<TableName, map<RowKey, Row>>

  gray_overlays:
    map<TableName, list<GrayConfig>>

  compressed_table_rows:
    map<TableName, bytes>

  config_id_to_row_key:
    map<TableName, map<ConfigID, RowKey>>

  command_cursors:
    map<TableName, int64>

  binlog_cursors:
    map<TableName, int64>
}
```

`config_id_to_row_key` 只对包含合法 `id` 的表建立；没有该字段时 map 可以为空。

### 10.2 不可变规则

- Snapshot 发布后，任何 map、slice、Row、DTO、`[]byte` 都禁止原地修改。
- 所有字段必须私有；查询只能通过只读 View 或复制后的响应对象访问。
- 局部刷新可以共享未变化表的内部对象，但不得把会继续修改的 builder 对象发布出去。
- Query 响应若直接跨进程编码，可以共享只读数据；任何进程内接口必须返回深复制或只读包装。
- 发布时只允许一次原子引用替换。
- 在途请求持有旧 View 时，新旧快照可同时存在；实现必须允许旧快照由 GC/引用计数自然回收。

### 10.3 发布前不变量

`SnapshotValidator` 必须至少检查：

| 编号 | 不变量 |
|---|---|
| INV-001 | generation 比当前快照大 1 |
| INV-002 | consumer→table 与 table→consumer 互为反向关系 |
| INV-003 | 每个订阅表存在合法表定义 |
| INV-004 | 每个已加载表存在 rows、compressed bytes 和版本；合成版本 0 的显式例外除外 |
| INV-005 | 每行可按当前定义重新生成相同 RowKey |
| INV-006 | 同一表没有重复 RowKey |
| INV-007 | 解压后的正式数据与 table_rows 深度相等 |
| INV-008 | prod digest 与正式压缩体摘要相等 |
| INV-009 | gray digest 与相应 env 的灰度稳定编码摘要相等 |
| INV-010 | `config_id_to_row_key` 与实际行的 `id` 双向一致 |
| INV-011 | 对仍存在的表，两类 cursor 均不小于旧快照；计划内删除表允许连同 cursor 移除 |
| INV-012 | 局部刷新只改变计划中的表及其派生索引 |
| INV-013 | 所有 GrayConfig 的 action/作用域/RowKey 合法，ADD/MODIFY 行完整 |
| INV-014 | 所有 TableVersion.table_name 与外层 map key 相等 |

任一不变量失败时：

1. 禁止发布。
2. 禁止推进游标。
3. 禁止发送通知。
4. 记录 invariant ID、表名、基础 generation 和触发来源；禁止打印完整行内容。
5. 返回系统级刷新错误并触发高优告警。

## 11. 稳定编码、压缩与摘要

### 11.1 正式数据规范编码

正式单表 `map<RowKey, Row>` 必须编码成 JSON 数组：

```json
[
  {"key":"row-key-1","value":[["column-a","value-a"],["column-b","value-b"]]},
  {"key":"row-key-2","value":[["column-a","value-c"]]}
]
```

算法：

1. RowKey 按 UTF-8 字节升序排列。
2. 每行列名按 UTF-8 字节升序排列。
3. 输出无空格、无换行的 UTF-8 JSON。
4. JSON 字符串必须正确转义引号、反斜杠、控制字符、U+2028/U+2029。
5. 为兼容 Go `encoding/json` 默认行为，`<`、`>`、`&` 分别编码为 `\u003c`、`\u003e`、`\u0026`。
6. 禁止在稳定编码中输出 map 对象表示 Row，必须使用有序二元数组。

### 11.2 压缩

- 使用 LZ4 Frame 格式压缩规范 JSON 字节。
- v1 参数固定为：4 MiB block、block independence 开启、content checksum 开启、block checksum 关闭、不写 content size、非 legacy frame、单并发、fast/default compression level。
- Go 参考实现必须显式应用与 `lz4.BlockSizeOption(lz4.Block4Mb)`、`lz4.BlockChecksumOption(false)`、`lz4.ChecksumOption(true)`、`lz4.ConcurrencyOption(1)` 等价的选项，禁止依赖未来库版本的默认值。
- 压缩实现和参数必须固定，并以 golden file 测试锁定。
- 同一规范 JSON 输入必须得到逐字节一致的压缩输出。
- 服务端启动时必须执行至少一个内置 golden self-check；失败则不得 Ready。

v1 最小 golden：空表的规范 JSON 是 UTF-8 字节 `[]`，压缩结果必须为以下十六进制字节，摘要必须完全相等：

```text
lz4_hex = 04224d186440a7020000805b5d00000000cc65d478
md5     = 3ff5c44e5bf64fd42d94024c930e7e00
```

### 11.3 正式摘要

- 非空压缩体：`lower_hex(MD5(compressed_bytes))`。
- 没有压缩体的空输入：固定字符串 `0`，不使用 `MD5(empty)`。
- 已加载的空业务表并非“空输入”：它先编码为 `[]`，再得到上节的非空 LZ4 frame，因此 prod digest 是 `3ff5c44e5bf64fd42d94024c930e7e00`。
- 合成版本或兼容路径确实没有压缩体时才允许使用 prod digest `0`。

### 11.4 灰度稳定编码和摘要

计算某表、某 env 的灰度摘要时：

1. 从该表全部 GrayConfig 中筛选 `environment == env`。
2. 按 `id` 升序。
3. 使用字段顺序固定的 JSON 编码；`row_content` 的列名升序。
4. 没有匹配项时输入为空，摘要为 `0`。
5. 有匹配项时对规范 JSON 字节计算 MD5，小写十六进制。

灰度摘要覆盖同一 env 的全部 stage，因此任一 stage 变化都可能使该 env 的客户端判断为 MODIFY。

## 12. 查询算法

### 12.1 获取版本

```text
GetVersion(table, env, view):
  if view.table_versions[table][env] exists:
    return it
  if view.table_versions[table]["prod"] exists:
    return it
  return {table, version:"0", prod_md5:"0", gray_md5:"0"}
```

### 12.2 合并灰度

```text
Merge(table, env, stage, view):
  result = deep-copy(view.table_rows[table])
  overlays = view.gray_overlays[table] sorted by id ASC

  for overlay in overlays:
    if overlay.environment != env:
      continue
    if env == "prod" and overlay.deploy_stage not empty
       and overlay.deploy_stage != stage:
      continue

    switch overlay.action:
      ADD, MODIFY: result[overlay.row_key] = deep-copy(overlay.row_content)
      DELETE:      delete result[overlay.row_key]
      otherwise:   impossible because SnapshotValidator rejects it

  return result
```

### 12.3 订阅集合比较

单个 Subscription 的稳定身份为：

```text
consumer + "\x1f" + table_name + "\x1f" + relation + "\x1f" + subscribe_keys
```

比较时构造 `map<identity,count>`，长度或任一 count 不同即判定变化。不得只按 slice 顺序比较。

### 12.4 查询隔离

- 每个 RPC 在参数校验后必须恰好获取一次 View。
- 一次 RPC 禁止再次读取全局当前快照。
- 多表循环中所有表必须使用该 View。
- 响应构建失败时不得降级到新 View 重试；应返回错误，让客户端重新发起请求。

## 13. 刷新模型与算法

### 13.1 强类型刷新计划

```text
RefreshMode = ALL | TABLES | GRAY | COMMAND | BINLOG | POLL_SINGLE_TABLE

RefreshPlan {
  mode: RefreshMode
  table_names: ordered unique list<string>
  affected_scopes: map<table_name, ordered unique list<Scope>>
  trigger: STARTUP | MQ | TCC_ALL | TCC_TABLES | VERSION_POLL | ADMIN
  release_number: string
  trace_id: string
  requested_at: timestamp
}

Scope {
  environment: string
  deploy_stage: string
}
```

校验：

- `ALL` 不允许传 table_names。
- `POLL_SINGLE_TABLE` 必须恰好一个表。
- 其他模式必须至少一个表，去重后最多 100 个。
- 表名必须先通过 TableDefinition registry 解析；元数据新增场景允许从数据库候选 registry 解析。
- MQ 计划必须填充事件携带的 `affected_scopes`；STARTUP/TCC/VERSION_POLL 计划必须为空。
- `affected_scopes` 为空表示本次刷新不主动推送，客户端依靠自身轮询收敛；不得解释成“广播全部连接”。
- Scope 的 env 必须非空；非空 stage 只能与同一个 env 搭配使用。

### 13.2 结构化结果

```text
TableRefreshResult {
  table_name: string
  status: CHANGED | UNCHANGED | FAILED
  old_revisions: map<environment,string>
  new_revisions: map<environment,string>
  cursor_source: optional COMMAND | BINLOG
  old_cursor: int64
  new_cursor: int64
  row_count_before: int64
  row_count_after: int64
  error_code: optional string
}

RefreshResult {
  published: bool
  base_generation: uint64
  new_generation: uint64
  tables: list<TableRefreshResult>
  started_at: timestamp
  finished_at: timestamp
}
```

系统级错误使用函数 error 返回；单表可隔离错误写入 TableRefreshResult。调用者必须用结果决定通知受众。

`error_code` 必须从以下稳定集合取值；适配器可以附加不含配置内容的诊断原因：

| ErrorCode | 含义 | 默认恢复动作 |
|---|---|---|
| `INVALID_PLAN` | 计划字段、表名或作用域非法 | 拒绝，不重试同一非法输入 |
| `SOURCE_UNAVAILABLE` | DB/事务/动态控制不可用 | 保旧并重试 |
| `INCREMENT_WINDOW_GONE` | cursor 落后于已清理 watermark | 指定表全量恢复 |
| `UNEXPLAINED_VERSION_CHANGE` | 版本变化但没有可消费记录 | 指定表全量恢复 |
| `UNKNOWN_ACTION` | 增量 action 不在允许集合 | 指定表全量恢复并告警生产者 |
| `ROW_BACKFILL_MISSING` | ADD/MODIFY 最终行不存在 | 指定表全量恢复 |
| `ROW_BACKFILL_DUPLICATE` | 唯一键/ID 回查多行 | 全量恢复通常也会失败，需修数据 |
| `ROWKEY_MISMATCH` | 回查行生成的 RowKey 与命令不一致 | 指定表全量恢复并告警生产者 |
| `SNAPSHOT_INVARIANT` | 候选违反 INV-* | 禁止发布，Critical 告警 |

### 13.3 公共刷新协议

所有刷新必须执行：

```text
1. ValidatePlan
2. Acquire global writer lock
3. Load base View from SnapshotCatalog
4. Open database ReadView with required consistency
5. Build candidate entirely in local memory
6. Close/commit read-only ReadView
7. Validate candidate against base and plan
8. Publish candidate atomically
9. Release writer lock
10. Intersect CHANGED tables with plan.affected_scopes
11. Resolve audiences from the newly published View
12. Notify the resolved ClientKeys
13. Emit metrics/logs and return RefreshResult
```

要求：

- 通知必须在释放刷新锁之后执行。
- Build/Validate 失败时第 8～12 步都不得执行。
- 只有 `CHANGED` 表触发通知；`UNCHANGED` 不通知。
- 计划没有对应 affected scope 的 `CHANGED` 表不得通知。
- 生成 trace ID 的触发器必须把同一 ID 贯穿 DB、刷新、发布和通知日志。
- 写锁等待时间必须单独监控。

### 13.4 `ALL`：全量刷新

一致性：默认 `ConsistentReplica`，但只有在数据库提供 REPEATABLE READ 且所有读取来自同一实例/事务时允许；否则使用 `ConsistentPrimary`。

在同一 ReadView 中：

1. 加载全部订阅，构建正反索引。
2. 加载全部版本。
3. 加载全部 `load_to_cache=true` 表定义并校验真实 schema。
4. 加载全部灰度，任一行无效则失败。
5. 逐表全量读取正式数据并构建 RowKey。
6. 构建 `config_id_to_row_key`。
7. 对每表规范编码、LZ4 压缩、计算 prod digest。
8. 对每表/每 env 计算 gray digest。
9. 查询 Command/Binlog 的每表最大记录 ID 和清理 watermark；每类初始 cursor 取二者最大值。
10. 构造完全独立的新 Snapshot，generation = base + 1。

全量刷新是全有或全无；不允许单表部分发布。

### 13.5 `TABLES` / `POLL_SINGLE_TABLE`：指定表全量刷新

在同一 ReadView 中对目标表完成：

1. 重新加载目标表相关订阅，并在旧正反索引中精确替换。
2. 重新加载目标表全部环境版本。
3. 重新加载目标表定义；支持新增、修改和删除。
4. 若定义仍存在且 `load_to_cache=true`：加载正式数据、灰度、压缩体、摘要和 ID 索引。
5. 若定义已删除或 `load_to_cache=false`：从全部数据/版本/灰度/压缩/游标/订阅索引移除该表。
6. 把两类 cursor 分别设置为同一 ReadView 中 `max(最大记录 ID, purged_through_id)`，避免全量后重放旧记录或反复判定断档。

`TABLES` 对整批目标表全有或全无。`POLL_SINGLE_TABLE` 只有一个表，语义相同。

### 13.6 `GRAY`：灰度刷新

在同一 ReadView 中：

1. 加载目标表的全部环境版本。
2. 加载目标表全部灰度记录，按 ID 排序并严格解码。
3. 替换目标表灰度集合。
4. 保留正式 rows、压缩体和 prod digest。
5. 重新计算目标表各环境的 gray digest。

任一目标表灰度无效时整批失败，不发布任何目标表。GRAY 不修改两类 cursor。

### 13.7 `COMMAND`：Command 增量刷新

一致性：必须使用 `ConsistentPrimary`。版本、灰度、Command 窗口和业务最终行必须从同一 ReadView 读取，禁止 repository 私自切换 DB handle。

按表处理；不同表可以部分成功：

```text
for table in plan.tables:
  old_cursor = base.command_cursor[table] default 0
  db_version = load versions(table)
  db_gray = load gray(table)
  purged_through = load COMMAND watermark(table)
  max_cursor = max command id(table)

  if old_cursor < purged_through:
    mark FAILED(INCREMENT_WINDOW_GONE)
    continue

  if db_version equals base version and max_cursor <= old_cursor:
    mark UNCHANGED
    continue

  if max_cursor <= old_cursor and db_version differs:
    mark FAILED(UNEXPLAINED_VERSION_CHANGE)
    continue

  commands = query table where id > old_cursor and id <= max_cursor order by id
  // ID 可以因其他表共享同一自增序列而不连续；不要求相邻。
  require commands is non-empty when max_cursor > old_cursor
  final_command = command with greatest id for each row_key

  next_rows = shallow-copy table row map
  for each final_command ordered by id:
    validate action
    DELETE:
      delete next_rows[row_key]
    ADD or MODIFY:
      decode row_key into unique-key tuple
      query exact current row from business table
      require exactly one row
      validate regenerated RowKey equals command.row_key
      next_rows[row_key] = immutable row

  build compressed bytes, prod digest, gray digest, versions, id index
  set command cursor to max_cursor
  validate complete table state
  mark CHANGED if rows/version/prod-or-gray-digest differs, else UNCHANGED
```

失败表必须保留旧 rows、压缩体、版本、灰度、ID 索引和 cursor。未知 action、回查 0/多行、RowKey 不一致、命令窗口超出保留期均使该表失败。

成功表可以与其他成功表一起发布一个新 Snapshot。只要至少一个表的快照字段或 cursor 发生变化，就可以发布；只有 cursor 前进但配置未变化时表状态为 UNCHANGED，不发送通知。

### 13.8 `BINLOG`：Binlog 增量刷新

一致性：必须使用 `ConsistentPrimary`。

按表处理：

```text
old_cursor = base.binlog_cursor[table] default 0
db_version = load versions(table)
db_gray = load gray(table)
purged_through = load BINLOG watermark(table)
max_cursor = max binlog id(table)

if old_cursor < purged_through:
  FAILED(INCREMENT_WINDOW_GONE)
  stop processing this table

if max_cursor <= old_cursor and db_version equals base version:
  UNCHANGED
  stop processing this table

if max_cursor <= old_cursor and db_version differs from base version:
  FAILED(UNEXPLAINED_VERSION_CHANGE)
  stop processing this table

records = query id in (old_cursor, max_cursor] order by id
require records is non-empty when max_cursor > old_cursor
validate every record action before aggregation
latest_record = greatest id per config_id
current_rows = batch query business table by config_id; duplicate config_id is failure
next_rows = shallow-copy old table rows

for unit in latest_record order by latest id:
  old_row_key = base.config_id_to_row_key[table][config_id]
  if missing and before_row exists:
    derive old_row_key from before_row

  if current_rows has no config_id:
    require old_row_key exists; otherwise fail the table
    delete next_rows[old_row_key] if present
    continue

  new_row = current_rows[config_id]
  new_row_key = BuildRowKey(new_row)
  if old_row_key exists and differs:
    delete next_rows[old_row_key]
  next_rows[new_row_key] = new_row

rebuild compressed bytes, digests, db_version, db_gray, ID index
set binlog cursor to max_cursor
```

失败隔离、发布和通知规则与 COMMAND 相同。Binlog action 不决定最终数据，只用于验证允许值和记录诊断。

### 13.9 版本轮询

默认每 15 秒执行：

1. 检查自动刷新策略；关闭时只记录状态并返回。
2. 一次读取全部版本表。
3. 基于当前 View 按表比较任一环境 revision 是否不同。
4. 将候选按表去重；每张表最多生成一个计划。
5. 对每个候选执行 `POLL_SINGLE_TABLE`。
6. 进入写锁后必须基于最新 View 二次比较；若已由其他刷新收敛，返回 UNCHANGED。

同一轮询不得因为同表多个环境不同而重复全量读取该表。

### 13.10 部分成功与 MQ ACK

- ALL/TABLES/GRAY 是整批原子模式；任一失败返回系统级错误，MQ 不 ACK 成功。
- COMMAND/BINLOG 是按表隔离模式。
- 增量模式中，至少一个表 FAILED 时：
  - 成功表可以发布并通知。
  - `TriggerRouter` 必须在增量 `Refresh` 已返回且写锁已释放后，把失败表合并成一个 `TABLES` 全量恢复计划，并继承原计划中这些表的 affected scopes。
  - 全量恢复成功时，这些表视为已收敛，可以 ACK 原消息；恢复失败时 handler 必须返回“部分失败”的可重试错误，使 MQ 重投整批。
  - 重投时成功表因 cursor/版本已收敛而幂等 UNCHANGED；失败表继续重试。
  - 超过 MQ 最大重试后进入死信并高优告警；版本轮询仍可单表全量自愈。
- 所有表成功或 UNCHANGED 时 ACK 成功。

### 13.11 幂等要求

- 同一事件重复投递不得改变最终配置。
- cursor 只单调前进，绝不因旧事件回退。
- 同一 revision 可以触发多次刷新；第二次应是 UNCHANGED。
- 通知允许重复，客户端必须幂等 Poll。
- 人工 TCC 只在值与 last-good 不同时触发；失败不更新 last-good。

## 14. Stream 与通知规格

### 14.1 客户端身份

```text
ClientKey = { consumer, environment, deploy_stage, client_id }
```

必须使用结构体或无歧义编码作为 map key，禁止用下划线字符串拼接。`client_id` 只在完整 ClientKey 内唯一。

### 14.2 连接状态

```text
ClientConnection {
  key: ClientKey
  last_active_unix_nano: atomic int64
  send_queue: bounded queue<NotifyResponse>
  stream: transport adapter
  context: cancellable context
}
```

每个连接必须恰好有：

- 一个接收循环：处理注册/心跳。
- 一个发送循环：串行调用 transport Send。

### 14.3 注册和重连

1. 首包校验成功后注册。
2. 同一 ClientKey 已存在时执行原子 Replace：先从 registry 摘除旧连接，取消旧 context，关闭旧 queue，再安装新连接。
3. Replace 必须幂等，旧发送循环必须在有限时间内退出。
4. 后续每个合法心跳原子更新时间。
5. EOF、context cancel、Recv/Send 错误均调用一次幂等 Unregister。

### 14.4 受众规则

事件先根据“变化表 → 订阅方”解析 Consumer，再应用作用域：

- 非 prod 事件：只通知相同 consumer + env；stage 非空时只通知相同 stage，空时通知该 env 全部 stage。
- prod 事件：兼容模式下通知相同 consumer 的 prod 目标 stage，以及该 consumer 全部非 prod 连接。
- 生产跨环境扩散必须由 `audience_policy=prod_fanout_all_envs` 显式配置；也可以选择 `exact_scope`。
- 多张变化表解析出的同一 ClientKey 只通知一次。
- prod 作用域 stage 为空时匹配全部 prod stage；非 prod 作用域 stage 为空时匹配该 env 全部 stage。
- STARTUP、TCC 和 VERSION_POLL 的 affected scopes 为空，因此不主动通知。它们完成数据收敛，客户端定时 Poll 负责发现变化。

### 14.5 背压

- 默认 queue 容量 100。
- 广播使用非阻塞 enqueue。
- queue 满时丢弃本次提示并增加 `notify_dropped_total{reason="queue_full"}`。
- 一个慢客户端不得阻塞其他客户端或刷新锁。
- 连续 queue full 达到 10 次应该主动关闭该连接，促使客户端重连。

### 14.6 清理

- 默认 heartbeat timeout 30 分钟。
- 默认每 30 分钟扫描一次。
- 清理先获取 registry 客户端快照，再逐个删除；禁止持有读锁时升级写锁。
- `last_active` 必须原子读写或由 registry 锁保护，race 检测不得报错。

## 15. 并发与内存模型

### 15.1 读写模型

- 查询：无锁，原子读取 Snapshot 指针。
- 刷新：单个全局 writer mutex 串行。
- 连接 registry：独立 RWMutex，不与刷新锁嵌套。
- 广播：在刷新锁外执行；不要求全局广播锁，只要求单连接 Send 串行。

锁顺序：

```text
禁止同时持有 refresh writer lock 与 stream registry lock。
registry lock 只保护 registry 结构，不覆盖网络 Send。
```

### 15.2 COW 规则

- 全量刷新新建全部 map。
- 局部刷新复制 Snapshot 顶层 map，未变化表引用旧不可变对象。
- 目标表复制该表 RowKey map；未变化行可以共享旧不可变 Row。
- ADD/MODIFY 必须整体替换 Row，不修改旧 Row 内字段。
- 版本 DTO 必须复制后再写 digest。
- cursor map 必须复制后再更新。

### 15.3 容量保护

- 单次全量构建预计峰值至少为旧快照 + 新快照 + 临时编码缓冲。
- 在发布前若预计内存超过 `max_snapshot_build_bytes`，必须失败并保留旧快照。
- 全量可以逐表流式编码压缩，但发布前仍需持有新结构化数据和压缩体。
- 建议容器内存 limit 至少为稳定快照 RSS 的 2.5 倍，并用真实最大表压测校准。

## 16. 生命周期与就绪状态

### 16.1 启动状态机

```text
NEW
  -> CONFIG_VALIDATED
  -> DEPENDENCIES_READY
  -> SNAPSHOT_BUILDING
  -> SNAPSHOT_READY
  -> BACKGROUND_TASKS_STARTED
  -> READY
```

启动顺序：

1. 解析并校验本地配置。
2. 初始化日志、指标和 trace。
3. 初始化 DB、MQ、动态控制和 RPC adapter。
4. 注册事件 handler，但暂不接收业务流量。
5. 执行稳定编码 golden self-check。
6. 执行 `ALL` 启动全量刷新。
7. 快照发布成功后启动版本轮询、TCC 轮询、连接清理和状态日志。
8. 启动 MQ consumer。
9. MQ consumer 和 RPC listener 均健康后置 Ready=true。

第 1～8 步任一关键步骤失败，进程必须以非零状态退出。禁止用空快照 Ready。

### 16.2 运行期

- 背景任务必须接受根 context。
- 单次任务 panic 必须被恢复、记录堆栈并进入下一周期，禁止整个循环永久退出。
- 自动刷新策略读取失败时使用 last-good 策略值；没有 last-good 时默认继续轮询并告警。
- 任意刷新失败不影响查询 Ready，除非快照年龄超过 `max_snapshot_staleness`。

### 16.3 就绪与健康

Liveness：事件循环未死锁且进程可调度。

Readiness 必须同时满足：

- 当前 Snapshot 非空且 generation > 0。
- Snapshot 通过校验。
- RPC listener 已启动。
- 快照年龄不超过 `max_snapshot_staleness`，默认 30 分钟。

DB、MQ、TCC 若配置为 required，必须在启动阶段连接成功后才能首次 Ready。运行期的瞬时断连单独上报依赖健康和告警，不立即取消查询 Ready；只有无法收敛导致快照超过最大陈旧时间时才取消 Ready。这样查询可以继续服务 last-known-good，同时编排系统仍能在数据过旧时摘除实例。

### 16.4 关闭

1. Ready=false，停止接收新请求。
2. 停止 MQ/TCC/版本轮询。
3. 等待在途刷新结束，默认上限 30 秒。
4. 取消全部 Stream context 并关闭发送队列。
5. 等待发送/接收循环退出，默认上限 10 秒。
6. 关闭 DB、MQ 和日志 writer。

## 17. 配置规格

示例 YAML：

```yaml
server:
  environment: prod
  cluster: default
  trust_request_psm: true
  strict_digest_mode: true
  audience_policy: prod_fanout_all_envs
  max_snapshot_staleness: 30m
  max_snapshot_build_bytes: 8GiB

database:
  dsn_ref: secret://fin-config-readonly
  schema: channel_config
  timezone: UTC
  full_refresh_consistency: replica
  incremental_consistency: primary
  isolation_level: REPEATABLE_READ
  page_size: 1000
  backfill_batch_size: 500

refresh:
  engine_mode: full_only    # full_only | shadow | incremental
  version_poll_interval: 15s
  max_tables_per_plan: 100
  command_retention: 168h
  binlog_retention: 168h

stream:
  queue_capacity: 100
  heartbeat_timeout: 30m
  cleanup_interval: 30m
  close_after_consecutive_drops: 10

dynamic_control:
  required_for_readiness: true
  namespace: fin_config
  refresh_all_key: refresh_all
  refresh_tables_key: refresh_tables
  engine_mode_key: engine_mode
  refresh_all_poll_interval: 5m
  refresh_tables_poll_interval: 10s
  engine_mode_poll_interval: 10s

mq:
  required_for_readiness: true
  mode: broadcast
  topic: fin_config_refresh
  consumer_group: fin_config_server
  max_message_bytes: 1MiB
  max_retry: 16

rpc:
  service_name: required-deployment-specific-value
  listen_address: 0.0.0.0:8080
  max_request_bytes: 8MiB

logging:
  version_log_interval: 10s
  sample_per_table: true
```

配置校验失败必须阻止启动。时长、字节数必须支持显式单位，禁止裸整数表示时间。

## 18. 可观测性与 SLO

### 18.1 建议 SLO

| 目标 | SLO |
|---|---|
| 查询可用性 | 月度 ≥ 99.99% |
| 正常 MQ：DB 提交到服务端快照 | P99 ≤ 5 秒 |
| 轮询兜底：DB 提交到服务端快照 | P99 ≤ 30 秒 |
| 有 Stream：快照到客户端收敛 | P99 ≤ 10 秒 |
| 只轮询：快照到客户端收敛 | P99 ≤ 60 秒 |
| cursor 倒退 | 0 次 |
| 校验失败后错误发布 | 0 次 |

### 18.2 必需指标

| 指标 | 类型 | 关键标签 |
|---|---|---|
| `fin_config_snapshot_generation` | Gauge | cluster |
| `fin_config_snapshot_age_seconds` | Gauge | cluster |
| `fin_config_snapshot_rows` | Gauge | table |
| `fin_config_snapshot_compressed_bytes` | Gauge | table |
| `fin_config_refresh_total` | Counter | mode, trigger, result |
| `fin_config_refresh_duration_ms` | Histogram | mode, phase |
| `fin_config_refresh_lock_wait_ms` | Histogram | mode |
| `fin_config_refresh_table_total` | Counter | table, result, reason |
| `fin_config_cursor_lag_records` | Gauge | source, table |
| `fin_config_cursor_lag_seconds` | Gauge | source, table |
| `fin_config_snapshot_validation_total` | Counter | invariant, result |
| `fin_config_stream_clients` | Gauge | consumer, env, stage |
| `fin_config_notify_total` | Counter | result, reason |
| `fin_config_background_task_last_success_seconds` | Gauge | task |

禁止把 revision、release number、client ID 作为高基数指标标签；这些值只进入结构化日志。

### 18.3 日志字段

每次刷新至少记录：

```text
trace_id, release_number, trigger, mode, tables,
base_generation, new_generation, published,
old_cursor, new_cursor, row_count_before, row_count_after,
build_ms, validate_ms, publish_ms, notify_ms,
result, error_code
```

禁止记录完整配置行、灰度内容、DSN、凭据和未脱敏个人信息。

### 18.4 告警

- Snapshot age > 2 × version poll interval：Warning。
- Snapshot age > max_snapshot_staleness：Critical，并取消 Ready。
- 任一 SnapshotValidator 失败：Critical。
- 任一 cursor 倒退：Critical。
- 单表 cursor lag > 2 个轮询周期：Warning；>10 分钟：Critical。
- 部分刷新失败连续 3 次：Critical。
- 通知 drop 比例 5 分钟 >1%：Warning。
- 后台任务超过 2 个周期无成功：Critical。

## 19. 安全要求

- 服务端 DB 账号只读，配置管理端写账号与其隔离。
- 动态表名必须经 registry allow-list，不接受跨 schema 标识符。
- 所有 SQL value 参数化。
- RPC PSM 应与 mTLS/服务身份匹配；兼容开关必须有下线计划。
- Stream client_id 最大 255 字符，所有身份字段必须限制长度，防止内存键滥用。
- MQ/TCC 消息必须限制大小、数组长度和字符串长度。
- 配置内容不得写入普通日志；调试抽样必须脱敏且有时效开关。
- 压缩解压必须设置输出上限，防止压缩炸弹；默认单表解压上限 1 GiB。
- 服务端只读取预置 schema，禁止运行 DDL。

## 20. 部署拓扑、灰度与回滚

### 20.1 推荐拓扑

- 每个机房至少 3 个无状态实例。
- 每实例独立持有完整 Snapshot 和 Stream registry。
- MQ 使用广播消费，确保每实例刷新自己的快照。
- 查询可经普通负载均衡；客户端 Stream 断线后重连任意实例。
- 版本轮询每实例独立执行，并增加 0～20% jitter，避免整齐访问 DB。

### 20.2 资源预置顺序

1. 创建控制表和索引。
2. 为业务表补唯一索引，校验 RowKey 无冲突/分隔符。
3. 写入表定义、订阅和初始版本。
4. 预置 MQ topic/group 和 TCC keys。
5. 创建只读 DB 凭据和网络授权。
6. 部署服务但不接流量，完成启动全量和 golden self-check。
7. 执行查询、刷新、Stream 冒烟。
8. 接入小比例流量。

### 20.3 引擎灰度

`engine_mode`：

- `full_only`：参考引擎。ALL/TABLES/GRAY 保持原模式；COMMAND/BINLOG 事件统一转换为目标表 TABLES 全量刷新。该模式数据库成本较高，但算法简单，是独立实现也能拥有的安全回切路径。
- `shadow`：`full_only` 构建并发布；`incremental` 使用同一基础代际只读构建候选并比较，不发布、不通知。
- `incremental`：按本文 COMMAND/BINLOG 算法构建和发布；保留立即切回 `full_only` 的能力。

Shadow 比较必须覆盖：

- 表行规范摘要。
- 表版本和 prod/gray digest。
- 订阅正反索引。
- 两类 cursor。
- 表定义和 config_id 索引。

Shadow 应按表采样，限制双倍 DB/CPU 开销。连续 24 小时零差异后，按 BOE → 单机房 1% → 10% → 50% → 100% 推进。

### 20.4 自动回切

触发任一条件时切 `full_only`：

- SnapshotValidator 失败。
- shadow/incremental 摘要持续差异。
- cursor 倒退或 lag 持续扩大。
- 查询 `CI010409/CI010410/CI010411` 比例超过基线阈值。
- 刷新 P99、峰值 RSS、GC pause 超过发布门禁阈值。

回切影响后续刷新，不修改当前 Snapshot 格式。若当前快照存疑，切回后执行人工 ALL 恢复。

## 21. 测试与验收规格

### 21.1 单元测试表面

测试必须通过以下公开接口验证行为：

```text
Refresh(plan) -> RefreshResult
SnapshotCatalog.View() -> read-only View
Query methods -> response/status
StreamHub.Serve(stream)
StreamHub.Notify(audiences, hint) -> NotifyResult
```

禁止通过修改 Snapshot 内部 map 制造测试状态；应使用 SnapshotBuilder/fixtures。

### 21.2 必测用例

#### 启动与全量

| ID | Given | When | Then |
|---|---|---|---|
| AC-001 | 所有表合法 | 启动 | 发布 generation=1，Ready=true |
| AC-002 | 任一表重复 RowKey | 启动 | 不 Ready，非零退出 |
| AC-003 | 灰度 JSON 非法 | 启动 | 不发布空缺灰度，非零退出 |
| AC-004 | 压缩 golden 不一致 | 启动 | 不 Ready |
| AC-005 | 全量中 DB 失败 | 运行期 ALL | 当前 Snapshot 指针不变 |

#### 查询

| ID | Given | When | Then |
|---|---|---|---|
| AC-010 | prod 正式 + stage 覆盖 | 单表查询 | 按顺序正确合并 |
| AC-011 | 非 prod 多个 stage 覆盖 | 查询 | 应用该 env 全部覆盖 |
| AC-012 | env 无版本、prod 有 | 查询 | 回退 prod 版本 |
| AC-013 | 两者都无版本 | 查询 | 返回版本/摘要 0 |
| AC-014 | 请求版本过期 | 单表/多表 | `CI010409`，无数据 |
| AC-015 | 请求期间发布新代际 | 多表查询 | 响应全部来自旧或新单一代际 |
| AC-016 | 客户端无新订阅表 | Poll | ADD |
| AC-017 | 版本/摘要/订阅变化 | Poll | MODIFY |
| AC-018 | 服务端取消订阅 | Poll | DELETE |
| AC-019 | 相同正式数据以不同 map 插入顺序构建 | 压缩查询并解压 | 压缩字节、digest 相同，数据无损 |

#### Command

| ID | Given | When | Then |
|---|---|---|---|
| AC-020 | ADD payload 与 DB 最终行不同 | COMMAND | 使用 DB 最终行 |
| AC-021 | 同 key ADD→MODIFY | COMMAND | 只应用最大 ID 最终态 |
| AC-022 | DELETE | COMMAND | 删除 RowKey |
| AC-023 | 唯一键变化用 DELETE+ADD | COMMAND | 删除旧键并新增新键 |
| AC-024 | 未知 action | COMMAND | 该表失败，cursor 不动 |
| AC-025 | 回查 0 行/多行 | COMMAND | 该表失败，完整保旧 |
| AC-026 | A 成功、B 失败 | COMMAND | 发布 A，B 全字段保旧，返回部分失败 |
| AC-027 | 重投同事件 | COMMAND | A/B 均幂等，无 cursor 回退 |
| AC-028 | 版本变但无新命令 | COMMAND | UNEXPLAINED_VERSION_CHANGE，不推进版本 |

#### Binlog

| ID | Given | When | Then |
|---|---|---|---|
| AC-030 | 同 config_id 多记录 | BINLOG | 以最大 ID 单元回查最终态 |
| AC-031 | 当前行不存在 | BINLOG | 删除旧 RowKey |
| AC-032 | 唯一键变化 | BINLOG | 删除旧键、新键写入 |
| AC-033 | noop | BINLOG | cursor 前进，不通知 |
| AC-034 | before_row 非法且索引无旧键 | BINLOG | 表失败，cursor 不动 |
| AC-035 | 10 万行、100 变更 | BINLOG | 使用 ID 索引，不扫描 100 次整表 |

#### 快照与并发

| ID | Given | When | Then |
|---|---|---|---|
| AC-040 | 候选 digest 错 | Publish | Validator 拒绝，旧 Snapshot 不变 |
| AC-041 | 并发 MQ/TCC/Poll | 刷新 | 全局串行，generation 连续 |
| AC-042 | 查询持有旧 View | 发布新 Snapshot | 查询完成且旧数据未被修改 |
| AC-043 | 失败表 | 部分发布 | rows/version/gray/compressed/index/cursor 全部保旧 |
| AC-044 | 全量后旧增量事件重投 | 刷新 | cursor 不回退，不重放旧数据 |

#### 触发、断档与恢复

| ID | Given | When | Then |
|---|---|---|---|
| AC-060 | Command cursor 小于 purged watermark | MQ 增量刷新 | 增量失败，TABLES 恢复成功后 ACK，cursor 对齐 max(记录 ID, watermark) |
| AC-061 | Binlog 版本变化但无新记录 | MQ 增量刷新 | 判定 unexplained，执行 TABLES 恢复，不发布新版本旧数据 |
| AC-062 | MQ 批次含一条非法事件 | HandleMQ | 整批拒绝，零刷新、零通知 |
| AC-063 | 同表含多个 env/stage 事件 | MQ 刷新成功 | 表只刷新一次，作用域取并集，每个 ClientKey 最多通知一次 |
| AC-064 | TCC 或版本轮询产生变化 | 刷新成功 | 快照发布，因 affected scopes 为空而不主动通知 |

#### Stream

| ID | Given | When | Then |
|---|---|---|---|
| AC-050 | 新连接只发首包 | Serve | 立即注册 |
| AC-051 | 同 ClientKey 重连 | Serve | 旧循环退出，只保留新连接 |
| AC-052 | 心跳与清理并发 | race test | 无数据竞争 |
| AC-053 | 一个客户端队列满 | Notify | 丢该客户端提示，其他客户端正常 |
| AC-054 | Send 失败 | 发送循环 | 注销连接，不影响广播 |
| AC-055 | prod 事件 | prod_fanout_all_envs | prod 目标 + 全部非 prod 收到一次 |

#### 生命周期、可观测性与引擎回切

| ID | Given | When | Then |
|---|---|---|---|
| AC-070 | Snapshot age 超过最大陈旧时间 | readiness probe | Ready=false，last-known-good 仍保留 |
| AC-071 | 版本轮询单次 panic | 后台调度 | panic 被恢复并记录，下一周期继续运行 |
| AC-072 | engine_mode=full_only | 收到 COMMAND/BINLOG 事件 | 转成目标表 TABLES 刷新，不执行增量算法 |
| AC-073 | engine_mode=shadow 且候选不同 | 刷新 | full_only 结果发布，增量候选不发布/不通知并上报差异 |
| AC-074 | incremental 动态切换 full_only | 下一次刷新 | 当前 Snapshot 不变，后续事件走 TABLES 回切路径 |
| AC-075 | 一次部分成功刷新 | 结束 | 必需 counter/histogram/gauge 与结构化日志字段完整、各记一次 |

### 21.3 集成与故障注入

- MySQL REPEATABLE READ：分页期间并发写入不得污染本次快照视图。
- 主从延迟：增量刷新不得发布“新版本 + 旧业务行”。
- MQ 丢消息：版本轮询在 SLO 内收敛。
- MQ 重复/乱序：最终快照相同。
- TCC 回调丢失：key 轮询触发恢复。
- DB 短暂不可用：查询继续读取 last-known-good。
- 压缩/解压失败：候选不发布。
- OOM 预估超过阈值：构建提前失败，不打垮进程。
- 进程优雅关闭：后台任务和 Stream goroutine 在超时内归零。

### 21.4 性能门禁

用最大生产表的 1x/2x/3x 数据量验证：

- 全量构建耗时和峰值 RSS。
- 单表全量 P50/P99。
- 100/1000/10000 条 Command/Binlog 增量。
- 1k/10k/50k Stream 连接广播。
- 查询 QPS、P99 和 GC pause。

发布阈值在 PR-0 用真实基线确定；默认要求 incremental P99 不比 full_only 回退超过 10%，峰值 RSS 不增加超过 15%。

## 22. 实施拆分与人力估算

以 2 名熟悉 Go/MySQL 的工程师估算：

| PR | 内容 | 预计 | 可独立回滚 |
|---|---|---:|---|
| PR-0 | 契约测试、golden、容量基线、dashboard 骨架 | 2 人日 | 是 |
| PR-1 | Stream 单接收循环、Replace、原子心跳、清理 | 3 人日 | 是，开关回旧 Stream |
| PR-2 | 严格 action/灰度解析、动态表 allow-list、Validator v1 | 3 人日 | 是，Validator 可 observe-only |
| PR-3 | RefreshPlan/Result、事件严格校验、按变化表通知 | 4 人日 | 是，engine_mode=full_only |
| PR-4 | SnapshotCatalog/View/Builder，字段私有化 | 4 人日 | 是，回切 full_only |
| PR-5 | SourceStore/ReadView、纯领域模型、primary 一致增量 | 4 人日 | 是，回切 full_only |
| PR-6 | config_id 派生索引、指标、生命周期/readiness | 3 人日 | 是，索引可关闭 |
| 灰度 | Shadow 24h、BOE、1/10/50/100% | 3～5 人日 | 是 |

总计约 26～28 人日，即 2 人并行约 3～4 周。数据库资源、MQ/TCC 预置和跨团队联调不计入编码人日，应并行排期。

## 23. 完成定义（Definition of Done）

- 本文 FR-001～FR-015 全部有自动化测试对应。
- INV-001～INV-014 全部由运行时代码校验并有负向测试。
- 本文列出的全部 AC 验收用例通过。
- 8 个 RPC 的兼容测试通过，旧客户端无需升级即可工作。
- Command/Binlog 单表失败证明所有相关字段和 cursor 保旧。
- race、故障注入和 3x 容量压测通过。
- Shadow 连续 24 小时零不可解释差异。
- SLO dashboard、告警、回切开关和人工 ALL 恢复均完成演练。
- 运维手册包含版本 lag、部分失败、Stream drop、Validator 失败、RSS 高水位五类处置流程。

## 24. 需求追踪矩阵

| 需求 | 规范章节 | 主要验收证据 |
|---|---|---|
| FR-001 | 10、13.4、16 | AC-001～AC-005、AC-070 |
| FR-002 | 8.4、12 | RPC 兼容测试、AC-015 |
| FR-003 | 8.3、8.5、12 | AC-010～AC-015 |
| FR-004 | 8.6 | RPC 兼容测试、元数据响应不含数据断言 |
| FR-005 | 8.7、12.3 | AC-016～AC-018 |
| FR-006 | 8.8、11 | AC-004、AC-019 |
| FR-007 | 9、13.4～13.9 | AC-020～AC-035、AC-060～AC-064 |
| FR-008 | 10.3、13.3 | AC-005、AC-040～AC-044 |
| FR-009 | 13.7、13.8、13.10 | AC-024～AC-028、AC-034、AC-043、AC-060 |
| FR-010 | 8.10、14 | AC-050～AC-052、AC-054 |
| FR-011 | 14.5 | AC-053 |
| FR-012 | 7.4、13.9、13.10 | AC-060、AC-061、MQ/TCC 丢失故障注入 |
| FR-013 | 10.3、16.2 | AC-005、AC-026、AC-040、AC-043、AC-070 |
| FR-014 | 18 | AC-075、dashboard/alert 验收 |
| FR-015 | 17、20.3、20.4 | AC-072～AC-074 |

## 附录 A：端到端示例

### A.1 正式 ADD

1. 管理端事务内向 `channel_config` 插入行 `{id:10, channel_code:"wx", merchant_id:"100"}`。
2. 生成 RowKey `wx\x1f100`，追加 ADD Command。
3. 更新 `(channel_config, prod)` revision 为 `2026-08-25T10:00:00.123Z`。
4. 提交后发送 MQ INCREMENT_REFRESH。
5. Server 在 primary ReadView 中读取 Command，按 RowKey 回查最终行。
6. 构建新单表数据、压缩体、digest、版本和 cursor；校验后发布 generation+1。
7. Server 通知订阅 `channel_config` 的目标连接。
8. Client Poll 看到 revision/digest 变化，按版本拉取表并原子替换客户端快照。

### A.2 灰度 DELETE

1. 管理端向灰度表追加 `{env:"prod", stage:"canary", action:"DELETE", row_key:"wx\x1f100"}`。
2. 更新 prod revision 并发送 MQ，事件不代表 prod/all_dc 正式数据。
3. Server 执行 GRAY，只更新 GrayConfig 与 gray digest。
4. canary 查询合并时删除该 RowKey；all_dc 不应用这条 stage 覆盖。
5. 压缩查询仍返回正式数据压缩体，并把该 GrayConfig 交给客户端合并。

### A.3 丢失 MQ

1. DB 事务已提交，MQ 投递永久失败。
2. Server 15 秒版本轮询读取到 revision 与 Snapshot 不同。
3. 按表去重后执行单表全量刷新。
4. 新 Snapshot 发布；即使 Server 没主动推流，Client 自身轮询也会发现差异并收敛。

## 附录 B：非规范性逆向证据

现有实现的源码证据、差距分析和迁移理由记录在同目录的 `fin_config_server_technical_design.md`。该文档只用于审计设计来源；实现本文规格不需要阅读它。
