# 发布单开源实施技术规格

> 规格版本：1.0.0
> 状态：Implementation Candidate
> 所属上下文：Admin
> 默认依赖：MySQL 8.0+、Gin、GORM
> 外部依赖：无；审批、可靠任务和通知均由项目自身实现。

## 1. 文档定位

本文定义开源版配置发布单。发布单将 Environment、Managed Table 变更、简单人工审批、正式数据写入、百分比灰度规则、版本、Command 和 MySQL Outbox 组织为一个可恢复流程。本文不兼容闭源版 Thrift、PSM、内部审批、RMQ、TCC、ChangeGate 或内部错误码。

强度词：MUST 表示实现和上线必须满足；SHOULD 表示默认满足，偏离必须记录理由；MAY 表示可选。

## 2. 业务目标与范围

### 2.1 目标

发布单 MUST：

1. 用唯一单号关联申请人、Page Model、变更内容、审批、执行节点、日志和 Outbox。
2. 支持单笔和批量 ADD/MODIFY/DELETE。
3. 保证同一 `environment + model_code + release_target` 同时最多一张在途单。
4. 将业务数据、版本、Command、发布状态和 Outbox 纳入可恢复的一致性边界。
5. 将百分比灰度策略、显式名单和正式发布作为模板节点。
6. 保证所有会影响 Server/Client 快照的变更都有 Command 记录。
7. 提供版本大盘，统一展示 DB 目标版本、各 Server 缓存版本和各 Client 缓存版本。

### 2.2 纳入范围

- 创建、查询、审批、执行、推进、回滚和取消；
- 模型与模板快照；
- 内置 RBAC 和简单人工审批；
- 正式配置和灰度规则；
- MySQL Outbox worker；
- Command、版本和 Server 刷新通知；
- 审计、重试、死信和人工重放。

### 2.3 范围边界

- 第一版单租户。
- 环境分为 `TEST` 和 `PRODUCTION`：允许多个测试环境，全系统只允许一个生产环境，并至少各配置一个。
- 所有发布单在创建时绑定一个环境，后续审批、执行、回滚、Command、版本和 Outbox 不得改变或跨越该环境。
- Admin 信任反向代理注入且经过签名或网络边界保护的身份头。
- 不提供通用 BPMN、脚本节点、任意表达式或第三方审批产品集成。
- 不在数据库事务中执行网络调用。
- 发布单不创建或修改业务表 Schema。
- Managed Table 运行态数据禁止绕过 Publication Committer 写入；导入、回滚和运维修复也必须走同一接口。

## 3. 术语

| 术语 | 定义 |
|---|---|
| Release Model | Page Model 对应的发布字段、目标和 Mutation Policy 投影 |
| Release Type | 选择流程模板的稳定编码 |
| Release Target | 从模型声明字段计算的稳定业务目标 |
| Environment | 发布单绑定的隔离空间；多个 `TEST` 环境使用不同编码，`PRODUCTION` 类型全系统唯一 |
| Release Template | 有序节点和节点参数的版本化定义 |
| Release Order | 一次配置变更的权威流程记录 |
| Rollout Policy | 百分比、分桶算法和显式名单组成的灰度受众 |
| Command | 记录某表某 ConfigKey 变化的单调 ID 事件 |
| Canonical Final Row | 业务写入完成后、提交前按实时 Schema 读取并规范编码的完整最终行 |
| Outbox | 与业务事务一起写入、由 worker 可靠投递的任务 |
| Active Target | 对在途目标的唯一占位 |

## 4. 角色、权限与核心用例

### 4.1 角色

| 角色 | 权限 |
|---|---|
| `VIEWER` | 查看有权限的模型和发布单 |
| `EDITOR` | 创建发布单、编辑尚未提交的内容 |
| `APPROVER` | 批准或拒绝待审批发布单；不能审批自己创建的单据 |
| `PUBLISHER` | 执行已批准发布单和回滚 |
| `ADMIN` | 管理模型、模板、角色和死信；高危操作仍审计 |
| `OUTBOX_WORKER` | 租约和投递 Outbox，不改变发布内容 |

主体由可信认证上下文提供。请求中的展示名称不得参与授权。

### 4.2 核心用例

- Editor 创建单笔或批量发布单。
- Approver 查看 Diff 并批准或拒绝。
- Publisher 执行批准后的正式或灰度发布。
- Admin 查看诊断、重放死信或取消异常在途单。
- Outbox worker 通知 Server 刷新并可靠重试。

## 5. 模块职责

| 模块 | 接口职责 | 隐藏的实现复杂度 |
|---|---|---|
| Release Definition | 解析模型、类型和模板 | 配置校验、版本和快照 |
| Release Order | Create/Detail/Act/Approve | 状态机、权限、事务和错误映射 |
| Publication Committer | Commit Change Set | Mutation、写后最终行、可信 Command、版本、日志、Outbox |
| Outbox Delivery | Lease/Dispatch/Retry | 租约、退避、死信和幂等 |
| Server Refresh Port | 请求 Server 刷新 | gRPC 适配器与测试内存适配器 |

## 6. 功能清单

| 功能 | 必要行为 |
|---|---|
| 环境隔离 | 允许多个测试环境、只允许一个生产环境；发布单创建后环境不可变 |
| 创建 | 校验模型、内容、权限和目标，冻结模型/模板，抢占 Active Target |
| 查询 | 返回状态、节点、Diff、审批、日志、Outbox 摘要和可执行动作 |
| 审批 | 内置 approve/reject，禁止自批，幂等并记录意见 |
| 执行 | 行锁内重验状态和版本，调用 Publication Committer |
| 灰度 | 保存 Rollout Policy、规则和名单，并写目标表 Command |
| 正式发布 | 修改 Managed Table，读取写后最终行，更新版本并写可信行级 Command |
| 回滚 | 使用冻结 before state 生成反向变更，作为新的事务提交 |
| 可靠通知 | MySQL Outbox 至少一次调用 Server 刷新接口 |
| 诊断 | 展示发布、Command、版本和 Outbox 的关联状态 |
| 版本大盘 | 按 Managed Table 展示 DB、Server 和 Client 三层版本，并识别落后、摘要不一致和状态未知 |

## 7. HTTP 接口规格

### 7.1 通用约定

- 所有写请求必须有 `Idempotency-Key`。
- 身份来自认证上下文。
- 成功使用 2xx；业务冲突使用 409；状态或内容校验失败使用 422。
- 错误响应包含稳定 `code`、`message` 和 `request_id`。

### 7.2 创建

```http
POST /api/v1/release-orders
Idempotency-Key: 01J...
```

```json
{
  "environment": "test-a",
  "model_code": "merchant_channel",
  "release_type": "standard",
  "description": "enable channel",
  "changes": [
    {
      "action": "MODIFY",
      "id": "42",
      "before": {"status": "DISABLED"},
      "after": {"status": "ACTIVE"}
    }
  ],
  "rollout": null
}
```

创建响应返回 `release_number`、状态、当前节点和版本。

### 7.3 查询详情

```http
GET /api/v1/release-orders/{release_number}
```

详情包含内容摘要、完整节点快照、审批记录、操作日志、Outbox 摘要和当前主体可执行动作。

### 7.4 执行动作

```http
POST /api/v1/release-orders/{release_number}:act
Idempotency-Key: 01J...
```

```json
{"action":"EXECUTE","expected_version":3,"comment":"ready"}
```

动作只允许 `EXECUTE`、`ADVANCE`、`ROLLBACK`、`CANCEL`。

### 7.5 审批

```http
POST /api/v1/release-orders/{release_number}:approve
POST /api/v1/release-orders/{release_number}:reject
Idempotency-Key: 01J...
```

请求必须携带 `expected_version` 和非空意见。创建人不得审批自己的发布单；ADMIN 也不绕过该分离规则，紧急流程应使用显式应急模板并审计。

### 7.6 Outbox 管理

```http
GET  /api/v1/outbox-events?status=DEAD_LETTER
POST /api/v1/outbox-events/{event_id}:replay
```

仅 ADMIN 可重放；重放不创建新业务结果，只重置投递状态并记录审计。

### 7.7 错误码

| code | HTTP | 语义 |
|---|---:|---|
| `RELEASE_NOT_FOUND` | 404 | 发布单不存在 |
| `RELEASE_CONFLICT` | 409 | Active Target 或幂等摘要冲突 |
| `RELEASE_STALE_VERSION` | 409 | expected version 过期 |
| `RELEASE_ACTION_NOT_ALLOWED` | 422 | 当前状态不允许动作 |
| `RELEASE_PERMISSION_DENIED` | 403 | 权限不足 |
| `RELEASE_SELF_APPROVAL_FORBIDDEN` | 403 | 创建人尝试审批自己单据 |
| `RELEASE_CONTENT_INVALID` | 422 | 内容、模板或灰度规则非法 |
| `OUTBOX_NOT_REPLAYABLE` | 409 | 事件不处于可重放状态 |

## 8. 数据模型与索引约束

### 8.1 主表

```sql
CREATE TABLE `rcc_release_orders` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_number` varchar(64) NOT NULL,
  `environment` varchar(64) NOT NULL,
  `model_code` varchar(100) NOT NULL,
  `release_type` varchar(64) NOT NULL,
  `release_target` varchar(500) NOT NULL,
  `status` varchar(32) NOT NULL,
  `current_node` varchar(64) NOT NULL,
  `version` bigint unsigned NOT NULL DEFAULT 1,
  `creator` varchar(128) NOT NULL,
  `description` varchar(2000) NOT NULL DEFAULT '',
  `model_snapshot` json NOT NULL,
  `template_snapshot` json NOT NULL,
  `node_context` json NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_release_number` (`release_number`),
  KEY `idx_release_model_status` (`environment`, `model_code`, `status`, `gmt_modified`),
  CONSTRAINT `chk_release_status` CHECK (`status` IN (
    'DRAFT','PENDING_APPROVAL','APPROVED','IN_PROGRESS','SUCCEEDED','REJECTED','ROLLED_BACK','CANCELLED','FAILED'
  ))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 8.2 变更明细

```sql
CREATE TABLE `rcc_release_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_number` varchar(64) NOT NULL,
  `item_index` int unsigned NOT NULL,
  `action` varchar(16) NOT NULL,
  `config_key` varchar(255) NOT NULL,
  `content_before` json NULL,
  `content_after` json NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_release_item` (`release_number`, `item_index`),
  KEY `idx_release_item_key` (`release_number`, `config_key`),
  CONSTRAINT `chk_release_item_action` CHECK (`action` IN ('ADD','MODIFY','DELETE'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8.3 Active Target

```sql
CREATE TABLE `rcc_release_active_targets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `environment` varchar(64) NOT NULL,
  `model_code` varchar(100) NOT NULL,
  `release_target_hash` char(64) NOT NULL,
  `release_number` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_active_target` (`environment`, `model_code`, `release_target_hash`),
  UNIQUE KEY `uk_active_release` (`release_number`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

终态事务必须删除占位。巡检发现终态残留时告警，不得自动删除未经核对的占位。

### 8.4 操作日志

```sql
CREATE TABLE `rcc_release_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_number` varchar(64) NOT NULL,
  `sequence` bigint unsigned NOT NULL,
  `principal` varchar(128) NOT NULL,
  `action` varchar(64) NOT NULL,
  `from_status` varchar(32) NOT NULL,
  `to_status` varchar(32) NOT NULL,
  `summary` json NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_release_log_sequence` (`release_number`, `sequence`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8.5 Outbox

```sql
CREATE TABLE `rcc_outbox_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `event_id` char(26) NOT NULL,
  `event_type` varchar(64) NOT NULL,
  `aggregate_id` varchar(128) NOT NULL,
  `idempotency_key` varchar(255) NOT NULL,
  `payload` json NOT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'PENDING',
  `attempts` int unsigned NOT NULL DEFAULT 0,
  `next_attempt_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `lease_owner` varchar(128) NULL,
  `lease_until` datetime NULL,
  `last_error` varchar(2000) NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_outbox_event_id` (`event_id`),
  UNIQUE KEY `uk_outbox_idempotency` (`idempotency_key`),
  KEY `idx_outbox_dispatch` (`status`, `next_attempt_at`, `id`),
  CONSTRAINT `chk_outbox_status` CHECK (`status` IN ('PENDING','PROCESSING','SUCCEEDED','DEAD_LETTER'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8.6 写请求幂等

```sql
CREATE TABLE `rcc_idempotency_records` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `principal` varchar(128) NOT NULL,
  `operation` varchar(64) NOT NULL,
  `idempotency_key` varchar(128) NOT NULL,
  `request_digest` char(64) NOT NULL,
  `response_status` int NOT NULL,
  `response_body` json NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_idempotency` (`principal`, `operation`, `idempotency_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8.7 审批记录

```sql
CREATE TABLE `rcc_release_approvals` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_number` varchar(64) NOT NULL,
  `node_code` varchar(64) NOT NULL,
  `decision` varchar(16) NOT NULL,
  `approver` varchar(128) NOT NULL,
  `comment` varchar(2000) NOT NULL,
  `request_key` varchar(128) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_approval_request` (`release_number`, `node_code`, `request_key`),
  CONSTRAINT `chk_approval_decision` CHECK (`decision` IN ('APPROVED','REJECTED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8.8 版本、Command 与百分比灰度

`rcc_table_versions`、`rcc_increment_commands`、`rcc_command_watermarks`、`rcc_rollout_policies`、`rcc_rollout_rules` 和 `rcc_rollout_subject_overrides` 的完整 DDL 由 Server 规格定义。Publication Committer 是这些表的唯一 Admin 写入口。

## 9. 配置 Schema

### 9.1 发布模型

```json
{
  "model_code": "merchant_channel",
  "target_fields": ["merchant_id", "channel_code"],
  "release_types": ["standard", "rollout"],
  "max_batch_items": 1000
}
```

模型必须引用启用 Page Model 和 Table Policy，目标字段必须存在且不可为空。

### 9.2 模板

默认模板：

```json
{
  "code": "standard_v1",
  "nodes": ["VALIDATE", "APPROVAL", "APPLY", "NOTIFY", "SUCCESS"]
}
```

灰度模板：

```json
{
  "code": "rollout_v1",
  "nodes": ["VALIDATE", "APPROVAL", "APPLY_ROLLOUT", "NOTIFY", "SUCCESS"]
}
```

模板发布时校验首尾节点、重复节点、已注册处理器和允许转移。

### 9.3 百分比灰度

```json
{
  "policy_code": "merchant_rollout_2026_01",
  "allocation_group": "merchant_channel",
  "allocation_seed": "01J...",
  "bucket_algorithm": "sha256-v1",
  "rollout_bps": 1000,
  "include_subject_keys": ["merchant-42"],
  "exclude_subject_keys": ["merchant-99"]
}
```

Admin 只保存并分发 `SHA-256(normalized_subject_key)`。同一 Key 同时出现在 include/exclude 时拒绝。自定义算法代码只存在于 Client；策略只保存带版本的算法名称。

第一版的 `normalized_subject_key` 就是调用方提供字符串的原始 UTF-8 字节：不 trim、不转换大小写、不做 Unicode 归一化；空字符串拒绝。Admin 录入名单和 Client 查询必须使用完全相同的业务 Key 表示，避免看似相同但字节不同的值产生歧义。

## 10. 状态机与动作矩阵

```text
DRAFT -> PENDING_APPROVAL -> APPROVED -> IN_PROGRESS -> SUCCEEDED
   |             |              |             |
   +-> CANCELLED +-> REJECTED   +-> CANCELLED +-> FAILED
                                                +-> ROLLED_BACK
```

| 状态 | 允许动作 |
|---|---|
| DRAFT | SUBMIT、CANCEL |
| PENDING_APPROVAL | APPROVE、REJECT、CANCEL |
| APPROVED | EXECUTE、CANCEL |
| IN_PROGRESS | ADVANCE、ROLLBACK |
| FAILED | RETRY、ROLLBACK、CANCEL |
| 终态 | 查询；SUCCEEDED 可按模板发起 ROLLBACK |

每次状态变化递增 `version`。所有写动作携带 expected version。

## 11. 节点 SPI 与语义

```go
type Node interface {
    Code() string
    Execute(ctx context.Context, input NodeInput) (NodeResult, error)
    Rollback(ctx context.Context, input NodeInput) (NodeResult, error)
}
```

正式数据写入由一个深的 Publication Committer 模块承接：

```go
type PublicationCommitter interface {
    Commit(ctx context.Context, input PublicationInput) (PublicationResult, error)
}
```

它在一个数据库事务内隐藏业务写入、写后最终行读取、Command 规范编码、校验和、版本推进、日志和 Outbox。节点和 HTTP 层不得分别执行这些步骤。

- `VALIDATE`：重验 Schema、Policy、before state、批量内容和灰度配置。
- `APPROVAL`：进入等待；approve/reject HTTP 接口完成节点。
- `APPLY`：调用 Publication Committer 写正式数据。
- `APPLY_ROLLOUT`：写 Rollout Policy、Rule、Override、版本和 Command。
- `NOTIFY`：只确认 Outbox 已创建，不同步等待 Server。
- `SUCCESS`：释放 Active Target 并进入终态。

## 12. 核心算法

### 12.1 创建

```text
authenticate and authorize
normalize request and digest
resolve model/template snapshots
validate every item
calculate and sort target hashes
begin transaction
  insert idempotency placeholder
  insert Active Targets in sorted order
  insert order/items/initial log
  store deterministic response
commit
```

### 12.2 查询

一次只读事务加载主表、明细、审批、日志和 Outbox 摘要；可执行动作由当前主体和状态机现场计算。

### 12.3 推进

```text
begin transaction
  SELECT release order FOR UPDATE
  verify version/status/permission/current node
  execute pure validation, or:
    apply business mutations
    batch SELECT final rows in the same transaction
    canonicalize and validate complete rows
    insert trusted final-state Commands
    advance table versions to transaction max command IDs
  append log
  update node context/status/version
  create Outbox if required
commit
```

ADD/MODIFY 必须在写入之后批量读取最终行，再写 Command.ConfigValue。这样数据库默认值、触发器、generated column、auto increment 和数据库侧归一化结果都会进入 Command。DELETE 写入 `ConfigValue=NULL` 的 tombstone。最终行读取、Command 插入或版本推进任一步失败时，整个事务回滚。

Command 的 `SchemaDigest` 来自本次发布使用的实时 TableDefinition；`PayloadChecksum` 按 Server 规格的规范信封计算。读取结果必须包含 Schema 声明的完整字段，并满足 Row ID 与 ConfigKey 相等。

### 12.4 审批

审批接口行锁发布单，校验待审批节点、禁止自批、写幂等审批记录和操作日志，再原子更新节点与版本。

### 12.5 Outbox worker

worker 使用 `SELECT ... FOR UPDATE SKIP LOCKED` 领取到期事件并设置短租约；网络调用在领取事务之外执行；成功按 event ID 幂等完成，失败指数退避，达到上限进入 DEAD_LETTER。

## 13. 事务、锁、幂等与一致性

1. 创建同事务写发布单、全部明细、Active Target、日志和幂等结果。
2. 发布同事务写业务表、版本、Command、发布状态、日志和 Outbox。
3. 网络调用不得进入数据库事务。
4. Command 的 `id` 是全局自增，但消费游标按表维护。
5. 每个受影响的 ConfigKey 至少写一条 Command；唯一键变化写旧键 DELETE 和新键 ADD。
6. ROW ADD/MODIFY Command 必须携带可信 Canonical Final Row、SchemaDigest 和 PayloadChecksum；DELETE 必须携带 tombstone。
7. 灰度规则或名单变化必须为每个受影响目标表写元信息 Command。
8. 批量发布按 ConfigKey 去重后分块读取最终行；单事务明细数、SQL 参数数、返回字节数和事务时长有硬上限。
9. Active Target 插入按哈希排序，降低批量交叉目标死锁。
10. 重试同一个 Idempotency-Key 必须返回原结果；摘要不同返回冲突。

发布端为每个变更 Key 执行至多一次逻辑最终态读取，并优先按表批量查询。Server 实例和 Client 数量不得增加这次读取次数。超过批量或事务预算的发布单应在提交前拒绝或拆成多张发布单，不能提交一半后再异步补 Command。

## 14. 外部依赖协议

开源核心只有一个远程端口：Server Refresh Port。

```text
Outbox event: SERVER_REFRESH_REQUESTED
payload: release_number + affected_tables + max_command_ids
success: Server 接受请求并返回 request_id
timeout: 3s
retry: exponential, configurable max attempts
```

没有 Server 时，Outbox 可以持续重试或由管理员暂停；数据库版本和 Command 已提交，Server 恢复后也能通过轮询收敛。

## 15. 权限、审计与安全

- 可信身份头缺失时 Admin 拒绝请求。
- RBAC 数据由 Admin 控制表维护；角色变更审计。
- 审批人不得等于创建人。
- Content、SubjectKey、名单和数据库错误不得写日志。
- Command.ConfigValue 与正式配置同级保护，不进入普通操作日志、指标标签或错误响应。
- SubjectKey 归一化后只保存 SHA-256。
- 模型、表、字段和模板节点使用白名单校验。
- 批量大小、JSON 深度、字段长度和请求频率有上限。
- 死信重放和应急回滚要求 ADMIN、原因和二次确认。

## 16. 可观测性与运维

### 16.1 版本大盘

开源版 MUST 提供面向运维诊断的版本大盘。环境是大盘的必选顶层筛选条件；大盘以该环境内的 Managed Table 为查询入口，至少展示：

- DB 目标版本：权威 `source_updated_at`；
- Server 缓存版本：节点标识、`source_updated_at`、数据 MD5 和缓存加载时间；
- Client 缓存版本：实例标识、`source_updated_at`、数据 MD5、缓存生效时间和最后上报时间；
- 聚合状态：Server 已收敛数量、Client 已收敛数量、落后数量、摘要不一致数量和状态未知数量。

Server 和 Client 展示的 `source_updated_at` 必须沿 DB 版本链路传递，不得使用本地缓存刷新时间替代。版本大盘的精确明细不得依赖日志检索或把 ClientID、版本、MD5 作为 Prometheus 标签。

本节先冻结功能需求。状态上报协议、当前状态存储归属、在线 TTL 和历史保留周期需要在实现前另行确认。

### 16.2 指标与巡检

```text
rcc_release_created_total{type,result}
rcc_release_active
rcc_release_node_duration_seconds{node,result}
rcc_release_approval_total{decision,result}
rcc_release_conflict_total{reason}
rcc_outbox_pending
rcc_outbox_oldest_age_seconds
rcc_outbox_delivery_total{type,result}
rcc_outbox_dead_letter_total{type}
rcc_release_inconsistent_total{kind}
rcc_release_final_row_read_total{table,result}
rcc_release_final_row_read_rows{table}
rcc_release_command_payload_bytes{table}
rcc_release_transaction_duration_seconds{type}
```

每日巡检 Active Target 泄漏、终态未释放、批量空明细、版本无 Command、Command 无版本、成功发布缺刷新 Outbox 和长期死信。

## 17. 部署、迁移与回滚

1. 创建新增控制表、索引以及 Command 的 `schema_digest`、`payload_checksum` 字段。
2. 部署 Admin 但关闭发布入口。
3. 启动 Outbox worker dry-run。
4. 启用只读详情和模板管理。
5. 启用创建与内置审批。
6. 对测试表启用新版 Publication Committer，验证 Canonical Final Row 和校验和。
7. Server 先以 shadow 比较可信 Command 与全量结果，再启用 command 模式。
8. 启用正式发布和 Server 刷新投递，最后启用灰度发布。

Schema 迁移只做 expand/backfill/contract；旧的不可信 ConfigValue 不允许伪造校验和，应按 Server 规格通过 cutover cursor 和一次受限全量建立新基线。应用回滚不得删除 Command、版本、Outbox、日志或灰度规则。

## 18. 验收测试

至少覆盖：

- 单笔与 1,000 条批量创建；
- 批量中任一非法时零残留；
- 相同目标并发创建仅一单成功；
- 创建人自批被拒绝；
- 重复 approve/reject 幂等；
- 旧 expected version 被拒绝；
- 发布事务前退出全部回滚；
- 提交后 worker 退出，Outbox 保留；
- Server 超时但已接受，重试不产生重复效果；
- 每个数据变化都有 Command，版本与 Command 同事务；
- ADD/MODIFY Command 包含 default、trigger、generated column 处理后的完整最终行；
- DELETE Command 为 tombstone，SchemaDigest 和 PayloadChecksum 校验通过；
- 1,000 条批量发布按表分块读取，不出现逐行 N+1；
- 任一最终行缺失、字段不全或校验和异常时业务写入、Command、版本和 Outbox 全部回滚；
- 灰度比例、算法名、名单冲突和 SubjectKey 哈希校验；
- 回滚生成反向 Command 并推进新版本；
- 版本大盘能够区分 DB 目标版本、Server 缓存版本和 Client 缓存版本，并识别版本落后、MD5 不一致和状态未知；
- 多个测试环境的发布、版本、Command、Outbox 和版本大盘相互隔离，生产环境全系统唯一；
- 1000 次并发无重复在途目标和重复副作用。

## 19. 与闭源参考实现的置换

| 闭源概念 | 开源实现 |
|---|---|
| 公司审批平台 | Admin 内置 approve/reject |
| RMQ/TCC | MySQL Outbox + Server gRPC + Server 轮询兜底 |
| ChangeGate/Flow Replay | 不进入核心；未来可作为 Outbox 适配器 |
| PSM、Stage、Region | 删除 |
| 闭源 Env 语义 | 替换为开源 Environment：多个 `TEST`、唯一 `PRODUCTION`，所有请求显式传环境编码 |
| Thrift 和内部错误码 | HTTP/JSON、gRPC/Protobuf 和开源稳定错误码 |
| 进程内后置任务 | 持久化 MySQL Outbox |
| 原始灰度名单 | SHA-256 显式包含/排除名单 |

本文不追求闭源实现的 bug-for-bug 兼容；参考文档中的并发、幂等和恢复问题被写成开源版必须满足的不变量。
