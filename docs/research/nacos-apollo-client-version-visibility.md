# Nacos 与 Apollo 客户端配置版本可视化实现调查

> 调查日期：2026-08-25  
> 范围：Nacos `develop`、Apollo `master` 的官方文档和官方 GitHub 源码。本文只讨论“服务端如何知道某个客户端当前使用哪个配置版本/摘要”，不讨论配置发布本身。

## 1. 结论先行

Nacos 与 Apollo 采用了两种不同的状态模型：

- **Nacos 是连接态模型。** 客户端在长连接上按配置上报当前 MD5，服务端把 `connectionId -> groupKey -> md5` 保存在连接所在节点的内存中。控制台查询时读取这些在线连接状态；需要集群结果时，再向其他节点聚合。因此它得到的是较实时的“当前连接声称正在监听的 MD5”，但连接断开后记录会立即消失。
- **Apollo 是审计态模型。** 客户端拉取配置时携带 IP 和旧 `releaseKey`；Config Service 选出实际返回的 Release 后，把“实例最终拿到的 releaseKey”异步写入共享数据库的 `Instance`、`InstanceConfig` 表。Portal 查询数据库，并把最近约 25 小时内有更新的记录视为活跃实例。因此它得到的是可跨节点、可保留一段时间的“最近一次成功交付记录”，而不是一条实时连接状态。
- 两者的 Prometheus 指标都不是精确版本可视化的权威数据源。Nacos 的精确 MD5 来自监听状态查询 API；Apollo 的精确 `releaseKey` 来自 `InstanceConfig`。Prometheus 主要承担容量、延迟、错误、线程池和连接数量等聚合监控。

这意味着 RCC 不宜简单照搬其中一个：如果要同时回答“现在仍在线吗”和“最后成功使用了什么版本”，比较稳妥的开源设计是同时维护**短期在线租约**与**最近成功快照审计**，精确 cursor/revision/digest 通过诊断 API 查询，Prometheus 只输出低基数的 lag、age、ready 和一致性计数。

## 2. Nacos：客户端监听 MD5 的在线连接视图

### 2.1 客户端如何提交身份和 MD5

Java Client 的每个 `CacheData` 保存配置身份与本地 MD5。`ClientWorker.ConfigRpcTransportClient` 构造 `ConfigBatchListenRequest` 时，为每个配置加入 `group + dataId + tenant + cacheData.getMd5()`；连接建立或重连后还会调用 `notifyListenConfig()` 重新同步全部监听上下文。客户端也能响应服务端发来的 `ClientConfigMetricRequest`，按指定 key 返回 `CACHE_DATA` 或本地 snapshot 的内容及 MD5，不过这是一条按需诊断链路，不是监听状态的常规持久化上报链路。[ClientWorker.java](https://github.com/alibaba/nacos/blob/develop/client/src/main/java/com/alibaba/nacos/client/config/impl/ClientWorker.java)

服务端的 `ConfigChangeBatchListenRequestHandler` 从 `RequestMeta` 获取 transport 产生的 `connectionId`、`clientIp` 和客户端标签；逐项把 `dataId + group + namespace` 组装为 `groupKey`，调用 `ConfigChangeListenContext.addListen(groupKey, md5, connectionId, ...)`，随后用 `ConfigCacheService.isUptodate(...)` 比较服务端 MD5。若不一致，响应只返回变化的配置身份，客户端再查询权威内容；取消监听则调用 `removeListen`。[ConfigChangeBatchListenRequestHandler.java](https://github.com/alibaba/nacos/blob/develop/config/src/main/java/com/alibaba/nacos/config/server/remote/ConfigChangeBatchListenRequestHandler.java)

连接自身的身份不是客户端随意填写的实例 ID。Nacos gRPC transport 在连接 ready 时生成 `connectionId`，客户端发送 `ConnectionSetupRequest`，服务端把 client IP、remote endpoint、client version、app、namespace、labels、创建时间和最后活跃时间放进 `ConnectionMeta`；普通请求通过 `RequestMeta` 把这些连接元信息交给领域 handler。[远程连接生命周期规范](https://github.com/alibaba/nacos/blob/develop/specs/zh-cn/design/foundation-remote-connection-spec.md)

### 2.2 服务端保存在哪里

监听状态是**节点本地内存态**，不是 Config 数据库表。官方监听规范明确规定服务端记录：

- `groupKey -> connectionId`；
- `connectionId -> groupKey + md5`；
- 该连接是否需要 namespace 兼容转换。

实现入口是 `ConfigChangeListenContext`；权威配置内容和其 MD5 则在 `config_info` 等持久记录以及节点 dump/cache 中，两者不要混为一谈。[Config 监听规范](https://github.com/alibaba/nacos/blob/develop/specs/zh-cn/config/config-listener-watch-spec.md) [ConfigChangeListenContext.java](https://github.com/alibaba/nacos/blob/develop/config/src/main/java/com/alibaba/nacos/config/server/service/ConfigChangeListenContext.java) [Config 持久化规范](https://github.com/alibaba/nacos/blob/develop/specs/en/config/config-persistence-history-spec.md)

因此 Nacos 展示的客户端 MD5具有以下语义：它是**某个当前连接最近一次上报的监听 MD5**。它不是一条数据库审计记录，也不能证明客户端业务代码已经消费完监听回调；它能证明的是 SDK 本地 `CacheData` 的 MD5 已经被该连接上报。

### 2.3 控制台和 API 如何查询

Nacos 3.x 提供两种核心诊断方向：

- `GET /nacos/v3/admin/cs/config/listener`：按 `namespaceId + groupName + dataId` 查询订阅者，结果为 `subscriber IP -> MD5`；
- `GET /nacos/v3/admin/cs/config/listener/ip`：按订阅者 IP 反查配置，结果为 `dataId+groupName+namespaceId -> MD5`。

两者都带 `aggregation` 参数，Admin API 默认 `true`，表示聚合其他节点的数据；Console API 暴露同样能力给 Web Console。[Nacos Admin API](https://nacos.io/en/docs/latest/manual/admin/admin-api/) [Nacos Console API](https://nacos.io/en/docs/latest/manual/admin/console-api/)

当前 V3 控制器 `ConfigControllerV3.getListeners` 接收配置身份和 `AggregationForm`，把请求转给 `ConfigListenerStateDelegate.getListenerState(..., aggregation)`。这说明“是否聚合”是明确的查询契约，而不是由 Prometheus 或日志事后拼接。[ConfigControllerV3.java](https://github.com/alibaba/nacos/blob/develop/config/src/main/java/com/alibaba/nacos/config/server/controller/v3/ConfigControllerV3.java) [ConfigListenerStateDelegate.java](https://github.com/alibaba/nacos/blob/develop/config/src/main/java/com/alibaba/nacos/config/server/service/listener/ConfigListenerStateDelegate.java)

旧实现也能帮助看清调用链：`ListenerController -> ConfigSubService.runConfigListenerCollectionJob -> 各节点 communication/configWatchers`，目标节点从 `ConfigChangeListenContext` 取 `connectionId/groupKey/md5`，再从 `ConnectionManager` 解析客户端 IP。V3 已把该逻辑收敛进 listener-state delegate，但本质仍是**向拥有连接的节点查询并合并节点内存结果**。[官方仓库 Issue #12282 中附带的实现代码](https://github.com/alibaba/nacos/issues/12282)

### 2.4 断连和过期如何清理

transport terminated、主动探测失败、过载踢除或显式关闭都会触发 `ConnectionManager` 注销连接。注销时先从连接注册表删除，再调用 `ClientConnectionEventListener.clientDisConnected`，由 Config 领域释放绑定在该 `connectionId` 上的监听映射；推送失败超过重试次数也可以注销失效连接。客户端重连被视为新连接，必须重新注册监听。[远程连接生命周期规范：注销与 Active Detection](https://github.com/alibaba/nacos/blob/develop/specs/zh-cn/design/foundation-remote-connection-spec.md) [Config 监听规范：变更推送](https://github.com/alibaba/nacos/blob/develop/specs/zh-cn/config/config-listener-watch-spec.md)

所以 Nacos 不需要在数据库里定期删除“离线订阅者”：在线监听状态随连接生命周期直接从内存消失。代价是服务端重启或节点故障也会丢掉这份节点本地状态，必须依靠客户端重连和全量 resync 恢复。

### 2.5 集群节点怎样得到查询结果

一个 SDK 连接只属于一个 Nacos Server 节点，监听映射也只在该节点内存中。查询参数 `aggregation=true` 时，由被访问节点通过集群成员视图和节点间请求收集其他节点的 listener state 后合并；`aggregation=false` 只看当前节点。Nacos 的 cluster-source RPC 与 SDK-source gRPC 分离，服务端间请求通过内部 RPC handler 路由，不需要把每个客户端 MD5复制进数据库。[Nacos Admin API 的 aggregation 契约](https://nacos.io/en/docs/latest/manual/admin/admin-api/) [内部 RPC 与集群请求规范](https://github.com/alibaba/nacos/blob/develop/specs/en/design/foundation-internal-rpc-spec.md)

需要注意，官方公开 API 返回 map 的 key 是 IP，不是稳定 instance ID；同一 IP 下多个进程可能相互覆盖。因此 Nacos 的展示模型更适合“订阅诊断”，不适合直接作为严格的实例级版本账本。

### 2.6 Prometheus 扮演什么角色

Nacos Server 通过 Actuator `/nacos/actuator/prometheus` 暴露 JVM、HTTP/gRPC、长连接数、订阅数量、notify task、push latency/error 等聚合指标，要求 Prometheus 分别抓取每个节点。官方监控手册没有把逐客户端 MD5 作为 Prometheus 时序标签；精确 MD5仍走 listener Admin API。这样避免把 IP、配置身份和持续变化的 MD5 全部放进标签导致高基数。[Nacos Monitoring Manual](https://nacos.io/en/docs/latest/manual/admin/monitor/)

## 3. Apollo：配置交付审计形成的实例版本视图

### 3.1 客户端如何提交身份和版本

Apollo 有两条相关 HTTP 链路：

1. `RemoteConfigLongPollService` 调用 `/notifications/v2`，提交 `appId + cluster + ip + namespaces/notificationId`，服务端用 `DeferredResult` 挂起约 60 秒；有发布时只通知变化的 namespace。
2. 客户端随后调用 `/configs/{appId}/{clusterName}/{namespace}` 拉取内容，并提交本地旧 `releaseKey` 和 IP。服务端返回新 `releaseKey`；相同时返回 HTTP 304。

官方设计文档给出了 `RemoteConfigLongPollService -> NotificationControllerV2 -> 再拉配置` 的调用链；`ConfigController.queryConfig` 的入参则明确包含 `releaseKey`、`ip`、`label` 和 `messages`。[Apollo 设计文档](https://github.com/apolloconfig/apollo/blob/master/docs/en/design/apollo-design.md) [ConfigController.java](https://github.com/apolloconfig/apollo/blob/master/apollo-configservice/src/main/java/com/ctrip/framework/apollo/configservice/controller/ConfigController.java)

重要区别是：数据库里记录的不是客户端请求带来的旧 `releaseKey`。`ConfigController` 先按客户端 IP/label/灰度规则选出实际 Release，合并得到 `latestMergedReleaseKey`，然后对每个实际交付的 Release 调用 `InstanceConfigAuditUtil.audit(...)`。因此审计记录表达的是**服务端认为本次给该实例交付了哪个 Release**。[ConfigController.java：`auditReleases`](https://github.com/apolloconfig/apollo/blob/master/apollo-configservice/src/main/java/com/ctrip/framework/apollo/configservice/controller/ConfigController.java)

### 3.2 服务端数据结构和存储位置

`InstanceConfigAuditUtil` 先把审计事件放入有界 `BlockingQueue`，由单线程异步消费；它使用两个 Guava cache 减少数据库查写：实例键缓存 1 小时，`instanceId + configAppId + namespace -> releaseKey` 缓存 1 天。缓存不相同或需要刷新活跃时间时，最终仍写共享数据库。[InstanceConfigAuditUtil.java](https://github.com/apolloconfig/apollo/blob/master/apollo-configservice/src/main/java/com/ctrip/framework/apollo/configservice/util/InstanceConfigAuditUtil.java)

持久模型有两张关键表：

- `Instance`：`AppId, ClusterName, DataCenter, Ip, DataChange_*`，唯一标识一个消费应用实例；
- `InstanceConfig`：`InstanceId, ConfigAppId, ConfigClusterName, ConfigNamespaceName, ReleaseKey, ReleaseDeliveryTime, DataChange_*`，记录这个实例针对某个配置 namespace 最近交付的 releaseKey。

实体类分别使用 `@Table(name = "Instance")` 和 `@Table(name = "InstanceConfig")`；`InstanceConfigRepository` 提供按 releaseKey、namespace 和最后修改时间查询的方法。[Instance.java](https://github.com/apolloconfig/apollo/blob/master/apollo-biz/src/main/java/com/ctrip/framework/apollo/biz/entity/Instance.java) [InstanceConfig.java](https://github.com/apolloconfig/apollo/blob/master/apollo-biz/src/main/java/com/ctrip/framework/apollo/biz/entity/InstanceConfig.java) [InstanceConfigRepository.java](https://github.com/apolloconfig/apollo/blob/master/apollo-biz/src/main/java/com/ctrip/framework/apollo/biz/repository/InstanceConfigRepository.java)

这不是严格心跳表：审计只在 Config Service 成功处理配置拉取时刷新；队列满时 `offer` 可以失败，且审计异步异常只记 tracer。因此 Apollo 的实例视图是运维辅助信息，不属于配置读取正确性的事务边界。[InstanceConfigAuditUtil.java](https://github.com/apolloconfig/apollo/blob/master/apollo-configservice/src/main/java/com/ctrip/framework/apollo/configservice/util/InstanceConfigAuditUtil.java)

### 3.3 Portal 和 API 如何查询

Admin Service 的 `InstanceConfigController` 提供：

- `GET /instances/by-release`：查正在使用某个 `ReleaseKey` 的活跃实例；
- `GET /instances/by-namespace`：查订阅某个 namespace 的活跃实例；
- `GET /instances/by-namespace/count`：计数；
- `GET /instances/by-namespace-and-releases-not-in`：找未使用给定 Release 集合的实例，并附带其实际 Release。

这些查询最终进入 `biz.service.InstanceService` 和两个 JPA repository，完全基于数据库，而不是去 Config Service 节点读取连接内存。[InstanceConfigController.java](https://github.com/apolloconfig/apollo/blob/master/apollo-adminservice/src/main/java/com/ctrip/framework/apollo/adminservice/controller/InstanceConfigController.java) [InstanceService.java](https://github.com/apolloconfig/apollo/blob/master/apollo-biz/src/main/java/com/ctrip/framework/apollo/biz/service/InstanceService.java)

Portal 旧 WebAPI 的 `/envs/{env}/instances/by-release`、`by-namespace` 等入口通过 Portal `InstanceService -> AdminServiceAPI.InstanceAPI` 调用对应环境的 Admin Service；源码注明 Portal UI 已转向 `/openapi/v1`，旧 controller 仅保留兼容性。[Portal InstanceController.java](https://github.com/apolloconfig/apollo/blob/master/apollo-portal/src/main/java/com/ctrip/framework/apollo/portal/controller/InstanceController.java) [Portal InstanceService.java](https://github.com/apolloconfig/apollo/blob/master/apollo-portal/src/main/java/com/ctrip/framework/apollo/portal/service/InstanceService.java)

### 3.4 断连、离线和过期如何处理

Apollo 不把长轮询连接作为实例版本记录的所有者，所以客户端断开时不会立即删除 `Instance` 或 `InstanceConfig`。查询层使用 `DataChange_LastTime > validDate` 过滤活跃记录；`getValidInstanceConfigDate()` 取当前时间减 1 天再减 1 小时，即约 25 小时，用额外 1 小时容忍时差。过期记录仍留在数据库，只是不再出现在活跃实例查询里。[Apollo biz InstanceService.java](https://github.com/apolloconfig/apollo/blob/master/apollo-biz/src/main/java/com/ctrip/framework/apollo/biz/service/InstanceService.java)

因此 Apollo 的“离线”不是连接断开事件，而是**最近约 25 小时没有新的成功配置交付审计**。这使它能容忍 Config Service 重启和短暂网络中断，也让 Portal 能显示最近实例，但无法给出秒级在线状态。

### 3.5 集群节点怎样得到查询结果

每个 Config Service 都可以处理客户端拉取，并通过 `InstanceConfigAuditUtil` 把交付结果写入同一环境的 ApolloConfigDB。`Instance` 与 `InstanceConfig` 的唯一约束/并发插入由数据库承接，`DataIntegrityViolationException` 被视为可安全重试或重新查询。Admin Service/Portal 查询共享数据库，因此无需像 Nacos 那样 fan-out 到所有 Config Service 节点。[InstanceConfigAuditUtil.java](https://github.com/apolloconfig/apollo/blob/master/apollo-configservice/src/main/java/com/ctrip/framework/apollo/configservice/util/InstanceConfigAuditUtil.java) [Apollo 分布式部署设计](https://github.com/apolloconfig/apollo/blob/master/docs/en/design/apollo-design.md)

换言之，Apollo 的集群收敛点是数据库；Nacos 的集群收敛点是查询时的节点聚合。

### 3.6 Prometheus 扮演什么角色

Apollo Java Client 2.4.0 起提供 `ConfigMonitor`，可以通过 JMX 或 `MetricsExporter` SPI 导出指标；官方 Prometheus 插件示例要求业务应用自己暴露 `/metrics`。现有官方指标主要是 namespace 使用次数、item 数、首次加载耗时、not-found/timeout、异常数和客户端线程池状态，没有把 `releaseKey` 作为 Prometheus 标签，也不替代 Portal 的 `InstanceConfig` 查询。[Apollo Java Client 监控文档](https://github.com/apolloconfig/apollo/blob/master/docs/en/client/java-sdk-user-guide.md)

所以 Apollo 的精确版本页面仍来自数据库审计；Prometheus 适合回答“客户端是否频繁超时、刷新线程是否积压、加载耗时是否异常”，不适合回答“实例 X 当前是哪一个 releaseKey”。

## 4. 对 RCC 开源版的直接启示

| 设计问题 | Nacos 做法 | Apollo 做法 | RCC 建议 |
|---|---|---|---|
| 精确状态 | 连接上报 MD5 | 拉取后审计 releaseKey | Client 在成功原子替换缓存后上报 table cursor/revision/digest |
| 在线性 | 连接存在即在线 | 最近约 25 小时审计即活跃 | 使用带 TTL 的 Client Session/Lease |
| 存储 | 节点内存 | MySQL | 在线租约可内存化；最后成功状态持久化到 MySQL |
| 集群查询 | 查询时 fan-out 聚合 | 共享 DB 查询 | 第一版优先共享 MySQL；规模上来后再加内存索引 |
| 断连 | 立即清理 connection state | 不删除，按时间过滤 | 在线态立即/TTL 过期；审计态保留历史 |
| 大盘数据 | Admin listener API + 聚合指标 | Portal DB API + 客户端指标 | 精确诊断 API + 低基数 Prometheus 指标 |

RCC 与两者还有一个本质差异：RCC 一张表同时有 `Revision`、`CommandCursor`、`DataDigest`、`RolloutDigest`，不能只保存一个 MD5 或 releaseKey。建议客户端在成功 `ClientCachesPtr.Store` 后异步提交：

```text
ClientID
TableName
Revision
CommandCursor
DataDigest
RolloutDigest
ObservedAt
SDKVersion
```

上报失败不能回滚业务缓存，但必须重试；服务端以 `(ClientID, TableName)` 覆盖最新状态并维护 last-seen。控制台通过精确状态 API 构建“实例 × 表”一致性矩阵；Prometheus 只暴露 `cursor_lag`、`revision_lag`、`last_seen_age`、`digest_match` 等受控标签指标。这样保留 Nacos 的实时诊断能力，也保留 Apollo 跨节点、跨短暂断线的可查询性。

## 5. 尚未证实或需要版本锁定的部分

- Nacos `develop` 正在持续重构 V3 Admin/Console API。本文确认了公开 `aggregation` 契约和 `ConfigListenerStateDelegate` 调用入口，但没有把其内部每一种 server-to-server request 类名作为稳定契约；实现时应锁定具体 Nacos tag 再引用内部类。
- Apollo 当前 Portal UI 源码注明使用 `/openapi/v1`，但旧 controller 仍能清楚展示查询语义。本文确认的权威数据源是 Admin Service 的 `InstanceConfigController` 与 `Instance/InstanceConfig` 表，而不依赖某一版前端路由名称。
- Apollo 的实例审计是有界异步队列，官方源码没有提供“审计写入成功”的客户端确认。因此不能把 Portal 中没有记录解释成客户端一定离线，只能解释成最近有效窗口内没有可查询的成功审计。
