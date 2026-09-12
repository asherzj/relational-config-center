# 验证命令

工作目录为隔离工作树的 admin/ 或 web/。所有真实数据库命令使用 `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true`。以下为集成后的实际最终命令；红灯测试用例保持同名，原结果与环境错误日志分别保留。没有运行完整全仓集成套件。

```sh
# admin/，25项最终结果来自该23PASS/1fixture FAIL及后两次定向运行
# mysql-integrated.txt
env DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./cmd/admin -run 'TestApprovalRole|TestLastEnabledAdministratorCannotBeRemovedOrDisabled|TestRoleChangesAndAccountMaintenanceAreConcurrentSafe|TestRoleChangeRaceAndAuditFailureDoNotLeavePartialGrants|TestReleaseApprovalCurrentRolesAndHistory|TestAccountControlTablesCannotBeDiscoveredOrManaged|TestSchemaBaseline|TestFrozenGooseInstallationMatchesFrozenAdoptionStructure|TestSchemaReadiness' -count=1 -v
# schema-integration-targeted.txt：角色迁移PASS，既有v5测试因latest漂移FAIL
# schema-v5-pinned-green.txt：固定历史构建后单项PASS
env DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./cmd/admin -run 'TestApprovalRoleSchemaPreservesFrozenBaselineAndRecoversUpgrade|TestReleaseSchemaStagesRecoverWithoutRewritingLegacyBusinessFacts' -count=1 -v
env DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./cmd/admin -run TestReleaseSchemaStagesRecoverWithoutRewritingLegacyBusinessFacts -count=1 -v
# browser-final-integrated.txt，12场景真实浏览器
env DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration,browser ./cmd/admin -run TestApprovalRoleBrowserSystemPath -count=1 -v
# go-checks-integrated.txt
go test ./internal/application ./internal/domain ./internal/interfaces/http ./internal/infrastructure/mysql ./cmd/admin

# web/，先按集成锁文件补齐fake-indexeddb，不改锁文件
pnpm install --frozen-lockfile --offline
pnpm typecheck
pnpm build
pnpm test:run src/api/client.test.ts src/api/client-stream.test.ts src/features/account-roles/AccountRolesPage.test.tsx src/features/accounts/WorkspaceAccess.test.tsx

# 从compose-source-export.json记录的逐文件校验副本执行正式脚本
python3 scripts/compose-migration-acceptance.py --artifacts /private/tmp/rcc-issue-93-approval-role-management/docs/verification/2026-09-11-approval-roles/compose
```

失败注入仅在外部边界：浏览器拦截网络；MySQL专用触发器/权限模拟请求结果写入和迁移确认失败；真实进程中断；未mock应用自身协作者。截图等待动画与toast完成，并断言实际抽屉几何、整页不溢出、390px表格方向键滚动与右侧操作可见。代码whitespace检查排除保留原始终端输出的验证目录。
