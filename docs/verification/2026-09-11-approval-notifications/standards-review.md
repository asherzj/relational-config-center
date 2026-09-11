# Standards 独立审查

固定点：`836c95da4100d06a102dc6cd0266035cafe18ceb`。实际审查命令：`git diff 836c95da4100d06a102dc6cd0266035cafe18ceb --`；覆盖完整待交付工作树及新增的重新准备锁等待观测调整。提交列表与三点差异为空，原因见 `review-input.md`。

依据：`AGENTS.md`、`docs/agents/domain.md`、`docs/agents/design.md`、`CONTEXT-MAP.md`、`admin/CONTEXT.md`、`web/DESIGN.md`、ADR-0001/0002/0012/0026，以及 code-review 的完整坏味道基线。工具已强制的规则跳过。

## 成文规范

**0 条硬性违规。** 通知与业务/资格维护共用事务和授权串行顺序；个人读取投影不进入原业务请求结果；新增结构仅使用第八版迁移。清理流程的九表扩展保留严格清单校验，唯一允许的通知→账号非级联外键未放宽未知副作用拒绝。领域文档保留业务含义，共享交互变化同步到 `web/DESIGN.md`。未发现新增的架构依赖、职责或跨账号共享状态风险。

## 主观坏味道

- **[P3] 可能是 Data Clumps（数据泥团）** — `admin/cmd/admin/approval_notifications_integration_test.go:417`，对应辅助函数及两处调用。`race` 让账号、路径、正文和请求键通过四个平行切片按下标配对，两个调用始终同时提供这些字段；未来追加并发场景时，长度或顺序失配可能使测试使用错误账号与正文。建议局部定义一个带 `actor/method/path/body/key` 的请求类型，改传其切片，直接声明 HTTP 方法。这是非阻断的维护性判断，并非成文规范违反或当前功能缺陷。

Standards：硬性发现 0，主观发现 1，最高 P3。本审查只读源码，未执行测试、Docker、浏览器、commit 或 push。

## 修复复核

上述 P3 已关闭。`admin/cmd/admin/approval_notifications_integration_test.go:418` 新增局部 `concurrentRequest`，将账号、方法、路径、正文与请求键封装为一项；两处并发调用都改为具名字段，HTTP 方法显式给出，cookie/CSRF 在启动各 goroutine 前按该请求账号绑定。原有资格恢复与审批并发场景、通知序号和计数断言保留，未发现新增问题。

已只读核对 `concurrent-receipt-refactor.txt`：`TestApprovalNotificationsConcurrentLateReadPreservesNewEvents` **PASS**，包结果 `ok`；测试由实现方执行。最终 Standards 剩余发现：**硬性 0、主观 0**。
