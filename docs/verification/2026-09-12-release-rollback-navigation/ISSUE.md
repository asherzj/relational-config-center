# Issue #112: 修复回滚浏览器验收中的通知请求页面错误

URL: https://github.com/asherzj/relational-config-center/issues/112

State at investigation: OPEN. Label: `ready-for-agent`. Assignee: `asherzj`. Comments: none.

关联规格 #100、已合并 PR #109 及收尾 PR #111。此工单只处理独立的回滚浏览器验收失败，与 #110 的合成线程 SIGSEGV 分开。

原始失败：CI 34683913900 / Browser job 103527409853，head `a221562036136942ff5cbbeace95a44d13532900`，无 ptrace。执行 16/34：15 PASS、`release-rollbacks.cjs@webkit` exit 1、18 未执行。该用例先完成 7 条业务检查，再于 line 333 `assert.deepEqual(errors, [])` 失败，唯一 page error 为 `/api/v1/approval-notifications due to access control checks.`。原事件无时间戳，尚不能确定在哪个步骤触发，也没有页面崩溃或退出信号证据。

目标：在真实回滚路径建立紧凑反馈，区分真实未处理请求错误与文档切换取消请求造成的验收误报，做有证据支持的最小修复；保留完整业务与页面错误断言。可参考 PR109 已归档的导航取消调查，但不能把其他路径的受控阳性对照直接当成本故障复现。

验收：

- AC-R01：冻结原始失败、准确执行范围及源码版本；为待验证的具体导航/请求时序假设建立真实红反馈或明确边界的诊断证据。
- AC-R02：原完成/快速回滚、真实竞争完成、丢响应同正文同键恢复、审批与权限、真实 SQL 及 7 项原业务检查保持；不得 force click、删 pageerror 断言、扩大超时或盲目 retry。
- AC-R03：修复后受影响三引擎回滚用例完成并且 page error 为零，真实后检与清理通过；必要导航/异常边界证据完整，其他有效证据可复用。
- AC-R04：独立工作树/固定基点、完整快照 Standards/Spec 双轴评审通过，提交由主代理接入 PR111；所有临时探针在本工单内实际删除或仅归档为历史证据。最终必需 CI 有效通过后按既定授权交付及关闭。

执行安排：独立分支 `codex/ci-rollback-navigation`、工作树 `.worktrees/ci-rollback-navigation`、基点 `a306c94f484a5a4796f727bc4a37c192d9538bef`。实现 agent 不提交、推送、合并或关闭；由主代理统一接入 PR #111。父规格 #100 不自动关闭。
