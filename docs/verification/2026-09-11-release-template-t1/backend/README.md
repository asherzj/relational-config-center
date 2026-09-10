# #101 后端请求结果修复证据

固定基点：56dff5360e94d904ab85c17cea5cc575919a29b1。工作树：/private/tmp/rcc-issue-101-release-templates。未提交、未推送；review、浏览器、迁移升级总体验收和工单收尾交回父任务。

## 修改

- 五种模板写入复用独立 `release-template:<action>` 请求命名空间，按当前账号、动作、请求键识别请求。摘要绑定动作、稳定编码、原版本及规范化正文。
- 请求锁 → 模板锁 → 业务变更 → 原结果持久化共同提交；已成功请求在读取当前目标前返回原结果。编辑结果从事务内重读，避免读到后续版本。删除完成凭据保留在请求记录，模板消失或同编码重建后仍安全重放。
- 当前登录身份和 ADMIN 授权仍每次检查。应急保护与类型不变校验在模板锁内完成。
- 合法删除默认常规模板不再破坏 readiness；必要应急模板仍须存在、启用且有效。未改任何迁移或发布单请求实现。

修改文件为 `source/` 中除 application/release_template_test.go 外的六个文件；纯规则测试文件仅快照复用。完整哈希见 source-sha256.json。正式迁移 00001～00005 对基点逐字节相同，见 published-migrations.json；候选 00008 清单仍为 d96af644b6d3dd98438287698cb929ca787f51130faff22117d9bad865c75dd2。

## HTTP 接口

所有写请求必填 `Idempotency-Key`，8～64 字符，`^[A-Za-z0-9][A-Za-z0-9._-]{7,63}$`。URL、正文、成功状态及结果形状不变：POST create 201，PUT replace / POST enable / POST disable 200，DELETE 204。重放保持原成功状态与业务响应正文；独立请求的 X-Request-ID 不要求相同。

同账号同动作同键的有效正文、目标或原版本不同：409 idempotency_conflict。新键携过时版本：409 release_template_conflict。缺失/非法键：422 invalid_release_template。当前权限不足：403 permission_denied。应急停用/删除：409 emergency_template_protected。存储不可用仍503 release_template_unavailable，客户端保留原请求并允许手动重推。

## 真实验证

环境：Go 1.27.0 darwin/arm64；Colima Docker 29.5.2，Ubuntu 24.04.4；testcontainers-go v0.44.0；MySQL 8.4。每个顶级集成测试创建独立容器和 rcc_test 库，并由 t.Cleanup 终止；未访问用户业务数据库。仅 socket 访问使用 require_escalated。

所有集成命令在 admin 目录执行，环境前缀相同：

```sh
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache
```

四条初始红灯分别保存原始失败日志 `logs/rcc-101-request-results-red-{replace,state,delete,ready}.log`：编辑/停用原请求重推409；删除原请求重推404；合法删默认常规模板后ready503。对应 green 日志全部exit 0。逐条运行使用 `go test -tags=integration ./cmd/admin -run '^完整测试名$' -count=1 -timeout=5m`。

最终范围命令：

```sh
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache go test -v -tags=integration ./cmd/admin -run '^(TestReleaseTemplateHTTP.*|TestCurrentSessionUsesRolePermissionMatrix|TestQueryPolicyHTTPLifecyclePersistsAndFailsClosed|TestMutationPolicyHTTPLifecyclePersistsRelationalRulesAndFailsClosed|TestAccountControlTablesCannotBeDiscoveredOrManaged|TestDatabaseTableListDiscoversOnlyOrdinaryBaseTables)$' -count=1 -timeout=10m
```

结果：exit 0，158.419秒，17个顶级测试全部通过。之后只追加合法正文改code/type也拒绝的断言，生产源码未变；按下列命令补跑通过，9.676秒，组成当前快照的完整证据：

```sh
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache go test -v -tags=integration ./cmd/admin -run '^TestReleaseTemplateHTTPProtectsEmergencyAndRejectsInvalidNodes$' -count=1 -timeout=5m
```

覆盖：所有写入同键原内容原结果、改变版本/正文/目标的冲突、后续版本/删除/同编码重建隔离、没有重复版本和审计、当前管理员撤权和伪造账号头、两管理员同旧版本竞争、五种相同请求并发、真实 MySQL 请求结果保存故障下的回滚及手动重推、应急保护、monitor_list为空数组、普通账号拒绝、严格受保护控制表入口、只读账号初始化应用及ready，以及既有权限/规则回归。只读检查使用公开健康接口；删除由公开管理HTTP完成。故障注入仅位于真实数据库边界，不mock自有Catalog/Repo/Service。

响应丢失采用等价重推证据：测试记录首次已提交HTTP响应后重新发送捕获的原键/正文/版本，再比对原结果和当前状态；未声称注入真实网络丢包。未新增自动重试或独立结果查询入口。

非集成与边界命令全部exit 0：

```sh
GOCACHE=/private/tmp/rcc-go-cache go test ./internal/application ./internal/interfaces/http ./internal/domain
GOCACHE=/private/tmp/rcc-go-cache go test ./cmd/admin -run '^(TestAdminHTTPCompositionCannotRunSchemaMaintenance|TestCurrentSchemaEntryPointsDoNotRestoreRetiredInitialization)$' -count=1
```

HTTP依赖方向、共享服务无请求身份字段、发布事务接缝等既有机器检查包含在对应测试包中。git diff --check通过。未运行全仓集成或浏览器；父任务负责整票两轴复核、浏览器、新装/升级等总体交付证据。
