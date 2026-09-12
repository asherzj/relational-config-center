# 依赖合并 Standards 首轮

结论：本轮代码发现 **0**，主观坏味道 **0**；源码规范轴通过，整体验收仍待后述证据。未运行测试、修改生产代码或读取 Spec 报告。

范围：`/private/tmp/rcc-release-template-approval-integration`，ours `7eb9dd36affe16739d8e8b178afdd9977d4976a3`，theirs `2612bb354ef71d42e193648564b9f30bd9281eb4`；仅审 scope 中 23 个双方修改文件与 7 个集成路径。逐块对照两父提交，复用 #102、#93/#94 已交付 Standards 结论，不要求 #103 实例或 #104 应急执行。

- **装配与权限**：双方服务、路由、控制表保护均保留；审批仍是 VIEWER 粗粒度入口、服务端当前逐表资格判定。表规则版本与 Idempotency-Key 路径保留；无新分层违规。依据 ADR-0012/0013、ADR-0026。
- **操作恢复**：表规则四入口与独立抽屉并存；成功反馈、慢读取/写入互斥、原请求重推及审批范围确认没有被合并覆盖。10 个关键实现已独立核对与相应父提交逐字节相同，符合 `web/DESIGN.md:97–98`。
- **迁移与文档**：正式 1–7 的 14 文件逐字节保留；候选 SQL 8/9 不变，实际 MySQL 累计 manifest 为 27/28 表。模板原定义及六张审批表定义保留；9 的表规则版本变化等同 ours。baseline 固定 5、历史 6/7 恢复边界独立，新增故障测试未弱化 readiness。模板 ADR 仅改编号为 0027，活动引用、领域词汇与设计文档一致。依据 ADR-0024、`docs/agents/domain.md`、`docs/schema-migrations.md`。

证据：已核对 schema 真实红例→14 项 PASS/exit0（197.021s）、224 个 Admin 输入 hash；Web 有效 188 项、TS/build、61 个源码及 21 个证据 hash；后端三个包单元通过。HTTP 19 项首次因 Docker provider 不可用失败，重跑进行中；真实浏览器与最终总索引待核验，不将这些待办报成代码缺陷。

实际阅读的 30 文件首尾 hash 全部一致，完整快照：[source-sha256](/tmp/rcc-template-approval-integration-standards-round1-source-sha256.json)，快照 SHA256 `94ee87bd015efb123c9e2d94e5735b47035ae3e7035f83b6d557d134717773a8`。

主观基线全部核对，均无新增可行动项：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库规则优先；工具已强制项跳过。
