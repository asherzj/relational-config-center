# Runtime Config Server 开源独立实现规格

> 规格版本：1.0.0
> 状态：Implementation Candidate
> 所属上下文：Server
> 传输：gRPC/Protobuf
> 默认依赖：MySQL 8.0+ 只读账号
> 约束：无 PSM、Stage、Region、Thrift、Kitex、RMQ 或 TCC；保留开源 Environment 隔离能力。

## 0. 规范约定

MUST 表示实现必须满足；MUST NOT 表示禁止；SHOULD 表示默认要求；MAY 表示可选。本文与 QueryPage、发布单和 Go Client 开源规格共同定义完整系统。

## 1. 目标、范围与非目标

### 1.1 建设目标

Server 必须：

1. 从关系数据库加载表定义、订阅、正式数据、百分比灰度规则、版本和 Command。
2. 为每个启用 Environment 构建独立的不可变进程内快照。
3. 通过 gRPC 提供全量、单表、元数据和行级 Command 增量同步。
4. 接受 MySQL Outbox 的刷新请求，并通过版本轮询处理通知丢失。
5. 通过 Stream 发送失效提示；提示不承担可靠数据传输。
6. 任意失败继续提供最后一次完整快照。
7. 每段 Command 只在 Server 刷新路径解析一次；Client RPC 只读取已发布快照，不触发数据库查询。

### 1.2 范围

- 单租户；支持多个 `TEST` 环境和全系统唯一的 `PRODUCTION` 环境；
- 全量和指定表刷新；
- Server 自身按 Command 增量刷新；
- Client `SyncByCommand` 行级增量；
- 百分比灰度策略和 SHA-256 显式名单分发；
- 不可变快照、COW、版本、cursor、水位和自愈；
- gRPC 查询、同步和 Stream；
- SLO、可观测性、容量和安全限制。

### 1.3 非目标

- Server 不写配置业务表和控制表。
- Server 不执行审批、发布单或 RBAC。
- Server 不根据业务 SubjectKey 计算灰度命中。
- Server 不提供任意数据库查询。
- Server 不支持 JOIN、Stage、Region 或多租户隔离；Environment 存储采用独立数据库还是同库逻辑隔离需要另行确认。
- 第一版不支持 Binlog/CDC 增量；Command 是唯一增量事实索引。

## 2. 系统上下文

```text
Admin publication transaction
  | writes business rows + version + Command + Outbox
  v
MySQL <------------------------- Runtime Config Server
  ^                                      |
  | read-only                            +--> gRPC snapshot/query
  |                                      +--> SyncByCommand
  +-------- version poll ----------------+--> WatchChanges
                                         |
                                         v
                                    Go Client SDK
```

### 2.1 权威关系

- Managed Table 是当前完整数据的最终事实源。
- Rollout 控制表是灰度规则的最终事实源。
- `rcc_table_versions` 是变化信号和发布事务边界。
- Command 是已提交变化的权威增量日志；ROW Command 的规范化 `ConfigValue` 是该次提交后的最终行事实。
- Server 已发布 Snapshot 是一次 gRPC 请求的唯一读事实源。

### 2.2 一致性目标

- 查询只观察请求所选 Environment 的一个 Snapshot generation。
- Server 刷新失败不改变当前 Snapshot。
- Client 增量永远不得超过捕获的 Server Snapshot cursor。
- 每张表的数据、元信息、灰度规则、版本和 cursor 作为一个原子单元提交。
- Command 丢通知时由版本轮询恢复；Command 过期时单表全量恢复。

## 3. 术语

| 术语 | 定义 |
|---|---|
| ConfigKey | Managed Table 的 `id` 字符串表示 |
| TableVersion | 表的单调 revision、摘要和 Server 已应用 Command cursor |
| Command | 带全局自增 ID、可校验最终态载荷的表级变化日志 |
| Table Cursor | 某消费者已成功应用的该表最大 Command ID |
| Snapshot | Server 当前完整、不可变的运行态读取模型 |
| TableSnapshot | 一张表的数据、元信息、灰度和 cursor 原子集合 |
| RolloutBundle | 某表引用的灰度 Policy、Rule 和名单哈希集合 |
| Full Sync | 返回完整 TableSnapshot 的同步方式 |
| Command Delta | `(clientCursor, snapshotCursor]` 的最终态行级变化 |
| ResolvedDeltaHistory | Server 刷新时物化、供所有 Client 复用的有界增量历史 |
| Environment | Server Snapshot、订阅、版本、Command 和 Client 连接的一级隔离维度；类型为 `TEST` 或 `PRODUCTION` |

## 4. 技术基线

| 关注点 | 选择 |
|---|---|
| 语言 | Go 当前稳定版与前一稳定版 |
| RPC | grpc-go + Protobuf |
| 数据库 | MySQL 8.0+，默认 8.4 LTS |
| DAL | GORM + go-sql-driver/mysql；动态表读取经受控编译器 |
| 压缩 | LZ4 Frame |
| 并发发布 | `atomic.Pointer[Snapshot]` |
| 指标 | Prometheus |
| Trace | OpenTelemetry |
| 日志 | 结构化日志 |

生产数据库账号只读。测试使用真实 MySQL Testcontainers；内存 fake 只用于模块接口测试。

## 5. 功能需求

| ID | 需求 |
|---|---|
| FR-001 | 启动时构建完整 Snapshot，失败则不 Ready |
| FR-002 | 提供消费者全量快照和指定表全量 |
| FR-003 | 提供 `SyncByCommand` 表级游标增量 |
| FR-004 | 每个请求只读取一个 Snapshot generation |
| FR-005 | Server 自身按 Command COW 刷新 |
| FR-006 | Command 断档、非法或不一致时单表全量恢复 |
| FR-007 | Stream 发送表变化提示，轮询保证最终一致 |
| FR-008 | 分发百分比灰度规则和 SHA-256 名单，不计算命中 |
| FR-009 | 失败表保持数据、元信息、版本、灰度和 cursor 全部旧值 |
| FR-010 | 支持优雅关闭、就绪、健康和容量保护 |
| FR-011 | Client 查询和同步 RPC 不访问 MySQL |
| FR-012 | ROW ADD/MODIFY 直接消费可信最终态 Command，不回查 Managed Table |
| FR-013 | 为版本大盘提供每个 Server 节点、每张表的源版本时间、数据 MD5 和缓存加载时间 |

### 5.1 必需模块与接口

```go
type SnapshotCatalog interface {
    View() SnapshotView
}

type RefreshModule interface {
    Refresh(ctx context.Context, plan RefreshPlan) (RefreshResult, error)
}

type RuntimeQuery interface {
    GetSnapshot(ctx context.Context, req GetSnapshotRequest) (GetSnapshotResponse, error)
    GetTables(ctx context.Context, req GetTablesRequest) (GetTablesResponse, error)
    SyncByCommand(ctx context.Context, req SyncByCommandRequest) (SyncByCommandResponse, error)
}

type NotificationHub interface {
    Serve(stream WatchChangesStream) error
    Notify(change ChangeHint) NotifyResult
}

type CommandDeltaResolver interface {
    Resolve(base TableSnapshot, commands []Command) (ResolvedTableDelta, error)
}
```

这些接口同时是主要测试面。`CommandDeltaResolver` 是 Server 内部接缝：默认 `TrustedCommandResolver` 负责校验、聚合和生成 COW 变更，测试使用内存 fake；它不向 RPC 调用方暴露数据库或 Command 细节。

## 6. 领域数据模型

### 6.1 Row

```go
type Row map[string]string
```

SQL NULL 通过字段存在且值为 JSON 文本 `null` 表示。所有行必须包含 `id`。

### 6.2 ConfigKey

`ConfigKey` 是经 Schema Codec 规范化后的 `id` 字符串。禁止用 `%v` 等不稳定格式生成。

### 6.3 TableDefinition

```go
type TableDefinition struct {
    TableName string
    Columns   []ColumnDefinition
    KeyField  string // 固定为 id
}
```

列按 ordinal position 排序。Server 不保存业务列定义副本，刷新时从实时 Schema 生成。

### 6.4 Subscription

```go
type Subscription struct {
    Environment string
    ConsumerID string
    TableName  string
    Code       string
    Type       string   // ONE / MANY
    KeyFields  []string
}
```

同一消费者、表和 code 唯一。字段必须存在于 TableDefinition。

### 6.5 TableVersion

```go
type TableVersion struct {
    Environment   string
    TableName     string
    Revision      uint64
    DataDigest    string
    RolloutDigest string
    CommandCursor int64
}
```

`Revision` 单调递增；不能用时间字符串排序。Digest 用于完整性和 shadow 比较。

### 6.6 RolloutPolicy

```go
type RolloutPolicy struct {
    PolicyCode      string
    AllocationGroup string
    AllocationSeed  string
    BucketAlgorithm string
    RolloutBPS      uint32
    Revision        uint64
}
```

`RolloutBPS` 范围 0～10,000。Server 只验证结构和已知内置算法格式；自定义算法是否注册由 Client 判断。

### 6.7 RolloutRule 与名单

```go
type RolloutRule struct {
    RuleCode   string
    TableName  string
    ConfigKey  string
    Action     string // ADD / MODIFY / DELETE
    Content    Row
    PolicyCode string
}

type SubjectOverride struct {
    PolicyCode  string
    SubjectHash string // lowercase SHA-256 hex
    Decision    string // FORCE_INCLUDE / FORCE_EXCLUDE
}
```

### 6.8 Command

```go
type Command struct {
    Environment  string
    ID           int64
    TableName    string
    ResourceKind string // ROW / TABLE_META / ROLLOUT
    Action       string // ADD / MODIFY / DELETE
    ConfigKey    string
    ConfigValue  Row    // ROW ADD/MODIFY 的规范化完整最终行
    SchemaDigest string
    PayloadChecksum string
}
```

ROW Command 是可直接重放的最终态日志：ADD/MODIFY 的 `ConfigValue` 必须是写入完成后、提交前在同一事务内读到的规范化完整行；DELETE 的 `ConfigValue` 必须为空，表示 tombstone。`SchemaDigest` 标识生成载荷时使用的 TableDefinition，`PayloadChecksum` 保护命令信封。Server 校验通过后直接应用，不再按 ConfigKey 回查 Managed Table。

TABLE_META 和 ROLLOUT Command 在第一版仍是完整替换信号，Server 从控制表加载最终 bundle；它们不得引起 Managed Table 行回查。两者的 `ConfigValue` 可以为空，`SchemaDigest` 使用目标表当前定义摘要，`PayloadChecksum` 仍覆盖完整命令信封。未来可以把完整 bundle 放入 Command，但不改变 Client Delta 接口。

## 7. MySQL 存储规格

### 7.1 通用规则

- 所有控制表使用 `rcc_` 前缀并禁止作为 Managed Table。
- 字符集 `utf8mb4`，时间使用 UTC。
- 不使用触发器生成 Command；Publication Committer 显式写入。
- 所有会改变运行时结果的事务必须更新版本并写 Command。
- 所有运行时控制记录必须携带 `environment`；唯一键、索引和仓储查询必须把 Environment 作为首个隔离条件。
- 环境目录允许多个 `TEST` 记录，只允许一个 `PRODUCTION` 记录，并至少各有一个启用环境。

### 7.2 控制表 DDL

```sql
CREATE TABLE `rcc_runtime_subscriptions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `consumer_id` varchar(128) NOT NULL,
  `table_name` varchar(64) NOT NULL,
  `code` varchar(100) NOT NULL,
  `type` varchar(16) NOT NULL,
  `key_fields` json NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_runtime_subscription` (`environment`, `consumer_id`, `table_name`, `code`),
  KEY `idx_runtime_subscription_table` (`environment`, `table_name`, `enabled`),
  CONSTRAINT `chk_runtime_subscription_type` CHECK (`type` IN ('ONE','MANY')),
  CONSTRAINT `chk_runtime_subscription_enabled` CHECK (`enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_table_versions` (
  `environment` varchar(64) NOT NULL,
  `table_name` varchar(64) NOT NULL,
  `revision` bigint unsigned NOT NULL,
  `command_cursor` bigint NOT NULL DEFAULT 0,
  `release_number` varchar(64) NOT NULL DEFAULT '',
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`environment`, `table_name`),
  CONSTRAINT `chk_table_version_revision` CHECK (`revision` > 0),
  CONSTRAINT `chk_table_version_cursor` CHECK (`command_cursor` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_increment_commands` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `table_name` varchar(64) NOT NULL,
  `resource_kind` varchar(16) NOT NULL DEFAULT 'ROW',
  `action` varchar(16) NOT NULL,
  `config_key` varchar(255) NOT NULL,
  `config_value` json NULL,
  `schema_digest` char(64) NOT NULL,
  `payload_checksum` char(64) NOT NULL,
  `release_number` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_command_table_id` (`environment`, `table_name`, `id`),
  KEY `idx_command_release` (`environment`, `release_number`, `id`),
  CONSTRAINT `chk_command_kind` CHECK (`resource_kind` IN ('ROW','TABLE_META','ROLLOUT')),
  CONSTRAINT `chk_command_action` CHECK (`action` IN ('ADD','MODIFY','DELETE')),
  CONSTRAINT `chk_command_schema_digest` CHECK (`schema_digest` REGEXP '^[0-9a-f]{64}$'),
  CONSTRAINT `chk_command_payload_checksum` CHECK (`payload_checksum` REGEXP '^[0-9a-f]{64}$'),
  CONSTRAINT `chk_command_row_payload` CHECK (
    `resource_kind` <> 'ROW'
    OR (`action` = 'DELETE' AND `config_value` IS NULL)
    OR (`action` IN ('ADD','MODIFY') AND `config_value` IS NOT NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_command_watermarks` (
  `environment` varchar(64) NOT NULL,
  `table_name` varchar(64) NOT NULL,
  `purged_through_id` bigint NOT NULL DEFAULT 0,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`environment`, `table_name`),
  CONSTRAINT `chk_command_watermark` CHECK (`purged_through_id` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_rollout_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `policy_code` varchar(100) NOT NULL,
  `allocation_group` varchar(100) NOT NULL,
  `allocation_seed` varchar(128) NOT NULL,
  `bucket_algorithm` varchar(100) NOT NULL DEFAULT 'sha256-v1',
  `rollout_bps` int unsigned NOT NULL,
  `revision` bigint unsigned NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_rollout_policy_code` (`environment`, `policy_code`),
  CONSTRAINT `chk_rollout_algorithm` CHECK (`bucket_algorithm` REGEXP '^[a-z][a-z0-9-]*-v[1-9][0-9]*$'),
  CONSTRAINT `chk_rollout_bps` CHECK (`rollout_bps` <= 10000),
  CONSTRAINT `chk_rollout_enabled` CHECK (`enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_rollout_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `rule_code` varchar(100) NOT NULL,
  `table_name` varchar(64) NOT NULL,
  `config_key` varchar(255) NOT NULL,
  `action` varchar(16) NOT NULL,
  `content` json NULL,
  `policy_code` varchar(100) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_rollout_rule_code` (`environment`, `rule_code`),
  UNIQUE KEY `uk_rollout_table_key` (`environment`, `table_name`, `config_key`),
  KEY `idx_rollout_policy` (`environment`, `policy_code`, `enabled`),
  CONSTRAINT `chk_rollout_rule_action` CHECK (`action` IN ('ADD','MODIFY','DELETE')),
  CONSTRAINT `chk_rollout_rule_enabled` CHECK (`enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `rcc_rollout_subject_overrides` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `policy_code` varchar(100) NOT NULL,
  `subject_hash` char(64) NOT NULL,
  `decision` varchar(16) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_rollout_subject` (`environment`, `policy_code`, `subject_hash`),
  KEY `idx_rollout_subject_decision` (`environment`, `policy_code`, `decision`),
  CONSTRAINT `chk_rollout_subject_hash` CHECK (`subject_hash` REGEXP '^[0-9a-f]{64}$'),
  CONSTRAINT `chk_rollout_decision` CHECK (`decision` IN ('FORCE_INCLUDE','FORCE_EXCLUDE'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 7.3 Managed Table 要求

- 目标表必须有启用 Table Policy。
- `id` 是唯一单列主键并可无损转换为字符串。
- Server 只选择实时 Schema 支持的列。
- 分页读取始终 `ORDER BY id ASC`，优先使用 keyset pagination。
- 所有运行态写入必须经过 Publication Committer；数据库账号和权限必须阻止旁路写入。
- trigger、外键级联或存储过程若会改变发布目标以外的行，Publication Committer 必须枚举这些 ConfigKey 并为其写 Command；无法枚举时禁止用于 Managed Table。

### 7.4 发布事务契约

一次变更必须在同一事务中：

1. 锁定发布单并重验实时 Schema、Policy 和 before state。
2. 修改业务行或 Rollout 控制表。
3. 对 ROW ADD/MODIFY，在同一事务内按 ConfigKey 批量读取写后最终行，覆盖数据库默认值、触发器、generated column 和归一化结果。
4. 使用稳定 Schema Codec 生成完整 `ConfigValue`、`SchemaDigest` 和 `PayloadChecksum`；DELETE 生成 tombstone。
5. 为每个受影响表和 ConfigKey 追加 Command。
6. 取得每表本事务最大 Command ID。
7. 单调增加 `rcc_table_versions.revision` 并写 `command_cursor`。
8. 写发布日志和 Outbox。
9. 提交。

任一最终行缺失、ConfigKey 不一致、Schema 变化或校验失败都必须回滚整个发布事务。由此保证“业务数据提交成功”等价于“对应可信 Command、版本和 Outbox 一并提交成功”。发布端只执行一次最终态读取，读取成本不再被 Server 实例数和失败重试放大。

名单或 Policy 变化时，为所有引用该 Policy 的表写 `ROLLOUT` Command。订阅或 Page/Runtime 元信息变化时写 `TABLE_META` Command。

### 7.5 Command 清理

清理任务对每张表在同一事务中先推进 `purged_through_id`，再删除不大于该值且超过保留期的 Command。禁止先删 Command 后写水位。默认保留 7 天，必须大于最大允许 Client 离线时间。

## 8. gRPC 逻辑契约

### 8.1 公共类型

Protobuf 使用 `string` 表示动态字段值；嵌套 map 通过显式 message 表达。所有响应携带 `request_id` 和稳定错误状态。

### 8.2 公共校验

- `environment` 必填、长度不超过 64，且必须对应启用环境；禁止默认环境和跨环境回退。
- `consumer_id` 和 `client_id` 必填、长度不超过 128。
- 表名只能来自该消费者当前订阅或明确允许的管理查询。
- Cursor 必须大于等于 0。
- 单请求最多 100 张表，响应受压缩前后字节上限控制。

### 8.3 `GetSnapshot`

返回消费者全部启用订阅对应的完整表、元信息、RolloutBundle、版本和 `command_cursor_map`。该响应是 Client 首次启动基线。

### 8.4 `GetTables`

返回指定表完整 TableSnapshot，用于 Command 过期、delta 过大或单表校验失败后的恢复。返回表数据与对应 cursor 必须来自同一 Server Snapshot。

### 8.5 `GetMetadata`

返回消费者订阅、TableDefinition、TableVersion 和 Rollout 摘要，不返回行数据。诊断使用，不参与 Client 正常同步主路径。

### 8.6 `SyncByCommand`

逻辑请求：

```go
type SyncByCommandRequest struct {
    Environment string
    ConsumerID string
    ClientID   string
    CursorMap  map[string]int64
}
```

逻辑响应：

```go
type TableCommandDelta struct {
    TableName       string
    FromCursor      int64
    ToCursor        int64
    Upserts         map[string]Row
    DeleteKeys      []string
    Version         TableVersion
    Definition      TableDefinition
    Subscriptions   []Subscription
    Rollout          RolloutBundle
    ReplaceMetadata bool
    ReplaceRollout  bool
    DropTable        bool
    NeedFullSync    bool
    ErrorCode        string
}

type SyncByCommandResponse struct {
    SnapshotGeneration uint64
    Deltas             []TableCommandDelta
}
```

响应允许表级部分成功。每张表独立携带结果；一个失败表不得阻止其他表推进。

### 8.7 `WatchChanges`

Client 建立服务端流并周期发送心跳。Server 返回去重排序后的 changed tables 和 Snapshot generation。提示不携带配置内容，Client 收到后调用 `SyncByCommand`。

### 8.8 `RequestRefresh`

Admin Outbox worker 调用独立的 Refresh Control service：

```go
type RequestRefreshRequest struct {
    Environment   string
    EventID       string
    ReleaseNumber string
    Tables        []string
}
```

相同 EventID 幂等接受。接受只表示刷新任务已排队，不表示 Snapshot 已更新。

### 8.9 错误码

| code | 语义 |
|---|---|
| `OK` | 成功 |
| `INVALID_ARGUMENT` | 参数非法 |
| `ENVIRONMENT_NOT_FOUND` | 环境不存在或未启用 |
| `CONSUMER_NOT_FOUND` | 该环境内无启用订阅 |
| `TABLE_NOT_SUBSCRIBED` | 请求未订阅表 |
| `CURSOR_AHEAD` | Client cursor 超过 Server Snapshot cursor |
| `COMMAND_GAP` | Command 已清理或窗口不连续 |
| `FULL_SYNC_REQUIRED` | 必须单表全量 |
| `SNAPSHOT_NOT_READY` | Server 尚无可用快照 |
| `RESPONSE_TOO_LARGE` | 超出响应限制 |
| `INTERNAL` | 内部错误 |

### 8.10 Protobuf service

```proto
service RuntimeConfigService {
  rpc GetSnapshot(GetSnapshotRequest) returns (GetSnapshotResponse);
  rpc GetTables(GetTablesRequest) returns (GetTablesResponse);
  rpc GetMetadata(GetMetadataRequest) returns (GetMetadataResponse);
  rpc SyncByCommand(SyncByCommandRequest) returns (SyncByCommandResponse);
  rpc WatchChanges(stream WatchRequest) returns (stream WatchResponse);
}

service RefreshControlService {
  rpc RequestRefresh(RequestRefreshRequest) returns (RequestRefreshResponse);
}
```

字段号冻结后只能追加，禁止复用或改变语义。

## 9. 刷新事件契约

### 9.1 MySQL Outbox

默认事件类型 `SERVER_REFRESH_REQUESTED`。payload 只包含 environment、event ID、release number 和受影响表。Server 不信任 Outbox payload 中的版本或行内容；它从该环境的版本表和 Command 读取权威变化，但 ROW 增量不得回查 Managed Table。

### 9.2 版本轮询

Server 默认每 15 秒按 Environment 读取 `rcc_table_versions`。任一 revision 或 command_cursor 与对应环境的 Snapshot 不同，生成 TABLES 或 COMMAND RefreshPlan。

### 9.3 人工刷新

管理员可通过受保护的 Admin 接口创建 Outbox 事件请求 ALL 或指定表刷新。Server 不暴露公网刷新端点。

### 9.4 刷新计划

```go
type RefreshPlan struct {
    Environment string
    Mode      string // ALL / TABLES / COMMAND
    Trigger   string // STARTUP / OUTBOX / POLL / MANUAL / RECOVERY
    Tables    []string
    EventID   string
}
```

## 10. 快照规格

### 10.1 逻辑结构

```go
type Snapshot struct {
    Environment     string
    Generation      uint64
    CreatedAt       time.Time
    Tables          map[string]*TableSnapshot
    ConsumerTables  map[string][]string
}

type TableSnapshot struct {
    Definition      TableDefinition
    Rows            map[string]RowRef
    Subscriptions   map[string][]Subscription
    Rollout          RolloutBundle
    Version          TableVersion
    DeltaHistory     ResolvedDeltaHistory
    CompressedRows   []byte
}

type ResolvedDeltaHistory struct {
    OldestCursor int64 // 可服务区间的左开边界，不一定等于首条保留 Command ID
    Commands     []ResolvedCommand // ID 严格递增
    Bytes        int64
}

type ResolvedCommand struct {
    ID           int64
    ResourceKind string
    Action       string
    ConfigKey    string
    FinalRow     Row // ROW ADD/MODIFY；其他情况为空
}
```

### 10.2 不可变规则

发布后所有 map、slice、RowRef 和压缩字节不可修改。COW 只能复用完全不变的 TableSnapshot；变更表构建新对象。

`DeltaHistory` 保存从某个 `OldestCursor` 到当前 `Version.CommandCursor` 的已校验 Command 摘要和最终载荷，按内存字节、命令数和保留时间设硬上限。它与表数据在同一次 Snapshot Store 中发布。`SyncByCommand` 只读这段历史，不访问 Command 表或 Managed Table。

### 10.3 发布前不变量

- table map key 与 Definition.TableName 一致；
- 每行 ConfigKey 与行内 `id` 一致；
- 无重复 ConfigKey；
- Subscription 字段存在；
- Rollout Rule 引用有效 Policy，名单无冲突；
- Data/Rollout digest 重算一致；
- Command cursor 不倒退且不超过数据库版本 cursor；
- 压缩解码后与 Rows 等价。

## 11. 稳定编码、压缩与摘要

### 11.1 正式数据编码

表行按 ConfigKey 字节序升序，每行字段按字段名升序，编码为 JSON 数组：

```json
[
  {"key":"1","value":[["id","1"],["status","ACTIVE"]]}
]
```

### 11.2 压缩

使用 LZ4 Frame。实现必须流式编码和解码，限制单表未压缩字节数，拒绝尾随数据和未结束 JSON。

### 11.3 摘要

`DataDigest = SHA-256(canonical_table_json)`。`RolloutDigest` 对 Policy、Rule 和 SubjectOverride 的规范排序编码计算 SHA-256。

### 11.4 Command 规范编码与校验和

TableDefinition 按列 ordinal position、名称、类型、nullable、默认值和 key 信息规范编码，`SchemaDigest = SHA-256(canonical_table_definition)`。

ROW Command 的校验信封固定为以下字段的规范 JSON：

```json
{
  "environment": "test-a",
  "table_name": "merchant_channel_config",
  "resource_kind": "ROW",
  "action": "MODIFY",
  "config_key": "42",
  "schema_digest": "...",
  "config_value": {"id":"42","status":"ACTIVE"}
}
```

对象字段名按字节序排列；Row 字段按名称排序；数字、布尔、时间、JSON 和 NULL 使用 Schema Codec 的字符串表示。`PayloadChecksum = lowercase_hex(SHA-256(canonical_command_envelope))`，不包含尚未生成的 Command ID。Server 应用前必须重算并比较。

### 11.5 分桶算法标识

内置 `sha256-v1` 的算法固定为：

```text
input = allocation_seed + 0x1f + allocation_group + 0x1f + subject_key
bucket = big_endian_uint64(SHA-256(input)[0:8]) % 10000
```

Server 不执行自定义算法；算法名称原样分发。

## 12. 查询与同步算法

### 12.1 全量查询

捕获一次 Snapshot View，按消费者订阅选表，返回表数据、元信息、Rollout 和各表 cursor。禁止在组装过程中重新加载快照。

### 12.2 指定表全量

从同一 Snapshot View 返回所有请求表。缺表按订阅删除语义返回 DropTable；未知或未订阅表返回错误。该 RPC 只是从 Server 内存发送完整 TableSnapshot，不触发 Server TABLES 数据库恢复。

### 12.3 `SyncByCommand` 核心算法

Server 必须先根据 `consumer_id` 计算当前订阅表集合，而不能只遍历请求中的 `CursorMap`。当前已订阅但请求未携带游标的表，说明 Client 尚无可衔接基线，必须返回 `NeedFullSync=true`；已取消订阅但仍出现在 `CursorMap` 中的表返回 `DropTable=true`。因此新增订阅表不会错误地从游标 0 依靠不完整的 Command 历史重建。

对每张请求表：

1. 捕获的 Snapshot 中读取 `targetCursor`。
2. 若 `clientCursor == targetCursor`，返回空 delta。
3. 若 `clientCursor > targetCursor`，返回 `NeedFullSync=true` 和 `CURSOR_AHEAD`。
4. 检查捕获快照中 DeltaHistory 的 `OldestCursor`；若历史不覆盖 `clientCursor`，返回 `NeedFullSync=true`，不得现场访问数据库补历史。
5. 从捕获 TableSnapshot 的 `DeltaHistory` 读取 `clientCursor < id <= targetCursor` 的已解析 Command；此步骤不得访问数据库。
6. 验证 ID 严格递增且位于请求区间内，并校验表名、Action 和 ResourceKind；ID 是全局自增值，允许因其他表 Command 出现空洞。
7. 对 ROW Command 按 ConfigKey 保留窗口内最后一条。
8. 最终 DELETE 写入 DeleteKeys；ADD/MODIFY 从捕获的 TableSnapshot 读取当前最终 Row 写入 Upserts。
9. 出现 TABLE_META 时返回完整 Definition/Subscriptions 并设置 ReplaceMetadata。
10. 出现 ROLLOUT 时返回完整 RolloutBundle 并设置 ReplaceRollout。
11. 返回 `FromCursor=clientCursor`、`ToCursor=targetCursor` 和捕获快照的 Version。

该范围查询是内存查询。带上下界的 SQL 只允许出现在 Server 自身的刷新构建路径：

```sql
SELECT id, table_name, resource_kind, action, config_key, config_value,
       schema_digest, payload_checksum
FROM rcc_increment_commands
WHERE table_name = ? AND id > ? AND id <= ?
ORDER BY id ASC;
```

禁止在 Client RPC 中执行这条 SQL，也禁止只查询 `id > startCursor`。前者会让数据库压力随 Client 数量放大，后者可能越过本轮刷新目标游标。

### 12.4 Delta 限制

默认单表最多 10,000 条 Command、10,000 个最终 Key、16 MiB 未压缩响应。超过任一限制返回 `NeedFullSync=true`，由 Client 调用 `GetTables`。

### 12.5 最终态校验

最终 ADD/MODIFY 的 ConfigKey 在捕获快照中不存在、Row ID 不一致或同一 Key 产生无法解释状态时，返回单表全量要求。Client 响应只输出快照最终行，不直接透传 Command ConfigValue。

## 13. Server 刷新模型与算法

### 13.1 强类型计划

ALL 构建全部表；TABLES 重建指定表；COMMAND 对每表消费 `(snapshotCursor, databaseVersionCursor]`。

### 13.2 全量刷新

在 REPEATABLE READ 只读事务中读取启用表集合、版本、实时 Schema、行、订阅和 Rollout。任一表非法时启动失败；运行期 ALL 失败保留旧快照。

### 13.3 指定表刷新

每张表独立构建并验证。多个成功表可以一次 generation 发布；失败表复用完整旧 TableSnapshot。

### 13.4 Command 刷新

Server 按表、按批查询自身 cursor 到数据库版本 cursor 的 Command。它必须先校验窗口内每一条 Command 的 SchemaDigest、PayloadChecksum、ConfigKey、Action 和完整字段集合，再按 ConfigKey 聚合最终动作；不得因为某条坏命令后来被覆盖而跳过校验。ROW ADD/MODIFY 直接应用 `ConfigValue`；DELETE 应用 tombstone。整个 ROW 增量过程不得查询 Managed Table。元信息和 Rollout 读取控制表最终态并替换完整 bundle。

每次成功应用同时把已校验 Command 追加到新 TableSnapshot 的 `DeltaHistory`，再与 Rows、元信息、Rollout、Version 和 cursor 一次性发布。多个 Client 请求复用同一段历史，不重复解析 Command，也不产生数据库读。

启动全量后，Server 可以用一次有界查询加载仍在保留期内、且不超过启动 Snapshot cursor 的 Command 以预热 DeltaHistory。预热失败不影响全量 Snapshot Ready，但历史覆盖不到的 Client cursor 必须返回 NeedFullSync。

### 13.5 断档恢复

Server 自身 cursor 早于 watermark、未知 Action、SchemaDigest/PayloadChecksum/载荷校验失败或 digest 校验失败时，可以申请该表 TABLES 数据库恢复。MySQL 暂时不可用、锁等待或限流拒绝属于可重试错误，必须退避重试 Command，不得立即放大为全表读取。

Client cursor 只是在 DeltaHistory 中无法覆盖时收到 `NeedFullSync`，随后调用 `GetTables` 从现有 Server Snapshot 获取完整表；该路径不申请数据库恢复。Server 数据库恢复与 Client 网络全量必须使用不同指标和限流名称。

### 13.6 容量保护

刷新协调模块必须隐藏以下容量控制：

- 每表单轮 Command 数、唯一 ConfigKey 数和载荷字节上限；
- Command 分批读取和分批推进 Snapshot cursor；
- 全局数据库查询并发、QPS 和批次大小限制；
- 同表刷新事件合并与 singleflight；
- 失败指数退避和随机抖动；
- TABLES/ALL 全量恢复使用独立 semaphore、QPS、冷却时间和内存预算；
- 主库或控制库过载时优先保留旧快照，禁止增量失败立即转整表读取。

默认可信 Command 路径的 Managed Table 增量回源次数为 0。数据库仍承担版本和 Command 顺序读取，以及启动/明确恢复时的全量读取；相应流量必须分别计量。

容量模型因此变为：发布端对实际变更唯一 Key 执行一次批量写后读取；每个 Server 实例顺序读取 Command 日志并在本地物化一次；任意数量 Client 只复用内存 DeltaHistory。Managed Table 增量读不再乘以 Server 实例数、Client 数量或消费重试次数。若未来 Server 实例导致 Command 控制表读取也成为瓶颈，可以在同一 `CommandDeltaResolver` 接缝增加共享物化适配器，但它不属于第一版核心依赖。

### 13.7 部分成功

表 A 成功、表 B 失败时可发布新 generation：A 使用新 TableSnapshot，B 完整复用旧对象。RefreshResult 明确列出成功、失败、恢复和真正变化表。

### 13.8 幂等

重复 Outbox event、重复版本轮询或相同 RefreshPlan 不得造成 cursor 回退、重复行、重复通知或 generation 无意义增长。

## 14. Stream 与通知规格

### 14.1 客户端身份

`environment + consumer_id + client_id` 唯一标识连接。同一 ClientID 可以存在于不同测试环境，但不能共享连接状态、游标或通知队列。

### 14.2 连接状态

每连接有有界发送队列、最后心跳、取消函数和连续 drop 计数。同一身份重连原子替换旧连接。

### 14.3 受众规则

RefreshResult 的 changed tables 与消费者订阅求交。每个目标连接一次提示，表名去重排序。

### 14.4 背压

通知不阻塞刷新。队列满时丢提示并计数；连续超阈值关闭连接。Client 仍通过周期 `SyncByCommand` 收敛。

### 14.5 清理

发送失败、上下文取消、心跳超时和 Server 关闭都必须注销连接并结束 goroutine。

## 15. 并发与内存模型

1. 查询只执行 atomic Load。
2. 每个请求只持有一个 Snapshot View。
3. 刷新全局串行；构建阶段可按表有界并行。
4. 候选 Snapshot 发布前不可见，发布后不可修改。
5. 压缩、Rows 和派生索引按容量预算计入构建估算。
6. 预计峰值超过 `max_snapshot_build_bytes` 时提前失败。

## 16. 生命周期与就绪

### 16.1 启动

`STARTING -> READY` 或 `STARTING -> FAILED`。启动全量成功并通过 Validator 后才能 Ready。

### 16.2 运行期

Outbox refresh、版本轮询、Stream 和清理任务在统一 supervisor 下运行，panic 被恢复、记录并按策略重启。

### 16.3 就绪与健康

进程存活不等于 Ready。Snapshot age 超过最大陈旧时间、版本轮询长期失败或没有 Snapshot 时 Ready=false，已有查询可继续读取 last-known-good。

### 16.4 关闭

停止接流量，取消后台任务和 Stream，在超时内等待 goroutine 归零，再关闭 DB 和 gRPC。

## 17. 配置规格

```yaml
server:
  max_snapshot_staleness: 30m
  max_snapshot_build_bytes: 8GiB

database:
  dsn_ref: env:RCC_SERVER_DATABASE_DSN
  timezone: UTC
  isolation_level: REPEATABLE_READ
  page_size: 1000

refresh:
  mode: full_only       # full_only | shadow | command
  version_poll_interval: 15s
  command_retention: 168h
  command_fetch_batch_size: 1000
  command_fetch_concurrency: 4
  command_fetch_qps: 20
  command_payload_bytes_per_batch: 16MiB
  command_retry_min_backoff: 1s
  command_retry_max_backoff: 1m
  max_commands_per_delta: 10000
  delta_history_max_commands_per_table: 100000
  delta_history_max_bytes_per_table: 256MiB
  full_sync_concurrency: 1
  full_sync_qps: 0.1
  full_sync_cooldown: 5m
  full_sync_start_jitter: 30s
  expected_max_server_instances: 10

stream:
  queue_capacity: 100
  heartbeat_timeout: 2m
  cleanup_interval: 30s

rpc:
  listen_address: 0.0.0.0:8081
  max_request_bytes: 8MiB
  max_response_bytes: 256MiB
```

配置错误阻止启动。DSN 只通过环境变量或文件引用提供，不进入控制表。数据库 QPS 配置是单实例预算，必须根据 `expected_max_server_instances` 校验，使所有实例理论总预算不超过数据库容量评估值；扩容 Server 前必须同步调整预算。

## 18. 可观测性与 SLO

### 18.1 SLO

| 目标 | SLO |
|---|---|
| 查询可用性 | 月度 ≥ 99.95% |
| Outbox 通知下提交到 Snapshot | P99 ≤ 5 秒 |
| 轮询兜底提交到 Snapshot | P99 ≤ 30 秒 |
| Command Client 同步 | P99 ≤ 10 秒 |
| cursor 倒退 | 0 |
| 校验失败后错误发布 | 0 |

### 18.2 指标

```text
rcc_server_snapshot_generation
rcc_server_snapshot_age_seconds
rcc_server_snapshot_rows{table}
rcc_server_refresh_total{mode,trigger,result}
rcc_server_refresh_duration_seconds{mode}
rcc_server_command_cursor{table}
rcc_server_command_lag{table}
rcc_server_command_fetch_total{table,result}
rcc_server_command_fetch_rows{table}
rcc_server_command_payload_invalid_total{table,reason}
rcc_server_managed_table_refresh_reads_total{mode,reason}
rcc_server_delta_history_commands{table}
rcc_server_delta_history_bytes{table}
rcc_server_delta_history_oldest_cursor{table}
rcc_server_sync_command_total{result}
rcc_server_sync_command_rows{operation}
rcc_server_sync_full_fallback_total{reason}
rcc_server_full_sync_throttled_total{reason}
rcc_server_stream_clients
rcc_server_notify_total{result}
```

### 18.3 日志

刷新日志包含 request_id、event_id、release_number、trigger、mode、tables、base/new generation、old/new cursor、published、duration 和 error code。禁止记录行内容、SubjectKey、名单、DSN 和凭据。

### 18.4 告警

Snapshot stale、cursor 倒退、Command gap 激增、Command 载荷校验失败、Managed Table 增量回源非零、DeltaHistory 覆盖持续缩短、全量兜底率异常、全量限流积压、Validator 失败、Outbox 刷新连续失败、Stream drop 比例和后台任务失联均需告警。

### 18.5 版本大盘状态

每个 Server 实例 MUST 为各启用 Environment 的当前已发布 Snapshot 提供表级版本状态，至少包含环境编码、稳定节点标识、表名、DB `source_updated_at`、数据 MD5、缓存加载时间、revision 和 Command cursor。状态只能在对应 TableSnapshot 完成校验并原子发布后更新；刷新失败时继续报告旧缓存状态。

该状态用于 Admin 版本大盘比较 DB 目标版本与各 Server 节点的实际缓存版本。精确状态不得只存在于日志或 Prometheus 标签中。跨节点采集协议、状态存储归属和历史保留策略在实现前另行确认。

## 19. 安全要求

- DB 账号只读且限制到一个 database。
- 动态表名只来自启用 Table Policy。
- SQL 值参数化；标识符经实时 Schema。
- gRPC 默认部署在受信网络，生产 SHOULD 启用 TLS/mTLS 或服务网格身份。
- Refresh Control service 与 Runtime service 使用独立授权。
- 请求、响应、压缩输出、表数、命令数和字符串长度有硬上限。
- 不记录配置内容和 SHA-256 名单；哈希不能被当作完全匿名数据。
- Command ConfigValue 含完整业务配置，数据库权限、备份、审计导出和诊断接口必须按正式数据同等级保护。

## 20. 部署、灰度与回滚

### 20.1 拓扑

每实例持有完整 Snapshot。查询经普通负载均衡；版本轮询加入 jitter。无消息队列。

### 20.2 资源预置

先建控制表和索引，再写订阅与版本，创建只读账号，部署 Server 完成启动全量，最后接入 Client。

若从不可信 ConfigValue 的旧 Command 迁移，禁止为历史载荷伪造 SchemaDigest 或 PayloadChecksum。迁移必须记录 cutover cursor，以 Managed Table 做一次受限全量建立新基线，只把 cutover 之后由新版 Publication Committer 生成的 Command 视为可信；旧 Client 或游标早于新历史边界时执行内存 GetTables 全量。

### 20.3 刷新引擎灰度

- `full_only`：所有变化按表全量。
- `shadow`：full_only 发布，可信 Command 候选只比较；记录校验和、Schema 和数据摘要差异。
- `command`：使用 Command 增量并保留立即回切。

连续 24 小时 shadow 的数据、元信息、Rollout、摘要和 cursor 零不可解释差异后切 command。

### 20.4 回滚

Validator 失败、cursor 倒退、Command gap 异常、载荷校验失败、摘要差异或资源超限时切回 full_only。回切受独立全量限流保护，不能让所有实例同时扫描大表。协议回滚不得丢弃已持久化 Command 或降低版本。

## 21. 测试与验收规格

### 21.1 单元测试面

围绕 `Refresh`、`SnapshotCatalog.View`、`RuntimeQuery` 和 `NotificationHub` 测试，不通过修改内部 map 构造状态。

### 21.2 必测用例

- 启动全量、重复 ConfigKey、非法 Schema、压缩 golden；
- 查询期间发布新 Snapshot 仍返回单一 generation；
- Server Command ADD/MODIFY/DELETE 直接重放，无 Managed Table 回查；
- 数据库 default、trigger、generated column 结果已进入 Command ConfigValue；
- SchemaDigest、PayloadChecksum、字段全集、ConfigKey 和 tombstone 校验；
- 100/1,000 个并发 Client 调用 SyncByCommand 不增加数据库查询次数；
- 同表事件合并、Command 分批、QPS/并发限制和失败退避；
- 临时数据库错误不触发立即全量，全量恢复遵守独立限流；
- 表级部分成功、失败表所有字段保旧；
- `SyncByCommand` 上界严格等于捕获 Snapshot cursor；
- DB 已有更大 Command 时不返回上界外数据；
- 多个 Command 同 Key 只返回最终态；
- Client cursor ahead、watermark 断档和窗口超限触发单表全量；
- 元信息和 Rollout Command 返回 replacement bundle；
- 重复请求返回幂等 delta；
- Stream 重连、背压、心跳和关闭无泄漏；
- 多个测试环境的 Snapshot、Command cursor、订阅、Stream 和版本状态严格隔离，生产环境只有一个；
- race detector、故障注入和 3 倍容量测试。

### 21.3 故障注入

覆盖 MySQL 短暂不可用、Outbox 丢通知、Server 重启、Command 清理、Command 载荷损坏、SchemaDigest 不匹配、DeltaHistory 过短、全量限流、压缩损坏、OOM 预估超限、客户端慢和 gRPC 中断。

### 21.4 性能门禁

以最大生产样本的 1x/2x/3x 测试全量构建、100/1,000/10,000 Command、批量发布期间数据库读 QPS、不同 Server/Client 实例数下的读放大系数、SyncByCommand 网络字节、查询 P99、峰值 RSS 和 GC pause。验收要求 Client 数量变化不增加 MySQL 查询次数，ROW Command 增量的 Managed Table 回源为 0。

## 22. 实施拆分

1. Protobuf、codec golden、控制表和 fixtures。
2. SnapshotCatalog、全量刷新和 GetSnapshot/GetTables。
3. RuntimeQuery、订阅和 WatchChanges。
4. 可信最终态 Command、校验和、Server 重放与 shadow。
5. ResolvedDeltaHistory、SyncByCommand、watermark 和单表全量兜底。
6. 分批、限流、singleflight、退避、独立全量限流和容量指标。
7. 故障注入和 command 模式灰度。
8. Server 表级版本状态采集和版本大盘联调。

## 23. 完成定义

- 所有 FR 有自动化验收证据。
- Protobuf 兼容检查和 codec golden 通过。
- `SyncByCommand` 不可能返回超过 Snapshot cursor 的 Command。
- `SyncByCommand` 和其他 Client RPC 不访问 MySQL。
- ROW Command 增量不回查 Managed Table，载荷校验失败时不推进 cursor。
- 数据、元信息、Rollout、版本和 cursor 原子发布。
- Command 断档只恢复受影响表。
- race、故障注入和容量门禁通过。
- full_only 回切和人工全量恢复演练完成。
- Server 表级版本状态只在 Snapshot 原子发布后变化，版本大盘能够识别落后节点和相同版本时间下的摘要不一致。
- 运维手册覆盖 stale、gap、payload invalid、history coverage、fallback throttling、stream drop 和高 RSS。

## 24. 需求追踪矩阵

| 需求 | 章节 | 主要证据 |
|---|---|---|
| FR-001 | 10、13、16 | 启动和 Validator 测试 |
| FR-002 | 8、12 | 全量 RPC golden |
| FR-003 | 8.6、12.3 | Command 范围和最终态测试 |
| FR-004 | 10、12 | 单 generation 并发测试 |
| FR-005 | 13.4 | Server Command COW 测试 |
| FR-006 | 7.5、13.5 | watermark 和恢复测试 |
| FR-007 | 9、14 | 丢通知与 Stream 测试 |
| FR-008 | 6.6、6.7、11.5 | Rollout golden |
| FR-009 | 13.7 | 部分成功测试 |
| FR-010 | 15～20 | 生命周期、容量和运维验收 |
| FR-011 | 10、12.3、13.4 | RPC 数据库查询计数为零 |
| FR-012 | 6.8、7.4、11.4、13.4 | 可信载荷重放与校验测试 |
| FR-013 | 10、18.5 | Snapshot 原子发布与版本状态联动测试 |

## 附录 A：端到端示例

### A.1 正式 MODIFY

Admin 在同一事务内更新 id=42、读取写后完整行、写入带 SchemaDigest/PayloadChecksum 的 ROW/MODIFY Command、推进版本 cursor 并写 Outbox。Server 校验并直接重放 ConfigValue，不回查 Managed Table，同时把 ResolvedCommand 发布进 DeltaHistory。Client 提交旧 cursor，Server 从捕获 Snapshot 的内存历史返回 id=42 最终行；Client COW 后原子推进表 cursor。

### A.2 灰度 Policy 变化

Admin 更新 Rollout Policy 和 SHA-256 名单，为引用表写 ROLLOUT/MODIFY Command。Server 替换 RolloutBundle 并通知订阅 Client。SyncByCommand 返回 ReplaceRollout；Client 原子替换规则，业务下一次灰度查询使用新规则。

### A.3 Command 过期

Client cursor 小于 watermark。Server 返回 `NeedFullSync=true`，Client 调用 GetTables 只恢复该表，并将完整数据与响应 cursor 一次性保存。

## 附录 B：闭源参考置换

闭源参考中的 Thrift、PSM、Stage、Region、RMQ、TCC 和 Binlog 模式不进入本规格。闭源 Env 被开源 Environment 模型替代：允许多个测试环境、只允许一个生产环境，Client 必须显式选择。保留的是不可变快照、Command COW、表级 cursor、失败保旧、版本轮询和 Stream 提示等行为契约；原先消费端最终态回查被可信最终态 Command 和可复用 DeltaHistory 取代。
