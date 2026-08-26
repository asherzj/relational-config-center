# 关系型配置中心开源项目调研

> 核查日期：2026-08-24（Asia/Shanghai）；Bean Searcher 补充核查：2026-08-25
> 来源范围：仅使用项目官方 GitHub 仓库、官方文档和官网。  
> 结论性质：项目选型与边界研究，不构成许可证法律意见。

## 结论摘要

截至核查日，没有发现一个仍采用无用途限制的开源许可证、且同时完整覆盖本项目两条主线的成熟项目：

1. **关系型配置管理面**：对既有关系表进行实体、字段、外键关系、约束、关联查询和受控行变更；
2. **配置运行时分发面**：发布/回滚、灰度、版本游标、客户端监听、本地缓存和断线恢复。

现有项目明显分成两类：

- **Mathesar、Hasura、Teable、Grist** 具备较强的关系建模、关系查询或数据 API 能力，但没有 Apollo/Nacos 式、以一次配置发布为边界的运行时配置分发语义。
- **Apollo、Nacos** 已具备成熟的发布、版本、灰度和客户端热更新能力，但其服务端把配置建模为 key/value 或不透明内容，不理解内容内部的实体关系和数据库约束。Nacos 官方甚至明确说明它只保存和分发 `content`，不理解配置文件内部业务字段的含义。[Nacos 配置模型](https://nacos.io/docs/latest/manual/user/config/overview/)

因此，本项目并非在重做一个已经完整存在的开源产品。最接近的组合式参照是：

- **Admin 管理面参照**：Mathesar（既有数据库和数据库原生权限）、Hasura（关系 API 与声明式权限）、Teable/Grist（关系数据编辑体验和历史）；
- **Query Policy 能力增强参照**：Bean Searcher（动态条件、多表查询、字段选择、聚合以及可扩展的参数解析与 SQL 生成）；
- **Server/Client 参照**：Apollo（发布与客户端可靠更新）、Nacos（监听、灰度、历史和多语言 SDK）；
- **最接近的单体产品参照**：Directus，但其当前许可证已经带有竞争用途限制，只能作为 source-available 参照，不能按无用途限制的开源替代品处理。[Directus 当前许可证](https://github.com/directus/directus/blob/main/directus/license)

## 本项目比较基线

本项目不是传统键值配置中心，也不是通用数据库设计器。当前仓库将自己定义为面向实体、字段和关系建模的配置管理系统，管理单元是具有 Schema、约束、引用和关联查询需求的配置数据。[项目 README](../../README.md)

当前已经冻结并实现的是 Admin 第一迭代：

- 一个部署只治理一个由部署配置指定的 MySQL Managed Data Source；
- Admin 不拥有业务表 Schema，不创建、迁移或删除业务表，只依据实时元数据治理既有表的行；
- 只有拥有 enabled Table Policy 的普通基表才成为 Managed Table，缺失或失效 Policy 时默认拒绝；
- 当前只提供受 Policy 约束的单表查询和按 `id` 的行变更；
- 关系查询、发布/回滚、持久化审计、Server、Client/SDK 和动态分发仍属于后续迭代。[Admin V1 技术基线](../admin-v1-technical-baseline.md) [架构说明](../architecture.md)

后续传播备忘提出的目标语义是：MySQL 当前表为事实来源；每次已提交变更拥有唯一、单调递增的全局 `revision`；Server 按序拉取增量，通知只降低延迟；历史缺口通过全量快照恢复。[Admin 到 Server 传播备忘](../design-notes/admin-to-server-change-propagation.md)

这组边界是下文判断“相似”的依据，而不是仅依据项目名称中是否出现“configuration”。

## 能力总览

符号说明：`✅` 为官方资料明确支持；`◐` 为能力存在但语义或范围不同；`—` 为本次审阅的官方资料没有找到对应能力，**不等于证明全部源码中绝对不存在**。

| 项目 | 关系数据模型/查询 | 既有业务库 | 配置发布/回滚 | 权限 | 审计/历史 | 运行时 API、监听或 SDK | 与本项目的主要重合 |
|---|---|---:|---:|---:|---:|---:|---|
| Mathesar | ✅ PostgreSQL FK、引用列、关系模式、Exploration | ✅ PostgreSQL | — | ✅ PostgreSQL role/privilege | — | ◐ 不稳定 JSON-RPC API | Admin 的既有关系表管理 |
| Hasura GraphQL Engine v2 | ✅ 数据库关系、嵌套 GraphQL 查询 | ✅ live database | ◐ Metadata/migration 可版本控制，不是配置数据发布 | ✅ 行/列/操作级规则 | ◐ 事件调用日志，不是行版本审计 | ✅ GraphQL/REST、订阅、Event Trigger | Server 关系查询、授权和实时 API |
| Bean Searcher | ✅ 多表映射、动态过滤/排序、子查询、分组聚合 | ✅ 复用 Java 应用 DataSource | — | — 由宿主应用负责 | — | ◐ Java 嵌入式查询库，不是独立运行时服务 | Admin Query Policy 的查询执行能力 |
| Teable CE | ✅ Link、Lookup、Rollup、关系记录 API | ◐ 自己拥有 PostgreSQL Schema；不是无侵入接管任意既有表 | — | ◐ 基础角色；细粒度 Authority Matrix 为 Pro+ | ✅ 记录历史；集中 Audit Log 版本边界需单独核实 | ◐ REST API，无 Apollo 式配置监听 SDK | 关系型配置编辑 Web/Admin 体验 |
| Grist Community | ✅ Reference/Reference List、公式和关联布局 | — 自有 document 模型 | ◐ 文档快照可恢复，不是发布流 | ✅ 条件式 Access Rules | ✅ Activity、Snapshots；集中 Audit Logs 属 Full Edition | ◐ REST API、Webhooks，无配置客户端缓存协议 | 关系编辑、历史与权限 UX |
| Apollo | — key/value 或文件型 namespace | — | ✅ 发布版本、回滚、灰度 | ✅ 管理权限、OpenAPI 授权 | ✅ 发布历史、操作 Trace | ✅ Java/.NET、HTTP、第三方 SDK；热更新、本地缓存 | Server/Client 的发布与可靠动态更新 |
| Nacos | — `namespace + group + dataId -> content` | — | ✅ 正式/灰度发布；历史内容重新发布实现回滚 | ✅ namespace read/write RBAC；可插拔认证 | ✅ 发布/删除/灰度历史 | ✅ Java/Go/Python SDK；监听、本地缓存、重连 | Server/Client 的监听、灰度和恢复 |

## 第一组：关系型数据治理与 API

### 1. Mathesar

**定位与数据模型**

Mathesar 是直接作用于 PostgreSQL 数据库、Schema 和表的自托管数据管理 UI，不增加一层业务数据抽象；可以连接既有 PostgreSQL 数据库，也能创建和修改 Schema、表与行。[官方仓库与功能列表](https://github.com/mathesar-foundation/mathesar) 这使它成为当前开源候选中与 Admin“治理既有关系表”最接近的项目。

Mathesar 从 PostgreSQL foreign key constraint 识别引用列，也会在创建引用列时写入真实外键；官方文档覆盖一对多、多对一、通过中间表实现的多对多、自引用等关系，并能在引用单元格中展示记录摘要。[关系文档](https://docs.mathesar.org/latest/user-guide/relationships/)

**权限、版本、审计和运行时能力**

- 数据访问直接使用 PostgreSQL role/privilege；协作者在每个数据库上绑定一个 PostgreSQL role，表级可使用 `SELECT`、`INSERT`、`UPDATE`、`DELETE` 等数据库原生权限。[Access Control](https://docs.mathesar.org/latest/user-guide/access-control/) [表权限](https://docs.mathesar.org/latest/user-guide/tables/)
- 需要注意：官方文档明确说明 Mathesar metadata 和 Explorations 当前对同一数据库的所有协作者均可读写，不受其 PostgreSQL role 约束。[Access Control 限制](https://docs.mathesar.org/latest/user-guide/access-control/)
- 它提供前端所使用的 JSON-RPC API，但官方将其标为尚不稳定，未来可能无预警破坏兼容性；未提供面向配置消费端的监听、缓存或断线恢复 SDK。[API 概览](https://docs.mathesar.org/latest/api/)
- 官方资料没有把行数据变更组织成 draft → publish → rollback 的配置发布流，也没有找到 Apollo/Nacos 式发布 revision 或客户端消费游标。

**与本项目的关键差异**

Mathesar 是 PostgreSQL 通用数据管理器，并且允许改变数据库 Schema；本项目第一迭代固定 MySQL、单数据源、默认拒绝，并明确不拥有 Managed Table Schema。[ADR-0001](../adr/0001-admin-governs-existing-tables.md) 因此它适合借鉴数据库发现、关系浏览、记录选择器和数据库原生权限映射，不适合直接代替本项目的 Table Policy 与后续配置发布/分发语义。

**许可证与活跃度**

Mathesar 使用 GPL-3.0，官方仓库将项目状态标为 public beta。[仓库与许可证](https://github.com/mathesar-foundation/mathesar) GitHub Releases 在核查时显示最新稳定版为 **0.12.0，发布于 2026-07-02**。[Releases](https://github.com/mathesar-foundation/mathesar/releases)

### 2. Hasura GraphQL Engine v2

**定位与数据模型**

Hasura v2 把既有 live database 暴露成带细粒度授权的 GraphQL/REST API，支持过滤、分页、批量增删改和 GraphQL subscription。[V2 官方 README](https://github.com/hasura/graphql-engine/blob/master/V2-README.md) Hasura Metadata 描述数据源、已跟踪的表/列、关系、权限、函数、computed field 等，可以导出为 JSON/YAML。[Metadata 工程说明](https://hasura.io/blog/a-hasura-2-0-engineering-overview)

关系查询是 Hasura 相对本项目当前单表 Query Policy 的主要增量：关系进入 GraphQL Schema，客户端可做嵌套查询；Metadata 还可以声明数据库外的 remote relationship。[Metadata 管理](https://hasura.io/learn/graphql/hasura-advanced/migrations-metadata/3-metadata/)

**权限、版本、审计和运行时能力**

- 可按 role 配置 select/insert/update/delete，并组合行条件、允许列和 session-variable preset；因此其权限粒度显著细于本项目当前 Table Policy 的表级能力开关。[权限示例](https://hasura.io/learn/graphql/hasura/authorization/1-todos-table-permissions/)
- Metadata 和数据库 Schema migration 可以文件化、应用并进入 Git 版本控制，但这是**服务/API 元数据和 Schema 的部署版本**，不是业务配置行的 draft、发布快照或回滚历史。[Metadata 管理](https://hasura.io/learn/graphql/hasura-advanced/migrations-metadata/3-metadata/) [Migration 管理](https://hasura.io/learn/graphql/hasura-advanced/migrations-metadata/2-migration-files/)
- GraphQL subscription 可向客户端持续返回查询结果变化；Event Trigger 可可靠捕获指定表的变更并调用 webhook。[订阅架构](https://github.com/hasura/graphql-engine/blob/master/architecture/streaming-subscriptions.md) [Event Trigger](https://hasura.io/learn/graphql/hasura/custom-business-logic/)
- Event Trigger 的 invocation log 是投递可观测性，不等同于对任意配置行提供可恢复的审计 revision。官方 OSS 资料中没有发现通用行版本审计或配置发布审批流。

**与本项目的关键差异**

Hasura 更像本项目未来 Server 的通用替代方向，而不是 Admin：它让客户端提交灵活 GraphQL，并通过声明式权限控制数据库访问；本项目的 Admin 则用封闭 Query Spec、策略构造器、复杂度限制和动态值类型解析缩小操作面。Hasura 的 Metadata-as-code、关系授权、订阅复用和事件可靠投递值得重点借鉴，但不能代替“当前配置表 + 单调 revision + 可恢复增量”的发布语义。

**许可证与活跃度**

本比较固定在当前稳定的 v2：其 core GraphQL Engine 为 Apache-2.0，v2 目录其他指定内容为 MIT。[官方仓库许可证说明](https://github.com/hasura/graphql-engine) 核查时 GitHub Releases 的最新版本为 **v2.49.5，发布于 2026-07-21**。[Releases](https://github.com/hasura/graphql-engine/releases)

### 3. Teable Community Edition

**定位与数据模型**

Teable 是以 PostgreSQL 为底座的关系型 spreadsheet/database 产品。一个 Base 对应一个 PostgreSQL Schema；Link field 表达记录关系，Lookup 和 Rollup 能沿 Link 读取或聚合相关记录。[Base 与 PostgreSQL 映射](https://help.teable.io/en/basic/base) [Link/Rollup](https://help.teable.io/en/basic/field/advanced/rollup)

其 REST API 能创建表、字段、视图和初始记录，字段模型显式包含 relation、foreign table/key、one-to-one 等信息；记录 API 支持按 Link field 写入关联记录。[Create Table API](https://help.teable.io/en/api-reference/table/create-table) [记录字段类型](https://help.teable.io/zh/api-doc/record/interface)

**权限、版本、审计和运行时能力**

- Community 产品具备 Space/Base 的 Creator、Editor、Commenter、Viewer 等协作角色。[Base Collaborators](https://help.teable.io/en/basic/space/base-invite)
- 细到视图、记录和字段的 Authority Matrix 官方标记为 **Pro plan and above**，不能把它直接计入无附加条件的 Community Edition 能力。[Authority Matrix](https://help.teable.io/en/basic/authority-matrix)
- Record History 可查看表或单条记录的 cell-level 变化并帮助恢复误删数据。[Record History](https://help.teable.io/en/basic/record/record-history)
- 有完整记录 REST API，但没有找到 Apollo/Nacos 式客户端监听、本地缓存与断线恢复协议；Record History 也不等于将多表配置作为一致发布单元。

**与本项目的关键差异**

Teable 拥有自己的 PostgreSQL Schema 和系统字段；官方 SQL 接入文档明确不允许外部连接直接写底层数据库，写操作应经过 Teable API。[SQL 访问边界](https://help.teable.io/zh/api-doc/sql-query) 本项目则接管部署者在系统外维护的既有 MySQL 表，并刻意不保存字段模型、不迁移 Schema。因此 Teable 更适合借鉴关系字段表单、关联记录选择、Lookup/Rollup、记录历史和 Web 交互，而不是复用其持久化边界。

**许可证与活跃度**

Teable 仓库声明 core apps 使用 AGPL-3.0、`packages/` 使用 MIT，但根 LICENSE 还附加了品牌资产不可修改/移除的条款；采用前应单独做许可证兼容性审查。[Teable LICENSE](https://github.com/teableio/teable/blob/develop/LICENSE) GitHub Container Registry 在核查窗口显示 **`release.2026-08-09T14-50-08Z.2564`** 为最新发布镜像，并且官方 GitHub 组织页显示主仓库在 2026 年 8 月仍有更新。[Container package](https://github.com/teableio/teable/pkgs/container/teable) [Teable GitHub 组织](https://github.com/teableio)

### 4. Grist Community Edition

**定位与数据模型**

Grist Community 是 Apache-2.0 的自托管“relational spreadsheet”。它在自己的 document 模型中使用 Reference 和 Reference List 连接记录，支持一对一、一对多、多对一、多对多、双向引用，以及通过公式沿引用读取相关字段。[官方仓库](https://github.com/gristlabs/grist-core) [Reference 文档](https://support.getgrist.com/col-refs/)

**权限、版本、审计和运行时能力**

- Access Rules 可以依据记录、用户属性和引用字段编写条件式访问控制。[Access Rules](https://support.getgrist.com/access-rules/)
- Document History 包含 Activity 和 Snapshots；Snapshots 是工作期间自动保存的完整文档备份，可比较和恢复旧版本。[Document History](https://support.getgrist.com/document-history/) [Automatic Backups](https://support.getgrist.com/automatic-backups/)
- REST API 可操作站点、workspace 和 document，API key 继承用户权限；Webhook 可在行新增或修改时通知外部系统。[REST API](https://support.getgrist.com/rest-api/) [Webhooks](https://support.getgrist.com/webhooks/)
- 这些能力仍没有提供一次关系型配置发布的稳定 revision、面向长连接客户端的游标协议和本地缓存恢复。官方发布说明还将集中 Audit Logs 标为 **Full Grist edition**，不能与 Community 的 Activity/Snapshots 混为一谈。[v1.7.16 release notes](https://github.com/gristlabs/grist-core/releases/tag/v1.7.16)

**与本项目的关键差异**

Grist 数据保存在其 document 抽象中，并不治理部署者已有的 MySQL 业务表；Reference 也不是直接复用既有数据库 foreign key。因此它主要是 Web/Admin 的产品体验参照，尤其是关系字段、关联布局、条件权限、活动时间线、快照比较与恢复。

**许可证与活跃度**

`grist-core` Community Edition 使用 Apache-2.0；官方同时提供包含未激活 source-available Full Edition 代码的默认镜像，也提供只含自由开源代码的 `grist-oss` 镜像。[官方仓库许可证与镜像说明](https://github.com/gristlabs/grist-core) GitHub Releases 在核查时显示最新版本为 **v1.7.16，发布于 2026-06-30**。[Releases](https://github.com/gristlabs/grist-core/releases)

## 第二组：Query Policy 能力增强候选组件

### Bean Searcher

**定位与查询能力**

Bean Searcher 是面向 Java 的只读动态查询库：由实体或 `SearchBean` 声明查询边界，以 HTTP 参数或参数 Map 表达过滤、排序、分页和统计。它支持单表零注解、多表映射与 Join、动态字段操作符、字段选择/排除、分组聚合、Select/Where/From 子查询，以及 Bean/Map 两种动态结果形态；可嵌入 Spring Boot、Solon 或直接绑定通用 `DataSource`。[官方 README](https://github.com/troyzhxu/bean-searcher/blob/dev/README.zh-CN.md)

项目还把 `FieldOp`、`FieldConvertor`、`DbMapping`、`ParamResolver`、`Dialect` 和 SQL 拦截器作为扩展点。这些扩展边界与本项目未来增加 Query Policy Type、关系查询、字段转换和独立数据库 Adapter 时需要解决的问题直接相关。

**与本项目的重合和候选定位**

它与当前 Admin 的重合集中在 `Query Spec -> 参数与字段校验 -> 动态 SQL -> Count/Scan -> 动态结果` 执行链，不覆盖 Table Policy、Policy 生命周期、实时 Schema 治理、Mutation Policy、认证、管理 API 或后续配置发布与分发。因此将它记录为 **Query Policy 能力增强的选型与行为基准**，而不是 Admin 的整体替代方案。

当前 Admin 使用 Go + GORM，并已冻结“单一主要 DAL、封闭 Query Spec、单表查询和受控 GORM Clauses”的边界。[ADR-0004](../adr/0004-mysql-gorm-and-validated-dynamic-queries.md) 所以近期优先级应是借鉴其能力分解和扩展点，而非直接引入依赖。只有未来出现 Java 查询 Adapter 或独立 JVM 查询服务的明确需求时，才评估直接集成；引入 JVM sidecar 本身不能仅由减少动态查询代码量来证明合理。

后续评估必须继续满足本项目现有安全与一致性门槛：表只能来自 enabled Table Policy，字段必须经实时 Schema 与 Policy 校验，操作符和排序保持封闭枚举，值使用参数绑定，并保留复杂度上限与一次请求内一致的 Policy Snapshot。Bean Searcher 更自由的客户端查询能力不能未经收敛直接暴露。

**许可证与活跃度**

Bean Searcher 使用 Apache-2.0；官方 Releases 在补充核查时显示最新版本为 **v4.8.12，发布于 2026-07-27**，并同时提供 JDK 8 构建。[官方仓库](https://github.com/troyzhxu/bean-searcher) [Releases](https://github.com/troyzhxu/bean-searcher/releases)

## 第三组：动态配置发布与分发

### 5. Apollo

**定位与数据模型**

Apollo 面向不同 application、environment、cluster 和 namespace 统一管理微服务配置。Properties namespace 是 key/value；也支持 XML、JSON、YAML 等文件型 namespace，但平台通常只做格式级处理，不提供实体关系、外键或关联查询。[官方 README 与功能列表](https://github.com/apolloconfig/apollo/blob/master/README.md)

**发布、版本、权限、审计和 SDK**

- 每次配置 release 都有版本并支持 rollback；同时支持仅对部分实例生效的灰度发布。[功能列表](https://github.com/apolloconfig/apollo/blob/master/README.md)
- Portal/OpenAPI 提供多环境管理、权限与流程治理；OpenAPI 规范还列出 Permission、Namespace Lock、Namespace Branch、Instance 和 AccessKey 管理接口。[Apollo OpenAPI](https://github.com/apolloconfig/apollo-openapi/blob/main/apollo-openapi.yaml)
- 官方原生 Java 和 .NET SDK，另有 HTTP API 与多种第三方 SDK；Java client 通过 HTTP long polling 尽快感知变更，定时拉取作为兜底，并把配置缓存到内存和本地文件，应用可订阅变化通知。[Java SDK 设计](https://github.com/apolloconfig/apollo/blob/master/docs/en/client/java-sdk-user-guide.md)
- Release history 可配置保留数量，并用于配置回滚；Apollo 2.5.0 又加入客户端增量配置同步。[部署参数](https://github.com/apolloconfig/apollo/blob/master/docs/en/deployment/distributed-deployment-guide.md) [2.5.0 release notes](https://github.com/apolloconfig/apollo/releases/tag/v2.5.0)

**与本项目的关键差异和借鉴点**

Apollo 缺少关系型数据语义，不能作为 Admin 的直接替代。但其“修改不等于发布”、release snapshot、灰度、回滚、客户端长轮询 + 定时拉取兜底 + 本地缓存，和本项目后续 Server/Client 的需求高度重合。尤其值得对照传播备忘中的“不依赖通知可靠性”和“全局 revision + 增量拉取”设计。

**许可证与活跃度**

Apollo 使用 Apache-2.0。[LICENSE/README](https://github.com/apolloconfig/apollo/blob/master/README.md) 核查时最新版本为 **v2.5.2，发布于 2026-07-12**。[Releases](https://github.com/apolloconfig/apollo/releases)

### 6. Nacos

**定位与数据模型**

Nacos 的配置资源由 `namespaceId + groupName + dataId` 唯一标识，值是一个 `content`；官方明确说明 Nacos 不理解配置内容中某个业务字段的意义。[配置概览](https://nacos.io/docs/latest/manual/user/config/overview/) 发布成功后会记录变更并通知节点刷新本地缓存。[Publish, Query and Listen](https://nacos.io/en/docs/latest/manual/user/config/publish-query-listen/)

**发布、版本、权限、审计和 SDK**

- 同一个配置资源可有正式版本和多个 gray version；内置 Beta IP 和 Tag 规则，未匹配灰度时回退正式配置。[Gray Release](https://www.nacos.io/en/docs/next/manual/user/config/gray-release/)
- 正式发布、删除、灰度发布和灰度删除都会写历史；历史记录包含操作者、时间、内容和发布类型。回滚的操作语义是审阅历史后，把旧内容重新发布为正式配置。[History and Rollback](https://nacos.io/en/docs/latest/manual/user/config/ops-and-troubleshooting/)
- Config Admin API 以 namespace 为授权边界区分 `read` 与 `write`；默认、LDAP、OIDC/OAuth2 和自定义认证插件可选，但官方也强调内置认证适合可信内网，不是抵抗恶意攻击的强认证系统。[Admin API](https://nacos.io/en/docs/latest/manual/admin/admin-api/) [Authorization](https://nacos.io/en/docs/latest/manual/admin/auth/)
- 官方维护 Java、Go、Python Client SDK；Client SDK 管理连接、本地缓存、listener/subscription 和重连恢复，Java 是运行时语义的参考实现。[SDK Overview](https://nacos.io/en/docs/latest/manual/user/overview/other-language/)

**与本项目的关键差异和借鉴点**

Nacos 同样不提供关系建模或关联查询，因此不能替代 Admin。它最值得借鉴的是：正式/灰度配置身份模型、历史字段、namespace 授权、管理 SDK 与消费 SDK 分离、listener + 本地 cache + reconnect，以及数据库为事实来源、节点本地 dump 只做 cache 的恢复边界。[Operations and Troubleshooting](https://nacos.io/en/docs/latest/manual/user/config/ops-and-troubleshooting/)

**许可证与活跃度**

Nacos 使用 Apache-2.0。[官方贡献说明](https://github.com/alibaba/nacos/blob/develop/CONTRIBUTING.md) 核查时最新稳定版为 **3.2.3，发布于 2026-07-14**；另有 **3.3.0-BETA，发布于 2026-08-06**。[Releases](https://github.com/alibaba/nacos/releases)

## 高相似但当前不应计为无用途限制开源替代品

### Directus

从产品能力看，Directus 是最接近完整目标形态的单体参照：它可为 SQL database 提供管理 UI、REST/GraphQL/SDK，支持标准关系类型；Content Versions 可创建未发布副本、比较并 promote 为 main；Activity/Revisions 记录操作者和 delta，并可恢复旧状态。[Data Model](https://docs.directus.io/app/data-model) [Relationships](https://docs.directus.io/app/data-model/relationships) [Content Versions](https://docs.directus.io/reference/system/versions) [Activity](https://docs.directus.io/reference/system/activity)

但是当前仓库许可证是 **MSCL-1.0-GPL**：禁止与许可方商业产品竞争的用途，并规定代码发布四年后才自动获得 GPL-3.0 许可。[当前 license 文件](https://github.com/directus/directus/blob/main/directus/license) 因此它应标记为 **source-available、未来转 GPL**，不应作为当前无用途限制的开源替代品。项目仍高度活跃，官方 GitHub 在 2026-08 显示持续提交。[Directus GitHub 组织](https://github.com/directus)

Directus 仍是最有价值的产品设计基准，特别是关系字段、细粒度 policy、Activity/Revisions、draft/main promotion、SDK 和 realtime；同时要注意其官方文档明确说明，直接绕过 Directus 修改数据库的操作不会进入 Activity Log。[Activity Log 边界](https://docs.directus.io/user-guide/settings/activity-log)

### NocoDB

NocoDB 可以管理字段和关系、展示 ERD、生成 REST/API snippets 和 Webhooks，因此也是关系数据管理的近邻。[Table Details](https://nocodb.com/docs/product-docs/table-details) 但其 `develop`/`master` 自 2026-01 起采用 **Sustainable Use License**，只允许内部商业用途或非商业/个人用途，并限制再分发方式。[当前 LICENSE](https://github.com/nocodb/nocodb/blob/develop/LICENSE.md) 所以本调研不把当前 NocoDB 计入开源候选主表。核查时 Releases 显示 **2026.05.2，发布于 2026-05-27**，说明项目本身仍在活跃发布。[Releases](https://github.com/nocodb/nocodb/releases)

## 对本项目路线的启示

### 1. 不要把 Admin 扩张成通用数据库设计器

Mathesar、Teable 和 Directus 都证明“可视化建表/改表”是一个完整而庞大的产品域。本项目现有 ADR 选择 Admin 不拥有 Managed Table Schema，能保持更小且更安全的边界；应继续把关系元数据读取、关系配置治理和业务 Schema migration 分开。

### 2. 关系能力应优先复用数据库事实，而不是再造 Link 字段真相源

Mathesar 直接把 PostgreSQL foreign key 作为关系事实，Hasura 则把数据库关系与 Metadata 中的逻辑关系组合起来。对本项目而言，若后续加入关系查询，优先考虑：

- 物理 foreign key 是可验证事实；
- Table Policy 只表达允许暴露/遍历哪些关系及复杂度上限；
- Query Spec 仍保持声明式和封闭操作符，避免开放任意 SQL/GraphQL；
- 不把既有表字段和关系完整复制进 Policy Catalog，除非确有独立版本或审计需求。

这与当前“每次操作读取实时 Schema、不保存 field infos”的决策一致。[ADR-0001](../adr/0001-admin-governs-existing-tables.md)

### 3. 发布版本不能直接等同于逐行审计 revision

调研中出现三种不同概念：

- Directus/Teable/Grist 的 item/record/document history：面向编辑追踪和恢复；
- Hasura Metadata/migration：面向 API/Schema 部署；
- Apollo/Nacos release history：面向运行时生效版本和客户端分发。

本项目后续需要先决定发布原子边界：一行、一个 Managed Table、一组相关表，还是一个命名空间。若需要关系一致性，单行历史不能自动解决跨表快照、引用完整性和原子 promote。

### 4. Server/Client 应把可靠性建立在可补拉事实源上

Apollo 使用长轮询尽快通知并以定时拉取兜底，Nacos 的本地 dump/cache 也不是事实源；二者都支持本地缓存和重连恢复。这与仓库传播备忘“通知只唤醒、正确性依赖全局 revision 和可补拉日志”的方向一致，应继续保持。

### 5. 权限必须区分管理者、发布者和运行时消费者

Hasura 展示了行/列/操作级授权，Apollo 展示了 namespace lock、release 和 OpenAPI 权限，Nacos 明确分离 Maintainer SDK 与 Client SDK。后续不宜只把当前部署级 Bearer Token 横向扩展，而应分别建模：

- 谁可以修改 Table Policy；
- 谁可以编辑配置行；
- 谁可以发布/回滚；
- 哪个应用/环境/实例可以读取哪些已发布配置。

## 建议的后续验证

若准备进入技术选型或原型阶段，建议按以下顺序做小规模 spike，而不是直接引入上述产品：

1. 用两张带 foreign key 的 MySQL 配置表扩展 Query Spec，验证关系 allowlist、join 深度、分页和循环关系限制；
2. 用一次跨两表变更验证 draft、校验、原子 publish、release snapshot 和 rollback 的事务边界；
3. 参考 Apollo/Nacos 验证 Go Client 的启动全量快照、revision 增量、长轮询/流式通知、定时补拉、本地缓存和历史缺口恢复；
4. 分别对照 Hasura、Apollo、Nacos 设计管理授权、发布授权和运行时读取授权；
5. 若考虑直接集成第三方，先做许可证和 Edition 能力核对，特别是 Teable 的附加品牌条款、Grist Full Edition、Directus MSCL 和 NocoDB Sustainable Use License。
6. 当 Query Policy 准备增加动态字段选择、关系查询或聚合时，以 Bean Searcher v4.8.x 作为行为基准，用同一组 MySQL fixture 比较操作符语义、字段边界、SQL 安全、Count 一致性和复杂度限制，并评估 Go 原生实现与 JVM sidecar 的全生命周期成本。

## 活跃度核查表

活跃度只作为维护信号，不等于成熟度或安全性。版本号与日期均来自官方 GitHub Releases/Packages 页面在核查日可见的记录。

| 项目 | 核查到的最新稳定 release/package | 其他可核信号 |
|---|---|---|
| Mathesar | 0.12.0 — 2026-07-02 | 官方仍标 public beta；GPL-3.0 |
| Hasura GraphQL Engine v2 | v2.49.5 — 2026-07-21 | v2 core Apache-2.0；仓库 2026-08 仍更新 |
| Bean Searcher | v4.8.12 — 2026-07-27 | Apache-2.0；Java 查询库，同时提供 JDK 8 构建 |
| Teable | release.2026-08-09T14-50-08Z.2564 — 2026-08-09 | 官方组织页显示 2026-08 仍更新 |
| Grist Core | v1.7.16 — 2026-06-30 | Community Apache-2.0；Full Edition 另有许可证边界 |
| Apollo | v2.5.2 — 2026-07-12 | Apache-2.0 |
| Nacos | 3.2.3 — 2026-07-14 | 3.3.0-BETA — 2026-08-06；Apache-2.0 |
| Directus（source-available） | 官方仓库 2026-08 仍持续提交 | 当前 MSCL-1.0-GPL，四年后转 GPL-3.0 |
| NocoDB（source-available） | 2026.05.2 — 2026-05-27 | 当前 Sustainable Use License |

## 核查方法与限制

- 先阅读本仓库 README、Context Map、Admin Context、技术基线、架构、传播备忘及相关 ADR，以项目自己的领域术语建立比较维度。
- 候选项目只引用其官方 GitHub 仓库、官方文档和官网；没有使用评测文章、聚合榜单或第三方许可证摘要。
- “没有找到”只表示在官方 README、用户/开发者文档、API 文档、LICENSE 和近期 release notes 中未发现，不把它表述为源码级不存在证明。
- Edition 差异按官方资料保守处理：明确标为 Pro、Enterprise、Full Edition 或 source-available 的能力，不计入无附加条件的 Community/OSS 能力。
- 活跃度使用 release/package 日期和官方 GitHub 最近更新作为可重复核查的代理指标，不使用 star 数作为质量判断。
