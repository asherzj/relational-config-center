# #103 Spec 最终增量复核

**1 项 P2，暂不放行。** 独立 Spec 轴，只读，未运行测试、未读取 Standards。固定基点 `f7b3f7bbdb42665b67a590b1b182a86f5ecf3d32`；工作树 `/private/tmp/rcc-issue-103-release-instances`，仍未提交。沿用 round1 已完整读取的 #100/#103、AC009/010/011/013/014/022 与领域/ADR/DESIGN；仅检查后续改动和证据。

## 当前发现

**[P2] 已回滚单仍把无法继续的完结节点显示为进行中。** `web/src/features/release-orders/ReleaseOrdersPage.tsx:93` 无条件渲染新 `ReleaseFlows`；`ReleaseFlows.tsx:25–27` 将 `ACTIVE` 显示为“进行中”及当前步骤。既有 `admin/internal/application/quick_rollback.go:135–140` 保存 `ROLLED_BACK` 时没有推进正向节点，因此此前 `SUCCEEDED` 的完结节点仍为 ACTIVE。页面同时显示已回滚与正在完结。这是 #103 新增展示对既有终态的回归，不是要求提前 #105 回滚模板；依据 #103“用户看到各表持久节点实例”、父规格“整单阶段显示一致的业务含义”和临时分支仅保留原回滚呈现的边界。当前文档声明“尚未推进”不能消除用户看到的矛盾。最小修正是在未接入阶段不渲染回滚单的活动节点区，保留原回滚总览和真实结果；或在既有回滚事务准确终止未完成正向节点。补回滚后公开响应/页面断言，不能只构造理想 STOPPED fixture。

## 已核验与剩余

Create/copy/replay 锁后当前权限、RR 联表快照、仅补缺实例、审批冻结分离、派生原子边界没有新增问题。两个回归夹具修正保留真实授权锁等待、409 新目标所有者及原子转移断言。保存成功提示由已确认写结果触发，后续详情 503 单独显示；相应组件测试验证两者同时存在。

真实 HTTP 有效 26 项=运行15的24 PASS＋运行18的2 PASS；原失败保留。236 输入对比确认仅两份已复测测试及领域文档变化，生产输入一致。故障14、进程17通过。Web 新增反馈后101项通过，47项沿用未受影响证据，TS/build exit0。浏览器20已实际 PASS（31.52s/package33.407s），四组检查及真实快照覆盖两表独立节点、配置隔离、明确补缺、原键原正文恢复、审批进度、390px和键盘来源展开。

六个负责 AC 的主证据可追溯；#104/#105 未接入不计缺陷。新增领域/ADR/API文档已读。剩余交付整理为总AC/证据索引、临时回滚分支在 #103/#106 的登记，以及上述 P2 修正后的针对性证据；不需要全仓重跑。

复核输入：`docs/verification/2026-09-11-release-instances/review-source-sha256.json` 34项全部匹配，SHA-256 `64b6c8e96b5847b6dc21a763de8f5a80c08fa6c8dcf5a1052c3255f9fde3d023`。浏览器20的432输入全部匹配，清单 SHA-256 `91df2826daa7855e59f3a3f621290d171c6855814c3c85efb7fe640599dc4d78`。

关键文件：ReleaseOrdersPage.tsx `708754a0f7e2ec851cba9d65d776e9988f53ed3880e3f96e98ee7ec672fe280b`；ReleaseFlows.tsx `2c58346559217fa25f6e0c028d9869298e8c42657daabaffdacc7fe30c791634`；ReleaseDraftEditor.tsx `7cc996261161312d233402bb395616d0b6b4787c385333ac4639f80a8dda1feb`。
