# 模板与审批依赖集成：Schema 验证

工作树 `/private/tmp/rcc-release-template-approval-integration`，2026-09-11。合并双方为模板 T2 `7eb9dd36affe16739d8e8b178afdd9977d4976a3` 与表级审批 T2 `2612bb354ef71d42e193648564b9f30bd9281eb4`。本次只接通已交付依赖，不实现 #103 的流程实例。

## 结果和原始证据

- 运行 01 为真实失败：在只含模板部分的旧候选 manifest 下，六张审批控制表逐个缺失时 `/health/ready` 都错误返回 200。原日志及 exit1 保留。
- 运行 02 在正式版本 7 的隔离 MySQL 上依次应用未部署的候选 8、9，逐表读取实际 `SHOW CREATE TABLE`，分别生成包含 27、28 张控制表的累计 manifest。生成通过，10.903s。只移除运行时 AUTO_INCREMENT 计数器；未拼接、猜测结构或放宽生产校验。临时生成器已归档为 `manifest-generator.go.txt` 并从源码目录删除。
- 运行 03 的 14 个顶层用例及所有子用例全部通过，197.021s，exit0。`effective-passing-coverage.json` 逐项指向实际通过日志。
- 自动权限检查首次超时，命令未执行。允许的一次重试成功启动运行 03；详情见 `permission-timeout.txt`，不把未执行的尝试视为测试结果。

环境：Go 1.27.0 darwin/arm64，MySQL `mysql:8.4`，Docker 29.5.2，testcontainers-go v0.44.0。所有容器串行启动、停止和删除；没有调用生产数据库。运行环境变量为 `TESTCONTAINERS_RYUK_DISABLED=true`、`DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`GOCACHE=/private/tmp/rcc-go-cache`。

从工作树根目录带上述环境变量运行：

```sh
go test -C admin -tags=integration ./cmd/admin -run '^(TestTemplateApprovalSchemaPreservesPublishedRolesAndChecksCompleteReadiness|TestTableReleaseSchemaMigratesExistingTablesAndPreservesSelections|TestTableReleaseSchemaReadinessRejectsMissingEmergencyAssociationReadOnly|TestTableReleaseSchemaDoesNotRestoreDisabledOrDeletedStandardDefaults|TestTableReleaseSchemaRecoveryKeepsExistingSelections|TestSchemaBaselineAdoptsCurrentDatabaseWithoutReplayingHistory|TestAccountUpgradeFromLegacyMatchesFreshSchema|TestSchemaReadinessContinuouslyChecksStateAndCompleteStructureReadOnly|TestApprovalRoleSchemaPreservesFrozenBaselineAndRecoversUpgrade|TestTableApprovalSchemaRecoversUpgradeWithoutChangingExistingFacts|TestSchemaMigrationInitializesEmptyDatabase|TestSchemaMigrationPartialDDLRequiresExplicitRecovery|TestSchemaMigrationUpgradesToNextRelease|TestSchemaMigrationCommittedVersionNeedsConfirmedRecovery)$' -count=1 -v
```

命令输出重定向到 `03-integrated-schema-acceptance.log`，shell 的实际退出码记录在同名 `.exit`。

## 合并后的保证

正式版本 1～7 的 14 个 SQL/manifest 文件与外部交付逐字节相同，见 `published-1-through-7.sha256.json`。候选 8、9 的 SQL 保持不变，只从真实数据库重建其完整累计清单。原候选版本仅用于一次性隔离库；这些旧隔离库的账本摘要不作为新前缀的升级或恢复输入。

历史接管仍固定版本 5，最新版本测试助手采用外部的动态迁移版本读取。历史 baseline 前后使用完整数据快照；显式升级允许新增模板配置表及表规则控制版本，仍逐项比较所有旧表规则字段。对于审批迁移新增的空表，仅消除空表快照标题，任何意外写入的行仍会导致比较失败。原账号、会话、密码、审计、规则与业务值的验证保留。

外部角色迁移测试固定在 6，表审批迁移恢复测试固定在 7；完成各自历史恢复及新装结构比较后，再显式升级到当前版本检查 Admin。没有把历史故障断言改为任意最新版本而丢失旧边界。

新增公共验收用例从正式版本 7 升级，保留角色、成员、永久引用、表分配、两类原请求结果、账号、表规则审计和业务数据，并与当前新装逐表比较实际结构。使用只有 SELECT 权限的身份验证六张审批控制表的缺失会阻止启动、使已运行进程返回未就绪，而且不写入数据。恢复后就绪重新通过。

其余用例覆盖模板默认关联与选择保留、合法停用/删除的标准默认模板不恢复、自定义应急节点完整性、部分 DDL 和种子失败恢复、已提交未确认的版本恢复、当前到下一测试版本以及完整只读就绪故障矩阵。

`03-after-run-admin-inputs.sha256.json` 记录本次通过后立即捕获的 Admin 构建与测试输入，供后续改动判断证据有效性。旧 T1/T2 与外部交付的历史证据保持原样，其文件路径及摘要对应各自原提交；合并后的结构由本目录的新证据证明。
