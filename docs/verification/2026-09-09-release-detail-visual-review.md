本文件保留独立视觉初评和随后同代码新截图确认的原文。初评中关于 WebKit 失败/runner 运行中的陈述是当时状态；最终正式全量三引擎已 exit0（244项），当前验收结果以[最终验证记录](2026-09-09-release-detail.md)及[结果摘要](2026-09-09-release-detail-results.json)为准。视觉结论本身只覆盖所见图面。

disposition: ship

输入边界：未提供 PRODUCT.md、五块命名方向卡及 seed、QUALITY BAR card、approved comp；本次为用户明确限定的既有界面局部 Operate 扩展，以已确认规格、领域文档、web/DESIGN.md 与原图信息组织为审阅依据。未读取完整 runner 最终输出，也未重新执行键盘、事务或浏览器测试。父任务最新报告正式 all 在 WebKit 键盘横滚最右端断言失败，正在诊断；下述 ship 仅为所见 Chromium 稳定视图的视觉裁定，不构成功能最终验收通过。

## persistence

pass（本次局部扩展范围）。候选 web/DESIGN.md 已持久化四阶段、身份与操作、申请/实际结果、最近历史、字段过滤、局部横滚及请求恢复规则，与截图一致；CONTEXT.md、CONTEXT-MAP.md、admin/CONTEXT.md 和已确认 spec.md / 05-detail.md 保留业务事实。PRODUCT.md 缺失作为项目文档输入限制记录，不把本次既有界面扩展扩大为新建全站产品文档或新视觉方向流程。四张指定截图全部已实际打开：1440px 桌面和三张 390px 手机全页图均从文档顶部开始，内容对应批准待发布、原单已回滚、快速回滚结果；无黑块、空白遮挡、toast 遮盖或整体横向溢出的可见证据。

## fidelity

参考图首先识别的信息组织为：四阶段横排；基本信息和当前操作并列；明细按项折叠、字段前后对比；历史位于末尾。蓝色主题、示例中文字段、示例单号不属于获批视觉要求。

| 元素 / 契约 | 判定 | 依据 |
| --- | --- | --- |
| THESIS：快速辨认进展、审阅变化、执行下一步 | match | 第一屏先出现阶段；桌面同屏继续呈现标题、身份和唯一石墨主操作。 |
| OWN-WORLD / TYPE | match | 系统中文无衬线、数据等宽、标题与辅助信息分级，符合既有 DESIGN；不是需要另选展示字体的新视觉世界。 |
| MATERIAL | match | 白色细边框面板和真实数据表，没有伪造材质、渐变替代资产或装饰性物理效果。 |
| GROUND | match | 画布中性近白、白色面板、石墨主操作与 DESIGN 的 #fafafa / #ffffff / #27272a 目标一致；原图蓝色未被误当为约束。 |
| STORY：阶段 → 信息与动作 → 变更/结果 → 历史 | match | 桌面与手机均保留该顺序，未增加无关模块。 |
| FIRST VIEWPORT | adaptation | 390px 四阶段为两列、基本信息和动作改为纵排，依据用户手机要求与 DESIGN 响应式约定；完整人员 ID 导致增高，保持可审阅而非截断。 |
| FORM | adaptation | 采用既有 shadcn 面板、按钮、字号与间距；用户明确保持全站视觉，故不要求新世界 concept roll 或新增签名形态。 |
| 四阶段及真实人员归属 | match | 已批准单显示准备/审批完成而发布待执行；原回滚单末阶段显示已回滚及减号；快速反向单明确无需再次审批；抽样代码按对应历史事件取人员和时间。 |
| 基本信息及操作层级 | match | 长标题、完整32位单号、申请人与审批人 ID 可换行；执行发布为唯一实心主操作，整单范围说明可见。 |
| 申请差异 | match | 默认勾选仅看变更、第一项展开；截图保留真实变化及生成说明。原值/申请值列以文字和浅红/浅绿共同区分，窄表局部横滚符合已确认方案。 |
| 实际发布 / 恢复结果 | match | 原单有申请差异、原发布结果、恢复结果入口；快速回滚结果显示恢复结果及实际执行者、版本、字段值，未将免审批表现为批准事件。 |
| 最近历史与关联单 | match | 中文动作、永久账号 ID、时间、版本和关联链接可见；原回滚单显示最近五条并提供查看全部七条。 |
| 真值与 craft floor | match | Browser acceptance / 浏览器验收数据明确为验收语境；没有商业承诺或虚构分发成功。无装饰 eyebrow、彩色侧条、硬阴影或 Unicode 代替图标；差异展开三角为原生 details 控件标记。 |

## ceiling

reached（既有中性灰管理界面的范围）。层级、局部红绿差异、长数据换行、状态图标、可见焦点和响应式重排足以支持本次审阅任务；没有需要借新字体、材质、装饰或动画补足的信息组织缺口。父任务已报告 WebKit 键盘横滚断言尚未通过；正式 Tab + ArrowRight、事务与三引擎最终通过状态由 runner 证据判定，本结论仅覆盖提供的 Chromium 视觉工件及抽样实现。

## material_fixes

无。

## keep

保留四段阅读顺序、唯一主动作、真实人员永久 ID 与时间、申请/实际/恢复来源分离、未提交与生成值区别，以及390px完整内容和局部字段横滚。

## 最终证据版本确认：fullall v3

disposition: ship（延续已完成的视觉裁定，仅覆盖下列已查看视图）。

已逐张重新使用 view_image 打开并确认以下正式 fullall v3 新图：

- /private/tmp/rcc-issue-64-three-engine-v3-20260909/release-workflow/approvals/release-detail-approved-desktop.png — 1440×2261，批准待发布桌面视图有效；文档顶部完整、无 toast 遮挡。
- /private/tmp/rcc-issue-64-three-engine-v3-20260909/release-workflow/approvals/release-detail-approved-390.png — 390×3030，批准待发布手机视图有效；文档顶部完整、无 toast 遮挡。
- /private/tmp/rcc-issue-64-three-engine-v3-20260909/release-workflow/rollback-chromium/release-rollback-original-mobile.png — 390×3687，原单已回滚及原发布结果视图有效；文档顶部完整、无 toast 遮挡。
- /private/tmp/rcc-issue-64-three-engine-v3-20260909/release-workflow/rollback-chromium/release-quick-rollback-result-mobile.png — 390×2922，快速回滚结果及免审批状态视图有效；文档顶部完整、无 toast 遮挡。

新图保持原视觉结论：信息顺序、灰/石墨方向、长内容换行、局部字段表边界和结果/历史组织一致，无新增取证缺陷，无需产品修正或重新取证。本次仅确认相同产品代码的截图证据版本，未重新开放视觉打磨。正式 runner 仍在收尾，不据截图宣称全量或跨引擎行为已通过；此前 v2 WebKit 失败记录保留为历史事实。
