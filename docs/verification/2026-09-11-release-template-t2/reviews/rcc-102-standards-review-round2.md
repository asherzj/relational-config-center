# #102 Standards 增量复审（第 2 轮）

固定基点 `8b79faa3b720f7e2a6837aa43ba7952b0ee76028`；工作树 `/private/tmp/rcc-issue-102-table-release-templates`。独立规范轴，只读源代码及证据，未运行测试，未读取 Spec 报告。

**原 2 项已清除**：关联抽屉 40 行使用 Sonner；表规则抽屉 78–83 行仅在刷新成功后更新版本并清除错误，失败保留输入、版本和错误，启停也有恢复入口。已核对相关组件回归，70 项及 build 原始日志均成功。

**新增 P2，1 项**：[TablePolicyDrawer.tsx:78](/private/tmp/rcc-issue-102-table-release-templates/web/src/features/table-policies/TablePolicyDrawer.tsx:78) 的恢复读取没有与写入共享忙碌保护。触发：版本冲突后开始慢刷新，再点击底部启停/保存；刷新先返回时，81 行会 reset 正在提交的 mutation。当前 TanStack 5.102.3 的 reset 移除 observer，使 148–155 行的成功、失败和 settled 回调不再执行，导致结果反馈/原请求保存丢失且 inFlight 留 true。违反 [DESIGN.md:76](/private/tmp/rcc-issue-102-table-release-templates/web/DESIGN.md:76) 的原请求保护，以及 [DESIGN.md:93](/private/tmp/rcc-issue-102-table-release-templates/web/DESIGN.md:93)、97 的真实操作状态和写入保护。修正：恢复读取与写入互斥；读取中禁用写动作；禁止 reset 活跃写入。补慢刷新与写入交错的回归。

缓存与持久原结果分离、未知结果后确定版本冲突解锁的新增路径未发现其他规范问题。迁移 11 个顶层成功日志已核对，9 个最终输入 hash MATCH；旧 12 个 SQL/manifest 与基点及证据逐字节一致。

完整主观基线已检查：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest；无额外主观发现，跳过工具强制项。

本轮 SHA-256：

- TableReleaseTemplatesDrawer.tsx：`ab75832189717f5d8f9a4a6d970dfa588088110b7dfa3dd57b3646a397959703`
- TablePolicyDrawer.tsx：`0da4bfb2008138130cfe855d555e03dbbf7c0b6bdb4fa26e849fbbdb8b97846e`
- queries.ts：`b03c2f4432e32ad2a391b3587333cac755d0491f05b0619bcae1525515907ad5`

待最终核验：本轮竞态修复、最终浏览器及 6 项 T2 + 17 项 T1 HTTP 结果、最终源码/证据索引；不宣称这些在运行项目已通过。
