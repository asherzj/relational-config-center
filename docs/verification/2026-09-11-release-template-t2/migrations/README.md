# #102 表与发布模板关联：迁移验证

日期：2026-09-11。工作树 `/private/tmp/rcc-issue-102-table-release-templates`，固定基点 `8b79faa3b720f7e2a6837aa43ba7952b0ee76028`。Go 1.27.0 darwin/arm64、MySQL 8.4、Colima Docker 29.5.2、testcontainers-go v0.44.0。所有数据库都在逐个创建并销毁的隔离测试容器内。

## 有效结果

11 个顶层用例及其全部子用例通过。`effective-passing-coverage.json` 将每个用例映射到有效日志；`runs.json` 保留全部原始成功、失败及真实退出码，没有把失败运行改记为成功。

| 日志 | 结果 | 验证范围 |
| --- | --- | --- |
| `07-association-schema-full.log` | 4 个顶层用例通过，52.919s | 正式版本 5 → 候选 9；已选关联及旧配置、审计和业务数据保留；新装与升级结构一致；只读就绪拒绝缺失关联及自定义应急模板的错误节点；标准默认模板合法禁用/删除后不恢复；部分 DDL 后显式恢复不覆盖选择 |
| `09-prior-callers-green.log` | 3 个顶层用例通过，45.053s | 历史基线仍只接管版本 1–5；显式升级保留旧字段和账号、会话、密码、业务数据；升级后结构与新装相同；全组只读启动/就绪故障矩阵及恢复 |
| `10-current-next-recovery.log` | 4 个顶层用例通过，46.144s | 空库初始化、部分 DDL 显式恢复、当前版本到下一测试版本、版本已提交但确认失败时的显式恢复与数据保留 |

运行环境变量：

```sh
TESTCONTAINERS_RYUK_DISABLED=true
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
GOCACHE=/private/tmp/rcc-go-cache
```

以下命令从工作树根目录运行，并带上上述环境变量。第一条是运行 07 的等价复现选择；实际用例以日志的 `=== RUN` 为准。后两条是运行 09、10 的实际测试命令。原输出重定向到对应 `.log`，shell 退出码记录在同名 `.exit`。

```sh
go test -C admin -tags=integration ./cmd/admin -run '^TestTableReleaseSchema' -count=1 -v
go test -C admin -tags=integration ./cmd/admin -run '^(TestSchemaBaselineAdoptsCurrentDatabaseWithoutReplayingHistory|TestAccountUpgradeFromLegacyMatchesFreshSchema|TestSchemaReadinessContinuouslyChecksStateAndCompleteStructureReadOnly)$' -count=1 -v
go test -C admin -tags=integration ./cmd/admin -run '^(TestSchemaMigrationInitializesEmptyDatabase|TestSchemaMigrationPartialDDLRequiresExplicitRecovery|TestSchemaMigrationUpgradesToNextRelease|TestSchemaMigrationCommittedVersionNeedsConfirmedRecovery)$' -count=1 -v
```

## 原始失败与修复

- 01：正式升级后关联表不存在，真实 MySQL 返回 1146。候选迁移及实际生成的 manifest 完成后，03 通过（11.945s）。
- 04：缺少必需应急关联时，只读就绪错误地返回 200；补充只读完整性检查后，05 返回 503 且数据不变（11.065s）。
- 06：自定义应急模板存在错误角色、错误节点类型、重复节点 code 时，仅检查节点数量的就绪逻辑错误地放行。生产就绪改用领域层共享节点校验器后，完整 07 通过。
- 08：三个既有调用者仍假定当前版本为 8：历史结构与当前新装直接比较、把新增表和控制版本误判为历史数据变化、索引故障恢复未重建当前准确的索引次序和 CHECK 字面量。修正测试边界后，完整 09 通过。生产结构校验没有放宽。

## 结构与证据边界

候选 9 仅新增第二张配置表、表规则控制版本及模板复合身份索引，并补齐缺失的有效默认关联。`node_list` 仍在模板定义中保存多个有序节点。应急关联由复合外键保证模板类型一致，CHECK 保证不可禁用，应用事务保证新表只配置应急默认项。

`00009_schema.json` 来自真实 MySQL：临时生成器先通过正式迁移命令应用固定版本 8，再在隔离库应用候选 9 SQL，逐表读取 `SHOW CREATE TABLE`，仅移除运行时 AUTO_INCREMENT 计数器。生成器源码归档为 `manifest-generator.go.txt`，生成运行 02 通过（12.098s）；临时 Go 测试已从源码目录删除。候选 9 的 ALTER 使模板 ASCII 列 CHECK 字面量由 MySQL 规范化为 `_ascii`，manifest 保留真实结果。

历史 baseline 固定为 5。历史基线前后使用完整数据快照；显式升级时比较所有原表数据及表规则每个旧字段，允许新增配置表和 `version` 列。原会话、密码、规则语义、业务原值与版本检查保留。故障测试在恢复唯一索引时使用临时覆盖索引维持外键，再恢复原索引顺序和当前 CHECK 定义，并断言完整 `SHOW CREATE TABLE` 前后一致。

`07-08-schema-inputs.sha256.json` 是新用例通过、旧调用者修复前的输入快照；`final-schema-inputs.sha256.json` 记录最终 9 个相关输入。期间仅三个旧调用者测试文件变化，均在 09 全组重跑；新迁移、manifest、就绪生产代码、领域节点校验与新用例未变。`final-owned.diff` 保存本子任务涉及文件相对固定基点的完整改动。整体应用和 Web 的验证由本票主实现证据索引记录。

版本 1–5 和已交付候选 8 的 12 个 SQL/manifest 文件均与固定基点逐字节相同，记录在 `prior-migrations-unchanged.sha256.json`。当前分支尚未接入外部 #94 的迁移 6、7；#103 接入已验证外部提交时，须保留外部迁移并从真实 MySQL 重建本任务未部署候选的累计 manifest。本次没有部署、改写历史账本或修改业务库。

原始日志与 diff 保留原始空白；源代码的空白检查不以改写证据文件为手段。
