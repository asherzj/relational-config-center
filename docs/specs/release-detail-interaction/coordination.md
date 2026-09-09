# 发布单详情交付编排

状态：五张子工单已独立交付并关闭，16 条验收要求全部有证据；Notion 项目记录已更新并读回核验。用户于 2026-09-08 授权按 flow-feature 及以下独立智能体分工实现，需求以 [GitHub #59](https://github.com/asherzj/relational-config-center/issues/59) 为准。详细结果见[验收索引](../../verification/2026-09-09-release-detail-acceptance-index.md)。

## 工单与主要验收归属

| 工单 | 完整功能 | 主要验收用例 | 独立智能体 | 已交付提交 |
| --- | --- | --- | --- | --- |
| T1 / [#61](https://github.com/asherzj/relational-config-center/issues/61) | 草稿标题冻结与相关人员身份 | AC-010、011 | GPT-5.6 Sol / high | [f7a39ba](https://github.com/asherzj/relational-config-center/commit/f7a39bafe2e767b706a5f4f158e7972cdfe53af0) |
| T2 / [#60](https://github.com/asherzj/relational-config-center/issues/60) | 发布待完结、目标保护、人工完结与普通回滚收尾 | AC-001、002、007、008 | GPT-6 Astra / xhigh | [911f23b](https://github.com/asherzj/relational-config-center/commit/911f23b3c1d2f79008f641b317a71b2ddb042a9e) |
| T3 / [#62](https://github.com/asherzj/relational-config-center/issues/62) | 原子重新准备与新审批 | AC-009 | GPT-5.6 Sol / high | [49a08fe](https://github.com/asherzj/relational-config-center/commit/49a08fe31c4771c4727a44f527d11c8f8bd22e49) |
| T4 / [#63](https://github.com/asherzj/relational-config-center/issues/63) | 快速回滚、权限、竞争及恢复 | AC-003、004、005、006、015 | GPT-6 Astra / xhigh | [dddf1c3](https://github.com/asherzj/relational-config-center/commit/dddf1c3c78f970a656642062ede28c7f41971a8f) |
| T5 / [#64](https://github.com/asherzj/relational-config-center/issues/64) | 完整详情、差异、结果、历史和请求恢复 | AC-012、013、014、016 | GPT-6 Astra / high | [fc205b0](https://github.com/asherzj/relational-config-center/commit/fc205b06f98d5e2e2f6a50e1b37294f846e8bb55) |

## 依赖与交付

T1/T2 无依赖；T3 依赖 T1，T4 依赖 T1/T2，T5 依赖 T3/T4。每张工单都在全部原生阻塞项关闭后，由全新上下文的智能体在独立工作树读取完整工单、领域与 ADR，完成实现、验证、Standards/Spec 双轴审查和独立提交。各工单分支均已推送并核验远端提交。

T5 使用已交付接口，整合 T3/T4 后的真实冲突解决也纳入审查；最终合并后的 MySQL、Go/Web 和两条正式浏览器入口通过。AC-016 跨操作互斥和 AC-012 默认过滤问题均在 T5 修复并留下真实回归证据。没有以候选代码或模拟接口充当已交付结果。

## 最终分支与文档

`codex/new-feature-20260908` 快进整合全部工单实现，未合并默认分支或改写远端历史。领域、ADR、接口说明及 `web/DESIGN.md` 随所属工单维护；本目录及设计讨论保留已确认需求和拆分依据，验收状态统一引用索引。未引入临时兼容层、双写或 Feature Flag，无额外删除工单。父规格 #59 和旧 #48/#55 的状态保持原样。

## 实现前参考的处置

此前主工作区位于 `f513859`，存在尚未验证的后端及部分 Web 中间实现。它们未作为共同生产基线提交，而是只供各工单选择性参考。最终整合前按确切文件范围归档并保存到 Git stash，再由已验证交付实现替换；只恢复需求、编排与验收文档，未恢复旧产品代码覆盖新实现。

功能实现和验收收尾时，用户原有 `AGENTS.md`、`docs/agents/domain.md`、`docs/agents/design.md` 改动按原文件哈希保留。随后按用户提交、推送并创建目标为 `main` 的合并请求的要求，将这三份规范文档原样纳入独立提交。原始参考归档位于 `/private/tmp/rcc-release-detail-reference`，其保存状态与最终快进过程有本地校验记录。
