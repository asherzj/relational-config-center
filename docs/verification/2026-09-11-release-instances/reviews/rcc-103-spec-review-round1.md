# #103 Spec 初轮

**源码 0 新发现；最终验收尚未闭合，暂不放行。** 只读审查，未运行测试、未读取 Standards。工作树 `/private/tmp/rcc-issue-103-release-instances`；固定基点 `f7b3f7bbdb42665b67a590b1b182a86f5ecf3d32`，提交列表为空；审查 `git diff <base> --` 及未跟踪新增源文件。

完整来源：当前 `spec/issue-100.json`、`issue-103.json` 正文/空评论/标签，已发布本地规格和切片，以及 AGENTS、领域词汇、相关 ADR 和 DESIGN。本票仅负责 AC009/010/011/013/014/022，不要求 #104 应急或 #105 回滚通知。

- **AC009–011**：“展示前各表的节点实例已经保存”“再次保存草稿补齐缺失实例，其他已有表实例保持不变”。`admin/internal/application/release_flow.go:12–48` 仅显式写入补缺，并按当前参与表过滤；`release_orders.go:165–176,805–813` 与主单、明细和原结果处于既有事务。GET 不创建流程，停用模板不替换旧实例。
- **AC013/022**：“新单按当前关联重新实例化”“完整旧版或完整新版”。`release_orders.go:978–1007` 派生实例不继承原实例/审批，源单与占用仍同事务；`mysql/release_flows.go:28–38` 用 RR 内联表读取关联及模板，真实读取故障返回 unavailable。并发测试通过真实锁安排交叠，并核对原键重推和不同正文冲突。
- **AC014/B004**：“提交不更换节点实例”“成员资格实时”。`release_orders.go:638–653` 保留提交审批快照；`release_flow.go:53–97` 从实际逐表决定/事件保存进度，没有旧 APPROVER 或通用 ADMIN 旁路。`ReleaseFlows.tsx:19–33` 展示各表自己的节点；固定总览仅剩回滚专用，已在 DESIGN 标明 #106 退出。

已声明在修的 create/copy 授权缺口现已修：`release_orders.go:128–136,892–900` 锁后读取当前账号并检查 EDITOR，先于原请求重放。真实 12 红→13 绿已核对，create/copy/replay 三例通过（11.68s）；不重复计作新发现。

已核对六项 AC 的实际绿日志；Web 有效 147 项及 TS/build 有交付记录。**唯一待核验清单**：存储/原结果失败、派生占用原子性及受影响旧发布路径回归；最终真实浏览器（多表、390px、键盘、原包恢复）；领域/契约文档、引入工单与 #106 临时分支登记；最终源码—证据映射。正在运行的故障日志和未执行浏览器脚本不算通过，不要求每层重复相同事实。

27 个当前变更/新增源文档输入及 SHA-256 记录在 `/tmp/rcc-103-spec-review-round1-inputs.json`，清单 SHA-256 `1073f51f41ec394152dbd676269912580e35e66576221a325d8cb1c0d4c63db9`。关键输入：

```text
application/release_orders.go 0b67f2f5022d41f38f28266f8da26bad1e994bfa2c9437d4c5b3e3abb481eb74
application/release_flow.go a66626d284386ff18e7af2377981879e842e4af654eb601349a85d3d98abef56
mysql/release_flows.go c196109b02045d02ab66a585b1a06db6a849e8d83513afb6b9448a9927a114b6
ReleaseFlows.tsx 2c58346559217fa25f6e0c028d9869298e8c42657daabaffdacc7fe30c791634
```
