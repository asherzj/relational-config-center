# Go Client SDK 开源复刻与落地技术方案

> 规格版本：1.0.0
> 状态：Implementation Candidate
> 所属上下文：Client
> 传输：gRPC/Protobuf
> 约束：无 PSM、Stage、Region、Thrift 或公司内部服务发现依赖；保留开源 Environment 隔离能力。

## 1. 方案摘要

本方案建设一个嵌入业务进程的 Go 配置 SDK。SDK 启动时必须绑定一个具体 Environment，从 Runtime Config Server 获取该环境订阅表的全量快照，在进程内构建不可变缓存，并通过表级 `CommandCursor` 完成真正的行级增量同步。业务查询只读取本地内存，不把 Server 可用性带入请求链路。

首次启动时，SDK 原子保存“全量数据、表定义、订阅索引、百分比灰度规则、版本和各表游标”。运行期间，SDK 将本地游标提交给 `SyncByCommand`；对每张表以 Copy-On-Write 应用 `Upserts`、`DeleteKeys`、元信息和灰度替换，成功后才推进该表游标。任何步骤失败都保留旧表和旧游标；Command 无法衔接时自动执行单表全量恢复。

百分比灰度由业务提供稳定 SubjectKey，SDK 使用策略指定的算法统一计算 0～9,999 桶。SDK 内置稳定的 `sha256-v1`，并允许业务在启动时注册带版本的自定义算法。显式排除名单、显式包含名单和百分比规则按固定优先级执行。

## 2. 建设目标

### 2.1 功能目标

1. 启动时获取消费者完整快照，并在成功校验后一次性对业务可见。
2. 为每张表独立维护 `CommandCursor`，通过 `SyncByCommand` 应用行级增量。
3. 支持 Stream 低延迟提示与周期同步兜底；两者统一进入一个同步控制器。
4. 支持按表、订阅 code、主键和结构体查询。
5. 支持业务提供稳定 SubjectKey 的百分比灰度查询。
6. 支持 SHA-256 显式包含和排除名单。
7. 支持 Command 过期、游标超前、增量过大和校验失败时的单表全量自愈。
8. 数据、元信息、灰度、版本和游标必须原子发布。
9. 为版本大盘提供每个 Client 实例、每张表的源版本时间、数据 MD5、缓存生效时间和最后上报时间。
10. Client 启动时显式传入环境编码，所有全量、增量、Stream 和状态上报只能访问该环境。

### 2.2 工程目标

- 查询路径无网络、无锁或只使用不可变对象。
- 同步逻辑可测试、可观测、可限流、可优雅关闭。
- SDK 公开 API 小而稳定，协议生成代码不泄漏到业务查询接口。
- 内置算法跨进程、跨机器和升级后结果一致。
- 支持 Go 当前稳定版与前一稳定版。

### 2.3 非目标

- SDK 不负责配置编辑、审批或发布。
- SDK 不执行任意 SQL，不直接访问 MySQL。
- SDK 不实现跨表 JOIN、事务查询或关系约束。
- SDK 不替业务生成随机 SubjectKey，也不承诺低基数 Key 能形成有效流量切分。
- SDK 不在业务请求中同步访问 Server。
- 第一版只提供 Go SDK；其他语言应复用 Protobuf 和算法测试向量独立实现。

## 3. 兼容边界

开源版以自身 Protobuf、公开 Go API 和快照格式为兼容边界，遵循语义化版本。它不兼容闭源版的 Thrift 字段、错误码、服务路由、区域快照或内部压缩封装。

兼容规则如下：

- Protobuf 字段只追加，不复用已删除 tag。
- 公开 Go API 的破坏性修改只进入新的 major 版本。
- `sha256-v1` 的输入编码和结果永久冻结，并提供跨语言测试向量。
- 自定义算法名就是行为版本；实现改变必须注册新名字。
- 本地持久化快照不是第一版公开兼容格式，进程重启必须重新全量获取。

## 4. 整体架构

```text
Business goroutines
        |
        v
Query API --> atomic.Pointer[ClientSnapshot]
                        ^
                        | one atomic Store
                  Snapshot Builder
                        ^
                        |
                   Sync Controller
                    /          \
        WatchChanges hint    periodic tick
                    \          /
                     SyncByCommand
                           |
                    Runtime Config Server
```

### 4.1 分层职责

| 模块 | 职责 |
|---|---|
| Starter | 校验配置、注册算法、全量启动、后台任务生命周期 |
| Transport | gRPC、超时、压缩、重试和协议转换 |
| Sync Controller | 合并提示、串行同步、退避、单表全量恢复 |
| Snapshot Builder | 全量构建、增量 COW、校验和原子发布 |
| TableStore | 紧凑保存行并提供主键、全表和订阅索引查询 |
| Rollout Evaluator | 名单判断、桶计算和规则覆盖 |
| Query API | 稳定的业务查询接口和结构体转换 |
| Event Dispatcher | 快照发布后的本地变化事件 |

### 4.2 建议代码目录

```text
client/
  config.go
  client.go
  query.go
  rollout.go
  event.go
  internal/
    transport/
    syncer/
    snapshot/
    tablestore/
    codec/
    validate/
```

`Client` 是对外深模块。Transport、协议对象、缓存布局和重试状态不得成为业务侧必需知识。

## 5. 配置设计

### 5.1 公开配置

```go
type Options struct {
    Environment      string
    ConsumerID       string
    ClientID         string
    ServerAddress    string
    PollInterval     time.Duration
    RPCDeadline      time.Duration
    StartupDeadline  time.Duration
    MaxSnapshotBytes int64
    MaxEventQueue    int
}

type BucketInput struct {
    SubjectKey      string
    AllocationGroup string
    AllocationSeed  string
}

type BucketAlgorithm func(context.Context, BucketInput) (uint32, error)

func WithBucketAlgorithm(name string, algorithm BucketAlgorithm) Option
```

业务必须显式提供环境编码、稳定且有意义的 `ConsumerID`、单进程实例唯一的 `ClientID` 和 Server 地址。Environment 在 Client 生命周期内不可修改；切换环境必须创建新的 Client。凭据通过标准 gRPC transport credentials 或调用方注入的 DialOption 配置，禁止写入日志。

### 5.2 默认值

| 配置 | 默认值 |
|---|---:|
| PollInterval | 15 秒，并加入最多 20% jitter |
| RPCDeadline | 5 秒 |
| StartupDeadline | 30 秒 |
| MaxSnapshotBytes | 256 MiB |
| MaxEventQueue | 1,024 |

重试采用有上限的指数退避并加入 jitter。Watch 断开不阻塞 Poll；Poll 不因 Watch 正常而关闭。

### 5.3 配置校验

- Environment、ConsumerID、ClientID 和 ServerAddress 必填。
- Environment 不提供默认值；Server 返回环境不存在、禁用或不匹配时启动失败，禁止回退到其他测试环境或生产环境。
- 时间配置必须为正数并位于实现给定安全上限内。
- 自定义算法名必须匹配 `^[a-z][a-z0-9-]*-v[1-9][0-9]*$`。
- `sha256-v1` 为保留名，禁止覆盖。
- 同名算法只能注册一次。
- 所有校验必须在首次网络请求前完成。

### 5.4 服务路由

SDK 只接受一个标准 gRPC target 或调用方提供的 `grpc.ClientConnInterface`。服务发现、负载均衡和 TLS 由 grpc-go 标准机制或调用方基础设施负责，SDK 不引入特定公司的路由概念。

## 6. RPC 与压缩协议

### 6.1 RPC 方法

SDK 使用 Server 规格中的以下方法，所有请求都必须携带启动时冻结的 Environment：

| 方法 | 用途 |
|---|---|
| `GetSnapshot` | 首次启动获取所有订阅表基线 |
| `GetTables` | 单表或少量表全量恢复 |
| `GetMetadata` | 诊断工具使用，不进入正常同步主路径 |
| `SyncByCommand` | 正常运行的统一增量同步入口 |
| `WatchChanges` | 接收可能发生变化的低延迟提示 |

SDK 不再执行“先比较版本、再整表拉取”的两次调用。即使没有收到 Stream 提示，也必须按 `PollInterval` 调用 `SyncByCommand`。

`GetSnapshot`、`GetTables`、`SyncByCommand` 和 `WatchChanges` 的请求处理只允许读取 Server 已发布的内存 Snapshot。特别是 `SyncByCommand` 不得按 Client cursor 现场查询 Command 表或 Managed Table；增加 Client 实例数不能增加 MySQL 查询次数。数据库解析、Command 校验和 ResolvedDeltaHistory 构建属于 Server 自身刷新路径。

### 6.2 全量与增量响应语义

`GetSnapshot` 和 `GetTables` 返回的表数据、定义、订阅、RolloutBundle、TableVersion 和 CommandCursor 必须属于同一个 Server Snapshot。SDK 不得组合来自不同响应的同一张表。

`SyncByCommand` 对每张表返回：

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
```

SDK 必须允许响应表级部分成功。`NeedFullSync` 只恢复对应表；`DropTable` 原子删除该表；单表错误不得回滚同批其他已成功表。

当前 Server 订阅中存在而本地没有的表，没有可信的全量基线。SDK 不得以游标 0 假设 Command 历史包含该表全部存量数据；它必须根据 `NeedFullSync` 调用 `GetTables`。

### 6.3 稳定压缩格式

传输层使用 gRPC 协商的 LZ4 Frame。解压前校验压缩字节上限，流式解压时校验声明长度和实际输出上限，解码后校验表数、行数、列数、字符串长度、主键唯一性和摘要。压缩炸弹、截断数据或未知必要字段一律拒绝，不改变当前快照。

## 7. 本地快照和紧凑存储

### 7.1 ClientSnapshot

```go
type ClientSnapshot struct {
    Environment string
    Generation uint64
    Tables     map[string]*ClientTable
}

type ClientTable struct {
    Store          *TableStore
    Definition     TableDefinition
    Subscriptions  []Subscription
    Rollout         RolloutBundle
    Version         TableVersion
    CommandCursor   int64
}
```

`ClientSnapshot` 及其可达对象在发布后全部不可变。`CommandCursor` 也可投影为内部 `commandCursorMap map[string]int64`，但只能从当前快照派生，禁止维护第二份独立可变状态。

### 7.2 TableStore

TableStore 应以列定义和紧凑行布局保存数据，同时提供：

- `ConfigKey -> row position` 主键索引；
- 全表稳定遍历；
- 订阅 code 对应的 ONE/MANY 复合键索引；
- COW clone 和批量 upsert/delete；
- 行解码为 `map[string]string` 或业务结构体。

实现可以共享未变列块或索引，但不得让新旧 Snapshot 共享可变容器。

### 7.3 订阅索引

订阅索引的 Key 必须按 Schema Codec 规范化，并使用无歧义的长度前缀编码，禁止简单字符串拼接。ONE 索引出现重复键、MANY 索引顺序不稳定或订阅字段不存在时，表构建失败。

## 8. 启动方案

### 8.1 状态机

```text
NEW -> STARTING -> READY -> CLOSING -> CLOSED
          |
          +------> FAILED
```

只有完整基线成功发布后才进入 READY。FAILED 状态不得启动后台任务或暴露空快照假装成功。

### 8.2 初始化顺序

1. 冻结 Options 和算法注册表。
2. 建立 gRPC 连接并等待 StartupDeadline。
3. 调用 `GetSnapshot`。
4. 解压、解码并校验所有订阅表。
5. 为每张表构建 TableStore 和订阅索引。
6. 校验 Rollout Policy 引用的算法均已注册或为 `sha256-v1`。
7. 校验 TableVersion.CommandCursor 与响应 cursor map 一致。
8. 构造一份完整 ClientSnapshot 并执行一次原子 Store。
9. 将状态切到 READY。
10. 启动周期同步、Watch、事件分发和健康统计任务。

启动期间任一步失败都返回带稳定错误分类的错误。调用方决定退出、降级还是重试，SDK 不应无限阻塞应用启动。

## 9. 更新和最终一致方案

### 9.1 统一同步入口

Watch 回调和周期计时器只提交“需要同步”信号。单实例 `SyncController` 合并信号，并保证任一时刻最多一个同步批次。额外信号在当前批次结束后触发下一轮，不并发写快照。

每轮从当前 ClientSnapshot 生成全部本地表的 `CursorMap`，调用一次 `SyncByCommand`。Server 会同时发现新增订阅和已取消订阅；Client 不根据 Stream 提示自行猜测表集合。

Client 只消费 Server 已物化的最终态 Delta。请求超时、重试、多个 Client 使用相同游标或同步风暴都只能增加 Server 内存读取与网络流量，不能触发数据库回源。如果 Server 的有界 DeltaHistory 已不能覆盖某个 cursor，响应必须是 NeedFullSync，而不是为该 Client 临时查库补齐。

### 9.2 行级增量同步流程

对返回的每张表独立执行：

1. 从同一份旧 ClientSnapshot 读取 `localCursor`。
2. 校验 `delta.FromCursor == localCursor`，且 `ToCursor >= FromCursor`。
3. `DropTable=true` 时标记从新快照删除该表。
4. `NeedFullSync=true` 时暂不应用 delta，加入单表全量恢复集合。
5. clone 该表 TableStore。
6. 应用 DeleteKeys，再应用 Upserts，并校验 Upsert 的 Map Key 等于 Row ConfigKey。
7. 重新构建受影响的主键和订阅派生索引。
8. 按 ReplaceMetadata/ReplaceRollout 替换完整元信息或灰度包。
9. 校验 Version、摘要、引用和 `Version.CommandCursor == delta.ToCursor`。
10. 在候选 ClientTable 中同时写入新数据、元信息、Rollout、Version 和 CommandCursor。

所有成功表和 DropTable 结果组装成一个新的 ClientSnapshot，最后只执行一次 `ClientSnapshotPtr.Store`。任一表的构建失败时，该表完整保留旧值和旧游标，其他成功表仍可提交。不得先修改游标再修改数据，也不得原地修改旧快照。

### 9.3 单表全量恢复

以下情况调用 `GetTables`：

- Server 返回 NeedFullSync；
- 本地缺少当前订阅表；
- Client cursor 超过 Server cursor；
- Command 已被清理或窗口无法衔接；
- delta 超出服务端限制；
- 增量应用、索引重建或摘要校验失败。

全量响应校验成功后，表数据和响应 cursor 一次性替换。恢复失败保留旧表；新增表恢复失败则继续不可查询，并记录未就绪状态。失败表采用退避重试，不阻塞其他表同步。

这里的 Client 单表全量是 `GetTables` 从 Server 已发布 Snapshot 读取完整表，不代表 Server 回源 Managed Table。Client 重试或大量 Client 同时恢复只能增加网络和序列化负载，不能触发数据库扫描；Server 内部是否需要数据库恢复由其独立刷新控制器决定。

### 9.4 Stream、Poll 与自愈

`WatchChanges` 只提供低延迟提示，不携带配置事实。断流后立即进入有上限的退避重连；周期 `SyncByCommand` 始终运行，因此漏提示、连接重置和 Server 重启都不会破坏最终一致性。

后台 goroutine 必须有顶层 panic 恢复、错误计数和可控重启。连续 panic 达阈值后停止对应任务并暴露不健康状态，不能静默退出或快速死循环。

## 10. 查询 API 方案

### 10.1 公开类型

```go
type Query struct {
    TableName       string
    SubscriptionCode string
    Keys            map[string]string
}

type RolloutEvaluation struct {
    SubjectKey string
}

type QueryResult struct {
    Rows    []map[string]string
    Version TableVersion
}
```

`SubjectKey` 是业务选择的稳定原始值，例如 user ID、merchant ID 或 device ID。SDK 不持久化、不发送、不记录该原始值。

### 10.2 公开函数

```go
func (c *Client) Query(ctx context.Context, query Query) (QueryResult, error)
func (c *Client) QueryWithRollout(
    ctx context.Context,
    query Query,
    evaluation RolloutEvaluation,
) (QueryResult, error)
func QueryAs[T any](ctx context.Context, c *Client, query Query) ([]T, error)
func QueryAsWithRollout[T any](
    ctx context.Context,
    c *Client,
    query Query,
    evaluation RolloutEvaluation,
) ([]T, error)
func (c *Client) Version(tableName string) (TableVersion, bool)
```

`Query` 只返回正式配置；灰度结果必须显式调用 `QueryWithRollout`，避免未传 SubjectKey 时产生隐式分流。ctx 只用于取消本地转换和自定义算法，不触发网络访问。

### 10.3 统一查询步骤

1. 原子 Load 一次 ClientSnapshot。
2. 解析 TableName 和 SubscriptionCode。
3. 使用 Schema Codec 规范化 Keys。
4. 从同一 ClientTable 查询正式行。
5. 灰度查询时，对相关规则执行名单和桶判断，并在临时视图应用 ADD/MODIFY/DELETE。
6. 返回深拷贝或不可变结果，不暴露内部切片和 map。
7. 整个查询禁止再次 Load Snapshot。

### 10.4 泛型结构体转换

结构体转换按列名、`config` tag 或明确注册的字段映射执行。数字、布尔、时间、JSON 和 NULL 使用稳定 Codec；未知列默认忽略，缺失必填字段、溢出和非法文本返回错误。转换失败不得返回半填充结构体。

### 10.5 业务百分比灰度

业务只提供 SubjectKey，不直接提供桶。SDK 根据 Policy 的 `BucketAlgorithm` 计算桶：

```text
bucket = algorithm(SubjectKey, AllocationGroup, AllocationSeed)
hit    = bucket < RolloutBPS
```

相同算法名、输入和 Policy 必须得到相同结果。若业务选择的 SubjectKey 在所有请求中只有一个值，所有请求必然进入同一桶，无法实现百分比切分；SDK 不做随机或实例 ID 回退。业务应改用更高基数且稳定的 Key，若目标是实例灰度则显式传稳定实例标识。

## 11. 灰度配置合并

### 11.1 固定判定顺序

对某 Policy 和 SubjectKey：

1. 规范化 SubjectKey；第一版定义为原始 UTF-8 字节，不 trim、不转换大小写、不做 Unicode 归一化，空值直接返回 `INVALID_SUBJECT_KEY`。
2. 计算 `subjectHash = lowercase_hex(SHA256(normalizedSubjectKey))`。
3. 命中 `FORCE_EXCLUDE` 时不应用该 Policy。
4. 否则命中 `FORCE_INCLUDE` 时应用该 Policy。
5. 否则调用 Policy 指定算法计算桶，并判断 `bucket < RolloutBPS`。

排除优先于包含，用于紧急止损。服务端只分发哈希，不分发显式名单原文。

### 11.2 内置 `sha256-v1`

输入字节固定为：

```text
AllocationSeed + 0x1f + AllocationGroup + 0x1f + SubjectKey
```

计算 SHA-256，取摘要前 8 字节按大端无符号整数解析，再对 10,000 取模。禁止本地化大小写、Unicode 模糊转换或平台相关格式。规范化规则和测试向量必须与 Server 规格及其他语言实现一致。

SHA-256 在高基数、无强偏置的业务 Key 上会得到足够均匀且稳定的桶分布，但它不能创造输入中不存在的多样性。验收时应使用代表性真实 Key 样本检查桶分布和策略调整后的稳定性。

### 11.3 自定义算法

自定义算法接收 SubjectKey、AllocationGroup 和 AllocationSeed，并返回 0～9,999。约束如下：

- 确定性、纯函数、快速、线程安全；
- 禁止随机数、网络、磁盘和其他 I/O；
- 返回超范围值视为错误；
- panic 被 SDK 捕获并转换为错误；
- 同名实现冻结，升级必须使用新版本名，例如 `merchant-mod-v2`；
- Policy 引用未注册算法时，该 Policy 查询失败，正式配置仍可通过普通 Query 获取。

自定义算法是 Client 扩展点。Server 和 Admin 只存储、校验并分发算法名，不执行其实现。

### 11.4 规则覆盖

命中 Policy 后，SDK 在本次查询的临时结果上按稳定的 RuleCode 顺序应用规则：ADD 插入不存在的 ConfigKey，MODIFY 以完整最终行替换目标行，DELETE 删除目标行。规则冲突、非法引用或动作与当前状态矛盾时返回错误，不污染正式缓存。

## 12. 事件方案

```go
type ChangeEvent struct {
    Type       string // DATA_CHANGED / METADATA_CHANGED / ROLLOUT_CHANGED / TABLE_REMOVED
    TableName  string
    FromCursor int64
    ToCursor   int64
    Revision   uint64
}
```

事件只能在新 ClientSnapshot 原子 Store 成功后投递，因此回调读取到的至少是事件对应版本。事件是进程内 at-least-once 提示，允许合并和重复；业务不得把它当作审计日志。

回调在独立 worker 上执行，不能阻塞同步。队列满时合并同表事件并记录 dropped/coalesced 指标；回调 panic 被隔离。SDK Close 时停止接收新事件，并在有限时间内排空。

## 13. 错误处理

稳定错误分类至少包括：

| 错误码 | 语义 |
|---|---|
| `NOT_READY` | 尚无可用完整基线 |
| `TABLE_NOT_FOUND` | 表未订阅、已移除或尚未全量成功 |
| `SUBSCRIPTION_NOT_FOUND` | 订阅 code 不存在 |
| `INVALID_QUERY` | 查询字段或 Key 非法 |
| `INVALID_SUBJECT_KEY` | 灰度 SubjectKey 为空或无法规范化 |
| `BUCKET_ALGORITHM_NOT_FOUND` | Policy 引用算法未注册 |
| `BUCKET_ALGORITHM_FAILED` | 算法报错、panic 或返回越界 |
| `CURSOR_MISMATCH` | FromCursor 与本地不一致 |
| `SNAPSHOT_INVALID` | 全量或增量校验失败 |
| `TRANSPORT_UNAVAILABLE` | Server 暂时不可用 |
| `CLOSED` | Client 已关闭 |

查询错误可包装上下文但必须支持 `errors.Is/As`。日志禁止包含行值、SubjectKey、名单原文、认证信息和完整响应。

## 14. 并发与内存规则

- 查询只读一次 `atomic.Pointer[ClientSnapshot]`。
- 只有 SyncController 能提交新快照。
- 旧快照只依赖 Go GC 回收，不手工复用仍可达内存。
- COW 构建设置单表和整批内存预算；预算不足转单表全量或保留旧值。
- Close 必须幂等，所有 goroutine 有归属的 context 和 WaitGroup。
- 自定义算法可能被并发调用，注册后不可修改。
- 对外返回值不能引用内部可变存储。

## 15. 可观测性

### 15.1 指标

```text
rcc_client_ready
rcc_client_snapshot_generation
rcc_client_table_revision{table}
rcc_client_command_cursor{table}
rcc_client_sync_total{result,mode}
rcc_client_sync_duration_seconds{mode}
rcc_client_delta_commands{table}
rcc_client_full_fallback_total{table,reason}
rcc_client_watch_reconnect_total
rcc_client_query_total{table,result,rollout}
rcc_client_query_duration_seconds{table,rollout}
rcc_client_bucket_error_total{algorithm,reason}
rcc_client_event_total{type,result}
```

表标签必须经过数量限制；不得以 SubjectKey、ConfigKey、ClientID 或错误文本作为指标标签。

### 15.2 告警

建议对长期未 Ready、同步连续失败、cursor 长时间不推进、单表全量兜底率突增、Watch 持续断开、算法错误、事件持续丢弃和内存超限告警。Watch 断开但 Poll 正常不应单独升级为最高级别事故。

### 15.3 版本大盘状态

SDK MUST 在 TableSnapshot 完成校验并通过 `atomic.Pointer.Store` 对业务生效后，生成对应表的版本状态。状态至少包含 Environment、ClientID、表名、DB `source_updated_at`、数据 MD5、缓存生效时间、revision、Command cursor 和最后上报时间；失败的增量或全量构建不得提前推进状态。

状态上报必须异步执行，上报失败不得回滚已经生效的业务缓存，也不得阻塞查询路径。精确状态不得依赖业务日志或把 ClientID、版本、MD5 写入 Prometheus 标签。上报 RPC、心跳周期、当前状态存储和离线判定在实现前另行确认。

## 16. 实施任务拆分

### 16.1 WP1：契约和测试基座

- 固化 Protobuf 与生成代码；
- 建立 fake transport、快照 fixture 和 MySQL 集成环境；
- 固化 `sha256-v1` 跨语言测试向量；
- 建立 race、fuzz 和 benchmark 基线。

### 16.2 WP2：快照与 TableStore

- 实现 Schema Codec、紧凑行和主键索引；
- 实现订阅 ONE/MANY 索引；
- 实现全量构建、摘要校验和 atomic Store；
- 实现无灰度查询 API。

### 16.3 WP3：更新控制面

- 实现 SyncController 和 CursorMap；
- 实现 Upsert/Delete COW 和派生索引重建；
- 实现 ReplaceMetadata、ReplaceRollout 和 DropTable；
- 实现 NeedFullSync 单表恢复及表级部分成功。

### 16.4 WP4：Stream、灰度和事件

- 实现 Watch 重连、Poll 兜底和 panic 自愈；
- 实现名单、`sha256-v1`、自定义算法注册和规则覆盖；
- 实现 QueryWithRollout；
- 实现发布后事件分发。

### 16.5 WP5：集成、性能和发布

- 与 Server 完成全量、增量、断档和订阅变更联调；
- 完成故障注入、容量、race、fuzz 和长稳测试；
- 输出示例、迁移指南、SLO 和运维手册；
- 完成 Client 表级版本状态上报和版本大盘联调；
- 灰度发布并验证回滚路径。

## 17. 验证方案

### 17.1 必测矩阵

- 首次全量的数据、元信息、Rollout、Version、Cursor 原子性；
- `(clientCursor, snapshotCursor]` 边界和重复同步幂等性；
- 同 Key 多条 Command 只应用最终态；
- ADD/MODIFY 使用 Snapshot 最终行，DELETE 移除目标；
- 单表失败保留旧表和旧 cursor，其他表正常推进；
- 新订阅、取消订阅、watermark 断档、cursor ahead 和 delta 超限；
- Watch 漏提示、乱序提示、断流与 Poll 收敛；
- 显式排除优先于包含，包含优先于百分比；
- `sha256-v1` 测试向量、分桶分布和相同输入稳定性；
- 自定义算法未注册、报错、panic、越界和并发安全；
- 低基数 SubjectKey 行为明确且无随机回退；
- Environment 缺失、未知、禁用或响应环境不匹配时拒绝启动和更新，多个测试环境之间无缓存、游标或事件串用；
- 版本状态只在缓存原子生效后推进，上报失败不影响查询和后续同步；
- Close、重复 Close、启动取消和 goroutine 泄漏；
- `go test -race`、Codec fuzz、Command 序列 fuzz。

### 17.2 性能验证

按代表性生产样本的 1x、2x、3x 数据规模测试：

- 启动全量时间与峰值 RSS；
- 1、100、1,000、10,000 行增量的耗时和临时内存；
- 1、100、1,000 个 Client 同步时，MySQL 查询次数保持不变；
- 并发查询 P50/P95/P99；
- 百分比灰度额外开销；
- Stream 风暴时的信号合并；
- 10,000 桶分布的卡方或最大偏差检查。

性能门槛由仓库基准文件固定，不能只在文档中给出不可复现的绝对数字。

### 17.3 完成定义

- 所有必测矩阵通过；
- Race 和 Fuzz 在约定时长内无问题；
- Client/Server 协议兼容测试通过；
- `sha256-v1` 跨语言向量一致；
- 故障期间持续返回最后完整快照；
- 恢复后 cursor、摘要和 Server Snapshot 一致；
- 版本大盘能够识别 Client 版本落后、MD5 不一致和长时间未上报；
- 示例应用可仅凭公开文档完成接入。

## 18. 发布与回滚

### 18.1 发布

1. 先发布支持完整协议的 Server。
2. Client 以全量同步模式进行 shadow，对比数据、元信息、Rollout 和 cursor。
3. 开启 `SyncByCommand`，保留单表全量兜底。
4. 开启 Watch 提示。
5. 最后开放业务灰度查询和自定义算法。

每一阶段都必须能通过配置关闭，而不改变公开查询 API。

### 18.2 回滚触发条件

- cursor 倒退或超过 Server Snapshot；
- 增量与全量摘要出现不可解释差异；
- 查询错误率、延迟、RSS 或 GC pause 超门槛；
- 自定义算法结果不稳定；
- 单表全量兜底持续异常升高；
- goroutine 泄漏或同步任务失联。

### 18.3 回滚动作

关闭 Watch 不影响 Poll；关闭 Command 增量后，以 `GetTables` 按版本变化执行表级全量；关闭灰度查询后，普通 Query 继续返回正式配置。必要时回滚 Client 版本并重新执行 GetSnapshot，不复用不兼容的本地状态。

## 19. 关键风险和对策

| 风险 | 对策 |
|---|---|
| Client 跑到 Server Snapshot 前面 | Server 以捕获 Snapshot cursor 为严格上界 |
| 全局 cursor 跳过失败表 | 每张表独立 cursor，成功才推进 |
| Client 数量放大数据库读取 | SyncByCommand 只读 Server Snapshot 的 ResolvedDeltaHistory |
| Command 载荷损坏 | Server 校验 SchemaDigest、PayloadChecksum 和完整行后才发布 Snapshot |
| Command 被清理 | watermark 检测并单表全量 |
| 新订阅无法从增量重建 | 缺少本地基线时强制 GetTables |
| Stream 丢消息 | 周期 SyncByCommand 始终开启 |
| COW 峰值内存过高 | 分表构建、预算、限流和全量兜底 |
| 灰度 Key 低基数 | 文档和指标暴露问题，要求业务选择高基数稳定 Key |
| 自定义算法漂移 | 版本化名字、冻结注册表和一致性测试 |
| 名单泄露 | 只分发小写 SHA-256 哈希，不记录原始 Key |

## 20. 审计资料

实现交付必须包含：

- Protobuf descriptor 和兼容性检查报告；
- `sha256-v1` 输入规范及跨语言测试向量；
- 全量、Command 增量、watermark 和订阅变更测试记录；
- 多 Client 并发同步不增加数据库查询次数的容量测试；
- Race、Fuzz、Benchmark 与长稳报告；
- Client/Server 摘要 shadow 对比报告；
- 公开 API、错误码、接入示例和迁移指南；
- 依赖清单、许可证和安全扫描结果。

闭源参考中的区域快照、内部路由、两阶段版本比较、Thrift 兼容和业务自算 bucket 不进入本规格。保留的是不可变本地快照、统一同步入口、Copy-On-Write、失败保旧、最终一致和面向业务的内存查询体验；新增的是表级 Command 行增量、MySQL Command 断档自愈、显式名单和可版本化的 SDK 分桶扩展点。
