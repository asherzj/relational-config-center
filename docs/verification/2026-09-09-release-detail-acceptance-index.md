# 发布单详情交互验收索引

父规格：[GitHub #59](https://github.com/asherzj/relational-config-center/issues/59)。五张子工单均已完成验证、评审、推送、Notion 更新与关闭，16 条验收要求全部交付；实现整合到 `codex/new-feature-20260908`，父规格保持打开。

| 验收项 | 行为 | 负责工单 | 状态与证据 |
| --- | --- | --- | --- |
| AC-001 | 普通发布待完结，重叠目标被保护，不重叠目标可提交 | #60 | 已交付；T2 验证记录 |
| AC-002 | 真实生成 ID 和删除身份持续受保护 | #60 | 已交付；T2 验证记录 |
| AC-003 | 其他当前发布人员可完结/快速回滚，其他角色拒绝 | #63 | 已交付；T4 验证记录 |
| AC-004 | 免审批整单快速恢复，保存真实结果并释放目标 | #63 | 已交付；T4 验证记录 |
| AC-005 | 快速回滚明确失败时无部分结果 | #63 | 已交付；T4 验证记录 |
| AC-006 | 完结/回滚竞争与未知结果重放至多生效一次 | #63 | 已交付；T4 验证记录 |
| AC-007 | 完结不改业务值，之后普通回滚重新审批 | #60 | 已交付；T2 验证记录 |
| AC-008 | 普通回滚结束原单与反向结果，无连续反向入口 | #60 | 已交付；T2 验证记录 |
| AC-009 | 原子重新准备、实际新申请人、明确失败不变与原键恢复 | #62 | 已交付；T3 验证记录 |
| AC-010 | 全部草稿入口的标题、Unicode 边界、版本校验与冻结 | #61 | 已交付；T1 验证记录 |
| AC-011 | 当前姓名、永久身份归属、查询失败回退与权限边界 | #61 | 已交付；T1 验证记录 |
| AC-012 | 字段值区别、仅看变化、混合操作与千项分页 | #64 | 已交付；T5 验证记录 |
| AC-013 | 默认实际结果，切换申请差异与回滚结果 | #64 | 已交付；T5 验证记录 |
| AC-014 | 全状态、最近五条/全部历史、复制与关联跳转、手机及键盘交互 | #64 | 已交付；T5 验证记录 |
| AC-015 | 快速回滚预览、原因、确认及完结影响说明 | #63 | 已交付；T4 验证记录 |
| AC-016 | 处理/未知请求跨动作互斥，刷新和会话恢复保留原内容及请求标识 | #64 | 已交付；T5 验证记录 |

- T2：[提交 911f23b](https://github.com/asherzj/relational-config-center/commit/911f23b3c1d2f79008f641b317a71b2ddb042a9e)、[验证记录](https://github.com/asherzj/relational-config-center/blob/911f23b3c1d2f79008f641b317a71b2ddb042a9e/docs/verification/2026-09-08-release-completion.md)、[#60 完成证据](https://github.com/asherzj/relational-config-center/issues/60#issuecomment-5591821061)。真实 MySQL 504 项、完整浏览器 17 套件/224 检查及其余 Go/Web 检查通过；Standards/Spec 无剩余问题。Notion PM-020/PM-022 已更新并读回核验。
- T1：[提交 f7a39ba](https://github.com/asherzj/relational-config-center/commit/f7a39bafe2e767b706a5f4f158e7972cdfe53af0)、[验证记录](https://github.com/asherzj/relational-config-center/blob/f7a39bafe2e767b706a5f4f158e7972cdfe53af0/docs/verification/2026-09-09-release-title-people.md)、[#61 完成证据](https://github.com/asherzj/relational-config-center/issues/61#issuecomment-5592494328)。Web 300 项、真实 MySQL 497 项、Go 浏览器入口及完整三引擎 17 套件/222 检查通过；Spec 无剩余发现，Standards 无硬性发现，保留 1 条已接受的非阻断 P3 建议。Notion PM-020 已更新并读回核验。

- T3：[提交 49a08fe](https://github.com/asherzj/relational-config-center/commit/49a08fe31c4771c4727a44f527d11c8f8bd22e49)、[验证记录](https://github.com/asherzj/relational-config-center/blob/49a08fe31c4771c4727a44f527d11c8f8bd22e49/docs/verification/2026-09-09-release-reprepare.md)、[#62 完成证据](https://github.com/asherzj/relational-config-center/issues/62#issuecomment-5593579110)。Web 302 项、真实 MySQL 500 项、两条正式浏览器入口（完整三引擎 17 套件/223 检查）通过；后续单处截图时机修正经 5 套件/42 检查和人工看图验证。Standards 无硬性发现，保留 2 条已接受 P3；Spec AC-009 核心无发现，跨操作互斥 P2 最终由 #64 修复并通过真实浏览器回归。Notion PM-020 已更新且读回核验。

- T4：[提交 dddf1c3](https://github.com/asherzj/relational-config-center/commit/dddf1c3c78f970a656642062ede28c7f41971a8f)、[验证记录](https://github.com/asherzj/relational-config-center/blob/dddf1c3c78f970a656642062ede28c7f41971a8f/docs/verification/2026-09-09-quick-rollback.md)、[#63 完成证据](https://github.com/asherzj/relational-config-center/issues/63#issuecomment-5593705098)。Web 309 项、真实 MySQL 530 项、两条正式浏览器入口及完整三引擎 17 套件/234 检查通过；每引擎包含 5 项普通回滚与 3 项快速回滚检查。真实依赖合并和最终增量均经双轴审查，Spec 无发现、Standards 无硬性发现，3 条非阻断 P3 已记录；Notion PM-020/PM-022 已更新且读回核验。

- T5：[提交 fc205b0](https://github.com/asherzj/relational-config-center/commit/fc205b06f98d5e2e2f6a50e1b37294f846e8bb55)、[验证记录](https://github.com/asherzj/relational-config-center/blob/fc205b06f98d5e2e2f6a50e1b37294f846e8bb55/docs/verification/2026-09-09-release-detail.md)、[#64 完成证据](https://github.com/asherzj/relational-config-center/issues/64#issuecomment-5595421841)。最终合并后真实 MySQL 533 项测试与子测试、Web 330 项及 Go/构建/开发环境检查通过；正式 Go 浏览器入口通过，全量 Chromium、Firefox、WebKit 验收 17 套件/244 检查通过，数据库复查、测试数据不变和清理均已核验。桌面与 390px 截图完成视觉复核，Standards 无硬性问题、Spec 无剩余阻断，3 条非阻断 P3 已接受。Notion PM-020 已更新并读回核验。

AC-012 的最终过滤语义按申请内容判断：MODIFY 默认隐藏未提交及确定同值字段，关闭过滤后恢复完整字段；自动填写和数据库生成说明继续显示，不推测数据库触发器的最终结果。真实部分更新检查覆盖过滤往返。AC-014 的宽表实际滚动容器可通过 Tab 到达，三引擎均验证用方向键向右滚动可查看完整申请值列。

#62 发现的 AC-016 跨动作互斥 P2 已在 #64 关闭：当 cancel/execute/reprepare 均允许时，重新准备请求在发出前或提交后丢失响应，关闭弹窗、刷新和当前角色变化均保持冲突动作禁用；恢复使用同一正文与请求标识，得到唯一新草稿。明确拒绝后先重新审阅再建立新请求，会话和账号隔离继续由原验证覆盖。

依赖整合提交 `e3b68383bea425f3258f726d34a967ef6b1aa61c` 已纳入真实冲突解决审查；最终功能分支包含全部五张子工单交付提交。未引入临时兼容、双写或 Feature Flag，无删除遗留。领域、ADR、接口说明和 Web 设计规则均随对应工单更新。

上述证据运行于 macOS，不能据此声称 Linux CI 已通过。功能分支交付不代表已合并默认分支或部署；父规格 #59 以及旧 #48/#55 未被自动关闭。
