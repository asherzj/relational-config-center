# Standards review — #97

固定点：`12fda54fc3c2f74e09e7dbde13a6e723d6a98e33`。审查命令为 `git diff 12fda54fc3c2f74e09e7dbde13a6e723d6a98e33`；`git log 12fda54fc3c2f74e09e7dbde13a6e723d6a98e33..HEAD --oneline` 为空。本次还完整读取三个未跟踪的 `release_notifications*.go` 文件及 `web/e2e/release-notifications.cjs`。

首轮 12 个源码文件快照 SHA-256：`b229daf7fd1af565d45b99a8c30758d94bcc6c0d2876f9244a853ee7674a3b99`。P3 修复后相同 12 文件：`d232378f1f6591fababd49bb38697ed6870ceb2394b367fdf9c5effb12ff40e7`。最终纳入 `release_original_execution_integration_test.go`，共 13 源文件：`4c71fce8a26575617cd2d0480d0636aa45942818c8950cf6c5ac3513e0e37f21`。计算方式为路径排序后依次拼接 `path + NUL + file bytes + NUL`。另读通知契约文档，SHA-256 为 `eb843010372a60a6007bb0a79d293278e672bae79378409fc69794b831d856c5`。

## 成文规范

**0 条硬性违反。** 对照 `AGENTS.md`、`docs/agents/*.md`、根与 Admin 领域词汇表、`CONTEXT-MAP.md`、`web/DESIGN.md`、架构文档、ADR-0022/0025/0026 及发布与通知契约核对：

- `approval_notifications.go:12` 使用申请人与真实审批决定中的永久账号身份，并合并同人多表参与，符合历史责任与当前资格分离约定。
- `release_orders.go:801,997`、`quick_rollback.go:147` 在已有业务事务成功变更后、保存原请求结果前接入原通知端口，符合 `docs/admin-notification-center.md` 的事务与去重要求，以及 ADR-0025 的整单原子性。已有存储端继续排除动作本人、保留既有结果未读；未引入新的通知事务或下游投递含义。
- 本次未调整正式页面、共享设计规则或领域概念，无需改写对应基准。工具已强制的检查未重复列作人工发现。

## 主观坏味道与修复

**首轮 1 条 P3，现已修复：可能是 Duplicated Code / Repeated Switches。** 初始快照的 `admin/cmd/admin/release_notifications_failures_integration_test.go:111–128` 与 `197–214` 重复四种生命周期准备分支，容易在操作契约变化后产生测试漂移。此项为主观维护建议，不是规范违反。

增量复核确认 `prepareReleaseNotificationAction` 已共用预先发布、回滚预览、重新准备正文及期望状态码的准备；HTTP 丢响应与 MySQL COMMIT 丢 ACK 保留各自真实注入和恢复断言，原发现关闭。正式账号维护命令停用、重新启用真实审批参与者后的结果读取测试沿用永久身份语义；通知文档新增的参与者、原子性和幂等契约与实现一致。无新增发现。

最终增量核对 `release_failure_retry_integration_test.go` 与 `release_original_execution_integration_test.go`：取消请求在真实认证后由 HTTP body reader 控制进入业务的时机，锁等待查询明确关联发布连接与取消连接的授权锁，符合 ADR-0026 的共享授权顺序；最终取消先于失败历史、申请人一次取消提醒及审批人无成功提醒仍有断言。原发布重放完整比较业务字段，只将资格上下文对照真实当前 GET，另核对持久原请求 JSON 不变，未削弱业务事实与当前资格分离的约定。文档交付边界已同步。无新增发现。

最终摘要：取消竞争的完整对象比较改用同一 `cancellingAdmin` 读取，避免混入其他账号的专属 `ApprovalContext`，所有业务字段比较保留，无新发现。`source-manifest.json` 的 600 项摘要均与当前文件一致；清单 SHA-256 为 `a88f74036367bb5153f08c2d8a0f70f0689d2bf856713faaa2241e788a5adef3`。已读取 `cancellation-notice-verified.log` 的 PASS，未另行运行测试。Standards 当前剩余发现 **0 条**；硬性违反 **0 条**，原 P3 已修复。
