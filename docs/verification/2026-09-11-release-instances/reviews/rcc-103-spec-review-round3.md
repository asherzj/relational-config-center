# #103 Spec 第三轮增量复核

**0 当前发现；上轮 P2 已关闭。源码与行为证据通过，最终交付清单仍待收口。** 保留 `/tmp/rcc-103-spec-review-final.md` 为原 P2 历史，不覆盖。只读检查、未重跑测试、未读 Standards。工作树 `/private/tmp/rcc-issue-103-release-instances`；固定基点 `f7b3f7bbdb42665b67a590b1b182a86f5ecf3d32`，沿用原规格和本票六个 AC 范围。

**P2 关闭依据：** `admin/internal/application/quick_rollback.go:140` 在成功回滚及事件生成后、保存主单与原请求结果前调用 `advanceReleaseFlows`。它保留原实例和已完成审批/发布，仅将未完成正向节点 STOPPED，不虚构完结操作者/时间，不新增 #105 回滚模板实例。`release_quick_rollback_integration_test.go:77–86,109–111` 从真实已发布实例验证原身份、前两节点完整事实、STOPPED、GET 持久化及原键精确重放。19 红→20 三项绿，另包含终态竞争和持久化失败；21 对受影响的分页、混合多表与历史进程路径再次通过。

`web/e2e/release-rollbacks.cjs:263–268` 在真实快速回滚后核对服务器状态和正式页面：已终止、没有“进行中”或当前步骤。22 的该子测试通过；已实际查看移动截图 `browser/legacy-02/release-rollbacks.cjs/release-quick-rollback-result-mobile.png`，新的逐表节点区与终态一致。原回滚总览的 #106 退出责任仍明确，未把未来切片缺失当成本票缺陷。

新浏览器夹具只为参与发布的表显式绑定 STANDARD，保持新表运行时仅默认应急的 AC007 契约。空草稿改验“无提交入口”符合服务器 `allowed_actions`，25项分页、目标占用和竞争断言保留。22 五个子路径 PASS，draft-targets 原失败保留，23 独跑通过；release-approvals 的后续变化仅修正检查组描述。24 已实际 PASS，37.38s/package38.561s，七项表审批检查及空错误数组齐。

`backend/effective-final-http-results.json` 的29个唯一用例逐项对应原 Go JSON PASS，未重复计数或将15/22失败批次改记全绿。20输入236项全匹配；23浏览器输入432项全匹配。六 AC 的正向run02、Web148、TS/build证据维持有效；新生产改动只涉及已补验的回滚分支。领域/API/ADR文字与最小修复一致。

**仅待 artifact：** 当前 `source-sha256.json` 的623项仍有两项旧值（`web/e2e/draft-targets.cjs`、`release-approvals.cjs`）；需刷新最终清单，并在总索引补齐29项及22/23/24有效来源。原运行快照应保留。#103/#106临时分支交接按既定关闭流程登记，无须再跑业务测试。

冻结43输入全部匹配：`final-review-source-v3-sha256.json` SHA-256 `c7fed6a4b7ea63bb9823ebfa7edb4c82e9a644347b7ed323bd0e63c8dd42f651`。QuickRollback SHA-256 `baeb85ff6d38c3b6590dc02a49f87a7296f29d86efbdb18f960eb92caa3fba24`；29项有效映射 SHA-256 `993998551ebf18792dc47737107fd2f3da17ef144f38e38443b4bd6d1d01b530`。本结论限上述输入，不宣称 #104/#105 或完整 #100 已交付。
