# #94 可重复验证命令

工作区 `/private/tmp/rcc-issue-94-table-approval`。下列命令在 `admin/` 运行；`DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`TESTCONTAINERS_RYUK_DISABLED=true` 是本机 Colima 的环境选择。每个运行等待前一任务自有 MySQL 清理后才开始，不改真实业务竞争屏障。

```sh
go test -p 1 ./internal/application ./internal/domain ./internal/interfaces/http ./internal/infrastructure/mysql ./cmd/admin

go test -p 1 -tags=integration ./cmd/admin -run 'TestTableApproval|TestReleaseSubmitRequiresIndependentApprover|TestPublicationPublisherHistoryAndApprovalSurvivesRevocation|TestReleaseReprepare|TestReleasePeopleResolveCurrentNamesWithoutAccountAdmin|TestReleaseWorkflowAtomicityAndCompetition' -count=1 -v

go test -p 1 -tags=integration ./cmd/admin -run '^(TestApprovalRoleSchemaPreservesFrozenBaselineAndRecoversUpgrade|TestSchemaReadinessContinuouslyChecksStateAndCompleteStructureReadOnly|TestSchemaMigrationUpgradesToNextRelease|TestSchemaMigrationCommittedVersionNeedsConfirmedRecovery)$' -count=1 -timeout=12m -v

go test -p 1 -tags=integration ./internal/infrastructure/mysql -run 'TestPublicationJSONIdentity|TestExplicitIdentity|TestPublicationRejectsIdentity' -count=1 -timeout=10m -v

RCC_E2E_OUTPUT=/path/to/new-evidence go test -p 1 -tags='integration browser' ./cmd/admin -run '^TestTableApprovalBrowserSystemPath$' -count=1 -timeout=10m -v
```

Web 命令在 `web/` 运行：

```sh
pnpm test:run src/features/release-orders/ReleaseOrdersPage.test.tsx src/features/table-policies/TablePoliciesPage.test.tsx src/api/release-orders.test.ts src/features/release-orders/release-journal.test.ts src/features/account-roles/AccountRolesPage.test.tsx --maxWorkers=2
pnpm typecheck
pnpm build
```

其余 Web 的原始命令及真实 red/green 结果保留在 `web/*.txt`；`web/reconciliation.md` 逐项说明真实全量失败及仅受影响的复验，不把原全量结果替换为局部通过。

受影响旧浏览器首次在仓库根运行：

```sh
RCC_E2E_SUITE=approval-contract-regression RCC_E2E_ENGINES=chromium RCC_E2E_ARTIFACTS=/path/to/new-evidence make test-browser-acceptance
```

前6脚本通过后，修复第7个脚本的跨查看者比较，剩余5脚本使用原Go系统入口精确选取（先创建输出目录；accounts.mjs是根步骤）：

```sh
RCC_E2E_OUTPUT=/path/to/existing-evidence-dir go test -p 1 -tags='integration browser' ./cmd/admin -run '^TestAccountBrowserSystemPath$/(release-rollbacks|release-multitable|release-rollback-reason|field-display)\.cjs$' -count=1 -timeout=15m -v
```

本张7组最终运行同前述 TestTableApprovalBrowserSystemPath 命令；证据在 browser-review-final.txt，其中另一根测试的输出目录环境失败不能记为全绿。
