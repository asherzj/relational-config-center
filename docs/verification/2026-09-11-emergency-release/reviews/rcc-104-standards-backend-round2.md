# #104 Standards 后端第二轮增量

**首轮 P3 已关闭；新增成文规范 0 发现、主观坏味道 0 发现。** 仅放行已审后端增量，不代表整票完成。保留 round1 原观察，不读取 Spec、不运行测试或修改源码。

工作树 `/private/tmp/rcc-issue-104-emergency-release`，固定基点 `e2564842e72e96b6ffb1e86708978b11dd8a651b`。范围为 v2 清单相对首轮的输入拆分、新增负例、两处旧夹具、浏览器 harness 和五份领域/接口文档；其他集成工作树不在范围。

- `admin/internal/application/release_orders.go:659` 的提交专用 `SubmitReleaseOrderInput` 承载原因，`:836` 历史归因同步采用该类型；`admin/internal/interfaces/http/release_orders.go:181` 仅提交解析它。发布、完结、回滚预览继续使用仅版本输入，原因不再扩大其字段或摘要，原 P3 职责耦合消除。新增公开 HTTP 负例 `admin/cmd/admin/release_emergency_integration_test.go:420` 验证三动作均拒绝原因字段。
- `release_failure_retry_integration_test.go:114` 在真实认证后的传输边界暂缓正文，再确认取消连接等待发布连接的授权锁；保留取消状态/版本、失败历史及零业务副作用断言。`release_original_execution_integration_test.go:58` 只按当前详情比较 ApprovalContext，原申请、节点与执行结果仍完整比较。符合 ADR-0025/0026 及审批契约对原结果与当前资格的区分，未削弱验收。
- 领域词汇、ADR-0027、草稿/审批/执行契约准确描述应急待发布、原因、原请求与整单生命周期；未新增回滚实例或下游已送达承诺，符合 domain/design 文档分工。

完整主观基线继续适用：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库优先、工具强制项跳过。

证据：14 原三失败/exit1 保留；15 六项真实 PASS（包 60.449s），16 仅编译通过，17 常规提交与应急原请求两项 PASS。独立核对运行输入；15/16 后变动仅未参与该批次的 browser harness/Web 测试，17 后仅 Web 测试变动，复用合理。

快照：v2 清单 SHA256 `6521acf17718e3f06ef7883fadff2493800bf7f4bfc5ca828f9658dc42642db6`；15/16 文件 MATCH。harness 后增 `RCC_E2E_ISOLATED=1` 已只读复核，实际 SHA256 `254875220f8d825f3140b072faad22be0ccbba0045c04cd832a934482107cc04`。完整实际输入：`/tmp/rcc-104-standards-backend-source-round2.json`；原始证据哈希：`/tmp/rcc-104-standards-backend-evidence-round2.json`。

待最终增量：Web 源码与 DESIGN、真实浏览器、最终有效回归和证据索引；不凭在建材料提前放行。
