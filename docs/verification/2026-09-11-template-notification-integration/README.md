# 发布模板与通知依赖集成：进行中

此目录记录独立集成工作树的实际验证，当前不是完成交付。根提交为 `e2564842e72e96b6ffb1e86708978b11dd8a651b`（#103），当前中间合并接入 `7334e1103f612b4735992ef63bf1a0b7643b40ae`（#97，包含 #95/#96）。#104 仍在另一工作树实现；其最终提交后还需接入本分支，并按影响复验。此 merge 将保存为未推送的本地中间检查点，继续接入后续正式依赖；未开始 #105。#98 已正式交付并合入 main（`eb55e841e80345cfef7733b8c560d6037256cd2c`），本目录当前记录的是接入 #98 前的局部检查点；完成此 merge 后继续正常合并该实际发布提交，模板候选将顺排为 10/11。

## 已完成的局部验证

- `schema/01-notification-readiness-red.log` 是真实失败：暂用旧模板累计清单时，缺失 `rcc_approval_notifications` 后 Admin 仍提供服务。原 exit 1 保留。
- `schema/02-generate-manifests.log` 在新建隔离 MySQL 上应用正式版本 1～8，随后逐步执行尚未部署的模板候选 9/10，并逐表读取 `SHOW CREATE TABLE`。真实累计清单分别为 28/29 张表，包含通知表。只剔除运行时 AUTO_INCREMENT 计数；未拼接 schema JSON 或修改已使用的账本。
- `schema/03-notification-readiness-green.log` 对同一缺表用例 PASS，证明缺失时拒绝、恢复后就绪，并保持只读。临时生成器归档为 `schema/manifest-generator.go.txt`，已从 Go 源码目录删除。后续新装、升级、恢复及完整受影响 schema 矩阵仍待运行。
- `schema/published-1-through-8-sha256.json` 的 16 个正式 SQL/manifest 与 #97 逐字节相同。候选模板 SQL 只从 8/9 顺排至 9/10，内容不改。历史验证目录不回写。
- `web/01-affected.log` 原始 10 文件、175 测试为 166 PASS/9 FAIL，exit 1。失败集中在两份通知测试旧响应夹具缺少新增流程 DTO 字段；补齐 `release_type/table_flows/missing_flow_tables` 后，`web/02-notification-fixtures-recheck.log` 两文件 13 项 PASS。其他八文件 162 项依赖未变，有效合计 175，不重复计算原批的四项已通过通知测试。生产逻辑没有为这些测试放宽响应校验。
- `backend/01-contracts-and-process.log` 记录 domain/application/MySQL/HTTP/cmd 的五包实际检查通过；早期 integration 编译检查（`-run '^$'`）不计行为覆盖。`backend/02-http-integration.jsonl` 对 ReleaseFlow、NotificationCenter、ReleaseNotifications 三组实际运行 20 个唯一顶层测试，全部 PASS，包耗时 210.115s、exit 0；覆盖实例、个人通知只读视图、结果通知、失败、并发及原请求恢复。`backend/02-http-results.json` 从原始日志逐条核对 20 创建容器与 20 终止容器。
- `web/03-typecheck.log` 与 `web/04-build.log` 均 exit 0。

## 合并语义与待办

保留通知序号、只读一致快照、详情已读确认与导航；保留模板实例、缺配置明确保存和实际节点事实。普通列表及通知列表重建审批环境时携带保存的流程来源；回滚在同一事务既终止未完成正向节点，也记录真实历史参与人通知。原请求结果仍不承载当前个人通知。两处竞争/历史读取测试保留双方已验证的断言。

尚需：接入 #104 最终提交，验证应急状态与通知读取交互；完整必要后端集成回归、当前版本的 Web/浏览器相交路径、两轴独立评审、全部源码/证据 hash、历史 artifact 比对、精确 index 检查与提交。#105 的新通知语义和模板回滚由其独立工单实现。本目录不能用于关闭 #104/#105/#106。

## 当前检查点评审

独立 Standards 对原 12 处冲突及新增对接检查为成文 0、主观 0，报告与实际输入哈希原样归档于 `reviews/`。此结论只覆盖 #103 + #97 中间合并，不是最终含 #98/#104 的集成验收。独立 Spec 对同一有界合并范围确认 0 项确定缺陷、无此范围合并阻断；原报告及实际输入原样归档。最终版本验证仍待。
