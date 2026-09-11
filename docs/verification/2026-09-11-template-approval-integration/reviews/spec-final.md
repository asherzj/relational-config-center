# 模板与审批依赖合并：Spec 最终增量复核

2026-09-11。**0 findings；Spec 放行本次依赖合并。验收证据缺口 0。** 未运行测试、未修改源码、未读取 Standards。工作树 `/private/tmp/rcc-release-template-approval-integration`，仍未提交；固定 ours `7eb9dd36affe16739d8e8b178afdd9977d4976a3` / theirs `2612bb354ef71d42e193648564b9f30bd9281eb4`。沿用 round1 的 23+7 路径范围与源码结论，仅复核三个新增测试调整及实际证据。

规格依据：#100/#102 AC006–008 的“每表两类唯一关联”“新建仅默认应急”“历史补缺不覆盖”；#93/#94 的当前账号/角色资格、逐表独立审批、默认审批人及范围冲突恢复。已交付 #101/#102 元数据原请求、事务、版本和生命周期语义保留；本次没有接入 #103 流程实例或 #104 应急执行，也没有新增未要求行为或临时层。

- `admin/cmd/admin/business_auth_integration_test.go:543–602` 真实观察撤销等待授权控制锁，随后释放业务锁，继续断言已授权发布成功、原操作者/业务值保存、撤销完成及旧会话新请求 401。调整与 #94 串行资格判定一致，未改为假等待或删除结果断言。
- `admin/cmd/admin/release_history_delivery_integration_test.go:218–251` 将实时 `ApprovalContext` 与历史事实分验：明确检查新 revision、VIEWER 无资格、待审默认 ADMIN 模式；其余保存字段逐项完全相等。拒绝删除/伪造修改后仍比较完整同身份响应。没有以忽略审批历史掩盖持久化差异。
- `web/e2e/table-release-templates.cjs:12,27` 增加真实导航与四入口可见断言；随后完整执行原流程，包括真实提交后断响应、同账号恢复、原键原正文手动重推、竞争输入保留、390px 与键盘操作。

证据核验：逐项读取 Go JSON，19 HTTP/MySQL 有效通过为原批次 17 项＋撤销复测＋历史复测，保留两次 exit1，未冒称单批全绿。Schema 14 项、Web 有效 188 项及 TS/build 延续有效。三个真实浏览器日志均 PASS/exit0，产物 JSON 合计 28 checks；角色、表审批、模板目录及恢复均覆盖。

复用边界：最终 224 个 Admin 输入全部匹配；相对 schema/原批次只变两个已复测测试文件，撤销文件与其通过快照一致。Web 61 输入只变上述浏览器脚本，由最新完整浏览器运行覆盖；19 个 browser 输入全部匹配。round1 的 router、security、TablePoliciesPage 哈希未变。父提交的相同实现证据与本次公开合并路径证据共同支持放行。

复核输入 SHA-256：

```text
business_auth_integration_test.go 8926ea0e0e90dd27de09d532546a7767413691793fc29ce9f9dba45dcc484dc9
release_history_delivery_integration_test.go de6ab79b477da5e01f36a10505738d2e6d06ffcce9549290bb1a7fdb50af3ff2
table-release-templates.cjs a60142244c14cef57cc7c194d674b52e89622e2a770383034ab14940df835299
backend/mysql-final-source-manifest.json 1f379b102c3530c5c50ade98bb2d21c74960f0906f9f0c35b7661159bdd4aa99
browser/browser-inputs.sha256.json 146e0c2bb4ae2adc672b9b0c54e7e6d34facb1eabe818af12c4117931bb619da
schema/03-after-run-admin-inputs.sha256.json 4d821c902cac74c24d350617d410606a5f63ef38bd4c88bd79e5ddb9d174754b
```

证据根目录：`docs/verification/2026-09-11-template-approval-integration/`。整体最终索引、全量清单与提交由 root 收口；本结论限于当前输入，不宣称后续 #103 已完成。
