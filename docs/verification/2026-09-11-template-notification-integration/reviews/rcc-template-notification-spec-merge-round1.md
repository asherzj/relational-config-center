# Spec：#103 + #97 中间合并复核

结论：**0 项确定缺陷；此检查点未发现阻止保存中间合并提交的规格问题。** 不表示含 #98/#104 的最终集成完成。

## 固定输入与范围

- 工作树：`/private/tmp/rcc-template-notification-integration`。
- HEAD：`e2564842e72e96b6ffb1e86708978b11dd8a651b`（#103）；MERGE_HEAD：`7334e1103f612b4735992ef63bf1a0b7643b40ae`（#97）；共同基点：`2612bb354ef71d42e193648564b9f30bd9281eb4`。
- 按 `git diff --no-ext-diff HEAD -- <paths>`、`git diff --no-ext-diff MERGE_HEAD -- <paths>` 双向核对工作树。HEAD 侧提交为 e256484、f7b3f7b、7eb9dd3、8b79faa、56dff53；对侧为 7334e11、12fda54、836c95d。12 处 index stages 尚保留，工作文件无冲突标记。
- 完整读取 #103/#97 最新正文、评论及标签（gh 只读获取，均 CLOSED），父规格 #100/#92 的归档全文及评论、相关领域/ADR、发布/通知契约与 Web 设计。没有读取 Standards 报告。
- 逐处覆盖：`release_copy_reprepare_multitable_integration_test.go`、`table_approval_schema_integration_test.go`、`release_order.go`、`00008_schema.json`、`docs/schema-migrations.md`、`web/DESIGN.md`、Web 的 `release-orders.ts`/测试、`AppShell.tsx`、`ManagedDataMutationPage.test.tsx`、`ReleaseOrdersPage.tsx`、`release-fixture.ts`；并追踪读取投影、实例保存、通知事务与候选迁移。

## 规格核对

- #100 AC-009/010/011/013/014/022：显式保存保留逐表实例；提交审批安排与节点来源分离；详情及两类列表保留流程字段，读取不补建实例。缺配置不能走固定流程兜底。
- #92 AC-019/020：发布、完结、回滚的结果收件人仍来自申请人及真实历史审批者，排除操作者；快速回滚停止未完成正向节点，并在原事务保存结果通知。重新准备的源单取消、替代实例、占用、通知及原请求结果仍共同提交。
- 读取 DTO 同时保留个人通知进度与流程，写入重放结果不携带可用于已读确认的个人进度；详情仍使用实际展示的序号，导航、缺配置提示及节点事实均保留。
- 正式 00001～00008 的 16 个 SQL/manifest 与 #97 逐字节一致；候选模板 SQL 仅由 8/9 顺排至 9/10。累计清单从真实 MySQL 生成，9 增加模板、10 增加关联及其必要键/版本；历史接管边界仍为 5。

## 证据及限度

本次只读审查，未修改源码或运行数据库。独立解析原始日志得到 20 个唯一顶层 HTTP PASS、0 FAIL。HTTP、包检查及 readiness 的 244 项源码哈希全部匹配。Web 首轮 166 PASS/9 FAIL 保留；仅两份通知夹具发生变化，补验 13 PASS，当前 167 项源码哈希全部匹配，有效覆盖 175 项。类型检查、构建及缺通知表 readiness 补验通过；原失败未抹除。

此次不将父工单后续范围列为本检查点缺陷：#98 接入后需再次顺排候选并验证新装/升级/保留数据与只读就绪；#104 的应急状态、最终相交浏览器路径、完整受影响矩阵和最终 index 仍须复核。#105/#106 的回滚实例和临时总览退出未交付，不据此关闭完整功能。

输入快照：`/tmp/rcc-template-notification-spec-merge-round1.inputhash.json`，包含 646 项源码、32 项本检查点证据及规格哈希、index stages 和证据匹配结果。

输入文件 SHA-256：`b94883caf26ee6fcea2119a451cac505ee9a5847e57f3a2c040f0aa0b94928db`。
