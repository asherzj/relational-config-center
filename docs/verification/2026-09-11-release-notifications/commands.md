# Reproduction commands

Run from this ticket's worktree. All real MySQL/browser commands use:

```sh
export DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true
```

Each test fixture creates and terminates its own MySQL 8.4. Use `-p 1`, no `t.Parallel`, and run these commands sequentially. Actual concurrency cases run competing requests inside one fixture.

Final ticket acceptance (the independently valid earlier logs and repaired test logs are indexed in README and mysql-results.json):

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestReleaseNotifications.*' -count=1 -timeout=5m -v
```

The executed acceptance batches were:

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestReleaseNotifications(PublicationUsesActualParticipants|TerminalResultsPreserveOwnUnread|ReprepareCancelsSourceAtomically)$' -count=1 -timeout=3m -v
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestReleaseNotifications(PersistenceFailuresPreserveAllFacts|LostResponsesRecoverOriginalFacts|ReprepareCancelsSourceAtomically)$' -count=1 -timeout=4m -v
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestReleaseNotifications(CommitUnknownRecoversEveryLifecycle|TerminalCompetitionHasOneResult)$' -count=1 -timeout=4m -v
```

The 20 affected callers are exactly `regression-selection.txt`; the executed argument array is `regression-command.json`. Both failed cases and both tests sharing the reviewed preparation helper were then run with:

```sh
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^(TestReleaseFailureAuditPreservesConcurrentCancellation|TestOriginalOrderRollbackPreservesApplicationAndBothExecutions|TestReleaseNotificationsLostResponsesRecoverOriginalFacts|TestReleaseNotificationsCommitUnknownRecoversEveryLifecycle)$' -count=1 -timeout=4m -v
go -C admin test -p 1 -tags=integration ./cmd/admin -run '^TestReleaseFailureAuditPreservesConcurrentCancellation$' -count=1 -timeout=3m -v
```

Browser output location is an absolute path inside the evidence directory:

```sh
RCC_E2E_OUTPUT="$PWD/docs/verification/2026-09-11-release-notifications/browser/initial" go -C admin test -p 1 -tags='integration browser' ./cmd/admin -run '^TestReleaseNotificationsBrowserSystemPath$' -count=1 -timeout=5m -v
```

This is the explicit runner #98 must include; root's historical acceptance script does not discover the new e2e script automatically.

Additional affected compile/contract/runtime checks:

```sh
go -C admin test -p 1 ./internal/domain ./internal/application ./internal/infrastructure/mysql ./internal/interfaces/http ./cmd/admin -count=1
pnpm --dir web exec vitest run src/api/release-orders.test.ts src/features/notifications/ApprovalNotifications.test.tsx src/features/notifications/NotificationsPage.test.tsx src/features/release-orders/ReleaseOrdersPage.test.tsx src/features/release-orders/release-journal.test.ts
pnpm --dir web typecheck
pnpm --dir web build
node --check web/e2e/release-notifications.cjs
git diff --check
```

The environment must permit loopback listeners for process/browser tests. Installed dependencies were reused through the untracked symlink; the first target lacked fake-indexeddb, so the final target uses #96's already installed dependency tree. Runtime package versions/manifests match in `dependencies.json`; neither package lock nor package definitions changed.
