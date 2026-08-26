# 分布式缓存版本可视化调研

> 调研日期：2026-08-25  
> 范围：Runtime Config Server 与 Go Client 的表级 revision、Command cursor、摘要和同步状态可视化  
> 结论：现成开源项目可以承担采集、存储和大盘，但 RCC 仍需定义自己的缓存状态契约。

## 1. 结论

推荐采用两层设计：

1. **监控层**：Server 和 Go Client 通过 OpenTelemetry Metrics API 暴露低基数状态；Prometheus 或 OTLP Collector 采集；Grafana 或 Perses 展示并告警。
2. **诊断层**：RCC 提供受保护的 `CacheStatus` 查询，按需返回实例、表、revision、cursor、DataDigest、RolloutDigest 和最后同步结果。摘要不进入 Prometheus 标签。

不再把“打印日志并从日志生成指标”作为核心路径。结构化日志仍可保留为审计和故障上下文，但版本一致性指标应由缓存状态直接产生。

## 2. 开源先例

### 2.1 Nacos

Nacos 3.x Console API 可以：

- 按配置查询监听客户端，返回 `client IP -> MD5`；
- 按客户端 IP 查询其订阅配置，返回 `配置标识 -> MD5`。

这与“查询哪些 Client 正在使用哪份缓存摘要”接近，说明“客户端身份 + 配置标识 + 摘要”适合成为独立状态模型，而不只是日志文本。

来源：[Nacos Console API](https://nacos.io/en/docs/latest/manual/admin/console-api/)。

Nacos 也提供 Prometheus 指标端点及 Grafana 监控指南，但该大盘主要面向 Nacos 运行状态，不直接覆盖 RCC 的表级 revision/cursor/digest 一致性。

来源：[Nacos monitor guide](https://nacos.io/en/docs/monitor-guide/)。

### 2.2 Apollo

Apollo 的官方项目说明明确包含客户端配置监控，可以查看哪些实例正在使用配置及其版本。Java Client 2.4.0 以后还提供 `ConfigMonitor`，并可通过 JMX 或 Prometheus 导出客户端监控数据。

来源：[Apollo 项目说明](https://github.com/apolloconfig/apollo)、[Apollo Java Client 指南](https://github.com/apolloconfig/apollo/blob/master/docs/en/client/java-sdk-user-guide.md)。

Apollo 的能力可以作为交互和状态建模参考，但其 namespace/release 模型与 RCC 的关系表、每表 Command cursor 和多摘要模型不同，不能直接作为 RCC 的可视化插件。

## 3. 可复用的开源观测组件

| 项目 | 适合承担的职责 | 能否直接解决 RCC 版本一致性 |
|---|---|---|
| OpenTelemetry Go | Server/Client SDK 内的厂商中立埋点接口 | 不能；需要 RCC 定义指标和状态 |
| OpenTelemetry Collector | 接收 OTLP，转发或暴露 Prometheus 数据 | 不能；负责管道，不理解缓存版本 |
| Prometheus | 数值状态、时间序列、PromQL 聚合和告警 | 部分；适合 lag/match，不适合保存变化中的 digest 标签 |
| Grafana | Prometheus 大盘、表格、状态历史和告警展示 | 部分；需要 RCC 提供数据模型和 dashboard |
| Perses | Prometheus 原生、Dashboard as Code 的开源展示 | 部分；可作为 Grafana 的 Apache-2.0 替代 |
| SigNoz / OpenObserve | 一体化接收指标、日志、Trace 并提供大盘 | 部分；可以替换观测后端，但不减少 RCC 建模工作 |

OpenTelemetry 官方建议：库只依赖 Metrics API，由宿主应用配置 SDK 和 exporter；异步 Gauge 适合在导出周期观察当前状态。该模式适合嵌入业务进程的 Go Client，SDK 不需要自行监听端口或强迫业务选择后端。

来源：[OpenTelemetry Go instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/)、[OpenTelemetry Go exporters](https://opentelemetry.io/docs/languages/go/exporters/)、[Collector exporters](https://opentelemetry.io/docs/collector/components/exporter/)。

Grafana 原生支持 Prometheus，并支持把 datasource 和 dashboard 文件纳入版本控制。Perses 同样原生支持 Prometheus，强调 GitOps 和 Dashboard as Code。

来源：[Grafana Prometheus datasource](https://grafana.com/docs/grafana/latest/datasources/prometheus/)、[Grafana provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)、[Prometheus 的 Perses 文档](https://prometheus.io/docs/visualization/perses/)。

许可证方面，Prometheus、OpenTelemetry Collector 和 Perses 使用 Apache-2.0；Grafana OSS 使用 AGPL-3.0。它们作为独立部署的观测组件时都可使用，但项目应在发行物和部署文档中明确许可证边界。

来源：[Prometheus LICENSE](https://github.com/prometheus/prometheus/blob/main/LICENSE)、[OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector)、[Perses](https://github.com/perses/perses)、[Grafana LICENSE](https://github.com/grafana/grafana/blob/main/LICENSE)。

## 4. 推荐状态模型

```go
type CacheStatus struct {
    ComponentType    string // SERVER / CLIENT
    ConsumerID       string
    InstanceID       string
    SnapshotGeneration uint64
    ObservedAt       time.Time
    Tables           []TableCacheStatus
}

type TableCacheStatus struct {
    TableName           string
    Revision            uint64
    CommandCursor       int64
    TargetRevision      uint64
    TargetCommandCursor int64
    DataDigest          string
    RolloutDigest       string
    LastSyncAt          time.Time
    LastSyncResult      string
    Ready               bool
}
```

Server 的 target 是自身已发布 Snapshot；Client 的 target 是最近一次从 Server 观察到的目标。状态必须从一次不可变 Snapshot 读取，不能分别读取数据、版本和游标后再拼装。

## 5. 指标层

建议的低基数指标：

```text
rcc_cache_ready{component,table}
rcc_cache_revision_lag{component,table}
rcc_cache_command_cursor_lag{component,table}
rcc_cache_last_success_age_seconds{component,table}
rcc_cache_sync_total{component,result}
rcc_cache_digest_match{component,table,digest_kind}
rcc_cache_tables{component,state}
```

实例身份应优先使用 OpenTelemetry Resource 的 `service.name`、`service.instance.id`，或 Prometheus scrape target 自带的 `job`、`instance`，而不是在每个指标中重复声明动态 ClientID。

需要谨慎的原始指标：

- revision 和 cursor 最终存入时序库时通常是浮点样本，不能作为无限范围整数的永久精确存储；
- 可以在确认值低于精确范围时用于展示，但一致性告警优先使用 lag 和 match；
- exact revision/cursor 仍由诊断接口返回整数。

## 6. 高基数约束

Prometheus 官方指出，每个 label set 都会产生额外时间序列；当某个指标的基数可能超过 100 时，应评估减少维度或转向通用处理系统。

来源：[Prometheus instrumentation practices](https://prometheus.io/docs/practices/instrumentation/)、[Prometheus data model](https://prometheus.io/docs/concepts/)。

因此禁止把以下值放入指标标签：

- DataDigest、RolloutDigest；
- CommandCursor、Revision；
- ConfigKey；
- release number；
- request ID；
- 完整错误文本；
- 无边界增长的 ClientID。

摘要每次变化都会创建新 label set。大盘只需要 `digest_match=0/1`；完整摘要在诊断接口或结构化状态记录中查看。

表维度也必须有预算。如果实例数 × 表数 × 指标数超过部署设定上限，应只输出聚合指标，并对少量 allow-list 表开启逐表指标。

## 7. 推荐数据流

```text
Server Snapshot ----> OTel observable gauges ----+
                                                  |
Client Snapshot ----> OTel observable gauges ----+--> OTLP Collector / Prometheus
                                                           |
                                                           +--> Grafana / Perses
                                                           +--> Alertmanager

Server / Client ----> protected CacheStatus interface ----> on-demand diagnosis
```

Go Client 作为库只接受调用方传入的 `metric.MeterProvider`，未提供时使用 no-op。它不得自行启动 `/metrics` 端口，也不得自行创建全局 MeterProvider。Server 是独立进程，可以在自身启动模块配置 exporter。

## 8. 大盘信息架构

### 8.1 总览

- Ready Server/Client 数量；
- 落后表数量；
- 最大 cursor lag、revision lag；
- 最久未同步时间；
- digest mismatch 数；
- Sync 失败、全量兜底和 DeltaHistory 不覆盖趋势。

### 8.2 表视图

- 当前 Server revision/cursor；
- Server 实例间 min/max 差值；
- Client revision/cursor 分布；
- 落后 Client 数和落后时长；
- Data/Rollout digest match 状态；
- 最近发布和最近同步时间。

### 8.3 实例视图

- 实例基本信息和最后观测时间；
-逐表 local/target revision、cursor 和 lag；
-最近一次同步结果；
-受保护诊断接口返回的完整摘要。

### 8.4 一致性矩阵

行表示实例，列表示表，颜色表示：一致、落后、摘要不一致、缺少订阅、实例失联。Grafana/Perses 的表格或状态历史面板可以承担展示，矩阵查询和状态码仍由 RCC recording rule 或查询适配器定义。

## 9. 推荐选择

第一版推荐：

- 埋点接口：OpenTelemetry Metrics API；
- 默认存储与查询：Prometheus；
- 默认大盘：Grafana；
- 许可证偏好为 Apache-2.0 时：Perses；
- 精确摘要：RCC `CacheStatus` 诊断接口；
- 告警：Prometheus rules + Alertmanager；
- 不引入日志转指标作为必要依赖。

若项目希望提供单体观测部署，可额外给出 SigNoz 或 OpenObserve 示例，但不应让 Client SDK 依赖任何特定后端。

## 10. 对 RCC 文档的影响

建议新增一篇跨上下文的《缓存版本可观测性规格》，再在 Server 和 Client 规格中引用其状态接口与指标契约。这样 dashboard、指标预算、诊断接口和一致性算法不会被分别复制到多个上下文。

需要先确认的产品选择：

1. 默认随项目提供 Grafana dashboard，还是采用 Apache-2.0 的 Perses dashboard；
2. 第一版是否只提供指标大盘，还是同时实现受保护的精确 `CacheStatus` 诊断页面；
3. 表规模和 Client 实例规模，用于确定逐表指标的默认预算。
