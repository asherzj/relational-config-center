# 可复现命令与环境

工作目录为本票 worktree。Go MySQL / browser 命令显式设置：

```sh
export DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true
```

每个 runner 按顺序创建并终止自己的 MySQL 8.4，禁止测试并行；业务竞争仍使用同一个 fixture 的真实并发连接。MySQL、迁移和正式浏览器之间串行运行。依赖不可用按失败处理，不 skip。

核心与结构/工具原始批次：

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^(TestApprovalNotifications.*|TestReleaseReset.*)$' -count=1 -timeout=10m -v
```

该原始批次 core-schema-reset.txt 含夹具失败；具体版本、有效项和修复关系见 README。故障修复复验：

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^(TestApprovalNotificationsSchemaUpgradeReadinessAndProtection|TestReleaseResetRefusesUnverifiedTargetsAndSchema)$' -count=1 -timeout=5m -v
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^(TestApprovalNotificationsSchemaUpgradeReadinessAndProtection|TestReleaseResetPreservesRecordsAndContinuesPublication)$' -count=1 -timeout=5m -v
```

受影响调用方：将 regression-selection.txt 的 18 个名字用 `|` 连接并加 `^(...)$` 作为 `-run`，其余参数为 `go -C admin test -p 1 -tags=integration ./cmd/admin -count=1 -timeout=10m -v`。保留原17PASS/1FAIL，锁观察夹具修复后仅：

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestMultitableReprepareTransfersChangedTargetsAtomically$' -count=1 -timeout=3m -v
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestApprovalNotificationsConcurrentLateReadPreservesNewEvents$' -count=1 -timeout=3m -v
go -C admin test -p 1 ./internal/domain ./internal/application ./internal/infrastructure/mysql ./internal/interfaces/http ./cmd/admin ./cmd/account-maintain -count=1
```

正式浏览器分别设置不同 RCC_E2E_OUTPUT 路径，并顺序执行：

```sh
go -C admin test -p 1 -tags='integration browser' ./cmd/admin -run '^TestNotificationCenterBrowserSystemPath$' -count=1 -timeout=5m -v
go -C admin test -p 1 -tags='integration browser' ./cmd/admin -run '^TestApprovalNotificationsBrowserSystemPath$' -count=1 -timeout=5m -v
```

Web 在 web/ 下运行 `pnpm test:run`，最终影响切片为 `pnpm exec vitest run src/features/notifications/ApprovalNotifications.test.tsx src/features/notifications/NotificationsPage.test.tsx src/api/release-orders.test.ts`；真实 TCP 单项用 `pnpm exec vitest run src/api/client-stream.test.ts` 在允许本机临时监听的环境补跑；`pnpm build` 含 `tsc --noEmit`。精确脚本输出和失败记录见 web/ 全部原始 .log。
