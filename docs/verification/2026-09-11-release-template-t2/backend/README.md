# 后端验证

所有命令从工作树根目录运行。Go 1.27.0 darwin/arm64；真实数据库为串行启动、测试清理终止的 MySQL 8.4 容器。环境为 `TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache`。网络、Docker socket 和本地监听使用已授权的升级权限。

## 红绿与边界

- `01-red.log`：新增公开 HTTP 验收在路由未实现时失败，返回路由未找到。
- `02-http.log`：公开关联读取因 GORM 不映射未导出嵌入字段而返回空字段，三项失败；改成导出的嵌入字段后，`03-http-fixed.log` 三项通过（30.214s）。
- `04-contracts.log`：application 与 HTTP 选择的边界/规则测试通过；cmd 包此选择没有匹配测试，不计入 cmd 验证。
- `05-unit-and-boundaries.log`：application 和 HTTP 包全套通过；cmd 因沙箱拒绝 TCP 监听失败。`06-cmd-unit-escalated.log` 使用升级权限重跑完整 cmd 包通过（20.424s）。

```sh
GOCACHE=/private/tmp/rcc-go-cache go test ./admin/internal/application ./admin/internal/interfaces/http ./admin/cmd/admin -count=1
GOCACHE=/private/tmp/rcc-go-cache go test ./admin/cmd/admin -count=1
```

## 最终公开 HTTP 与真实 MySQL 回归

`07-current-http-regressions.log` 对应以下实际命令（带上述环境）：

```sh
go test -tags=integration ./admin/cmd/admin -run '^(TestTableReleaseTemplateHTTP.*|TestTablePolicyHTTPConcurrentCreateAndEnablePreserveEmergency|TestReleaseTemplateHTTP.*|TestCurrentSessionUsesRolePermissionMatrix|TestQueryPolicyHTTPLifecyclePersistsAndFailsClosed|TestMutationPolicyHTTPLifecyclePersistsRelationalRulesAndFailsClosed|TestAccountControlTablesCannotBeDiscoveredOrManaged|TestDatabaseTableListDiscoversOnlyOrdinaryBaseTables|TestTablePolicyCodeAssignmentsValidateActiveDefinitionsAndReplaceAtomically|TestTablePolicyCreationPersistsDisabledCodeReferencesForHTTPInspection|TestTablePolicyCreationRejectsPrincipalFailuresWithoutPartialPersistence|TestOperatorColumnsRejectIncompatibleWritesAndPreserveHistory|TestLocalManagedTableFixture.*|TestApprovedPublicationRechecksPolicyAndNextDraftUsesReplacement|TestDraftConcurrencyKeyProtectsValuesAndDefinition|TestQuickRollbackRejectsChangedSchemaRulesAndRecordVersions|TestReleaseHistorySurvivesExecutableRestartAndExternalChanges|TestCommittedWritesRemainSingleWhenHTTPResponsesAreLost|TestBusinessAPIsRequireSessionAndCSRF|TestConcurrentAccountsOwnTheirBusinessChanges)$' -count=1 -timeout=15m -v
```

新增验收覆盖不同表共享/不同模板、每表每类型唯一、同类型外键、两个独立管理员同时切换、旧版本冲突、应急禁止停用/解绑/删除、被关联标准模板不能删除、整表停用、原结果重放不覆盖后续选择、原版本/目标/内容及当前权限检查、两个创建请求共享结果、并发启用、关联创建/修复与表规则写入原子性。真实 MySQL trigger 故障使请求结果写入失败，验证关联、版本、审计与整表状态均回滚。自有 Catalog/Repository/Service 没有替身。

17 项 T1 回归重新验证共享持久请求事务、模板管理、规则生命周期、会话角色矩阵与受保护控制表；未将旧版本 T1 事务证据直接视作新共享函数的通过证据。响应丢失的真实网络注入在浏览器验收中完成，HTTP 测试通过重复原包验证持久原结果。

最终 37 个顶层用例有适用于当前源码的通过证据：批次07中的36个通过，唯一失败为旧双账号并发测试遗漏请求键/版本；只修改该测试函数后，08补跑通过（10.652s）。07原失败退出码1保留，未改记成全批通过。所有6个T2验收和T1原17项均在07通过。有效逐用例映射见 `effective-passing-coverage.json`。相同文件内其余测试函数未变，生产代码未变。

```sh
go test -tags=integration ./admin/cmd/admin -run '^TestConcurrentAccountsOwnTheirBusinessChanges$' -count=1 -timeout=5m -v
```
