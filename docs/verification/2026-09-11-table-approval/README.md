# #94 按表审批交付验证

隔离分支 `codex/issue-94-table-approval`，固定基点 `2baec220f4fc0a54e722f3dc7ed9711853079c04`。原始来源为 [工单 #94](https://github.com/asherzj/relational-config-center/issues/94) 和 [父规格 #92](https://github.com/asherzj/relational-config-center/issues/92)；正文、评论、标签分别归档于 `issue-94.json`、`issue-92.json`。本张负责 AC-003～011，以及相应迁移、桌面、390px 和键盘接线；通知与旧账号存储值清理仍属于后续工单。

环境：Go 1.27、Node 24.19、pnpm 10.28.2、MySQL 8.4、Colima Docker 29.5.2；Go 使用 `-p 1`，同一时刻只运行一个任务自有 MySQL，浏览器和迁移验收串行。浏览器访问正式 React 页面，经 Vite 同源代理和真实 Admin 进程进入 MySQL。故障位于 HTTP、数据库或外部维护进程边界，不 mock 自有协作者。

## 验收与定向回归

| 范围 | 证据与结果 |
| --- | --- |
| AC-003/004 表角色配置与永久引用 | `ac004-red-http.txt` 为缺少路由的真实 404；`ac004-green.txt` 通过。最终 `mysql-final-targeted.txt` 覆盖多角色、空分配、独立版本、重复/不存在角色、权限、原请求重推、同键异内容、解绑后永久引用、删除与首次引用竞争，且查询/变更等其他表配置逐字不变 |
| AC-005/006 多表批准与拒绝 | `ac005-red.txt` 保留旧 APPROVER 门槛 403；`ac005-green.txt` 及最终 MySQL 通过角色 VIEWER 分表批准、部分进度、本人全部可审批表范围、完成表不得再处理、合法拒绝终止整单、真实人/时间/意见/资格来源、批准重推不重复决定 |
| AC-007/008/011 快照、实时资格与历史 | `ac007-011-snapshot.txt`、最终 MySQL 验证冻结角色身份/当时名称/空集合，改绑只影响新单，成员实时加入/移出，停用角色或账号，ADMIN 默认与恢复退出，没有通用越权，历史合法批准在撤权后保留；只改审批分配后仍能执行原冻结业务规则 |
| AC-009 无独立审批人 | `ac009-red.txt` 为旧提交成功的真实红灯；`ac009-green.txt` 及最终 MySQL验证 422 指出问题表、草稿/版本保留、任何人不能自审、在途进度保留并可补员恢复。`cancel-draft-red.txt` 保留未提交草稿取消后响应不可读的问题，`fallback-scope-concurrency.txt` 和最终 MySQL证明修复 |
| AC-010 竞争与失败语义 | 最终 MySQL覆盖同表旧工作流竞争、不同表批准/批准、批准/拒绝、批准/取消，以及成员移除、角色停用、账号停用、ADMIN 撤权四种真实竞争；以数据库事件时间核对资格与决定提交顺序。资格目录读取失败不授予默认权限，请求结果保存失败回滚审批/版本/历史，同键重推仅生效一次，同键异意图拒绝 |
| 迁移 7、既有结构与就绪 | `TestTableApprovalSchemaRecoversUpgradeWithoutChangingExistingFacts` 在 `mysql-final-targeted.txt` 中 PASS：6→7 部分 DDL/拒绝普通重跑/明确恢复/重复升级、原账号/角色/成员/引用/请求/业务/发布单事实保留、新装结构等价，缺新表时启动和就绪拒绝，控制表不能由通用表能力读写 |
| 冻结 baseline 与迁移调用方 | `schema-regressions.txt` 四项全部 PASS：#93 的 baseline 固定 5 再显式 up6、当前 7 之后的升级及已提交未确认恢复、持续 readiness 版本/列/约束/索引/引擎故障。历史测试显式构建原版本，未改变 00001～00006 发布文件 |
| Go 定向结果 | `mysql-final-targeted.txt` 17 个顶层用例全部 PASS，174.892s；`schema-regressions.txt` 4 个顶层用例全部 PASS，67.892s。此前 `availability-faults-regressions.txt` 7PASS/1FAIL 的原记录保留；失败是写响应空 approvals 与详情空数组不一致，`qualification-races-and-retry.txt` 和最终旧 workflow 严格比较已通过。`identity-regressions.txt` 8 项身份/引擎/触发器/真实插入归属回归 PASS；合并原运行中仍适用的4项通过，共 [33项MySQL用例](mysql-reconciliation.json)逐项通过。`go-checks-final.txt` 五个受影响包的非 integration 检查 PASS |
| Web 局部规则/请求与恢复 | `web/final-targeted.txt` 五文件 108 项 PASS，`web/regression-recheck.txt` 两文件 43 项 PASS，`web/tcp-recheck.txt` 真实 TCP 中断 PASS；类型检查和生产构建通过。原全量 399PASS/5FAIL 与环境端口错误没有改记全绿，逐项归并见 [Web 说明](web/reconciliation.md) |
| 正式浏览器主链 | `browser-review-final.txt` 中 `TestTableApprovalBrowserSystemPath` 最终七组 PASS，38.76s：管理员键盘绑定，390px 分配冲突保留选择，VIEWER 分表进度/ADMIN 不越权，全表通过后真实发布，资格范围冲突保留意见跨刷新并键盘重审，空快照默认 ADMIN 来源，以及无独立审批人时提交拒绝/在途进度/取消释放。`browser-review-final/result.json` page errors 为空，截图已人工查看；该日志同时存在旧入口环境 FAIL，不把整次运行称为通过 |
| 旧浏览器契约调用方 | [11脚本逐项归并](browser-reconciliation.json)：`browser-regressions.txt` 前6脚本通过，第7回滚脚本跨两个查看者比较动态资格revision失败；改为同一查看者在竞争前后完整header相等。`browser-remaining.txt` 剩余5脚本全部PASS，109.968s。保留原混合运行和截图，未重复前6有效脚本 |
| 评审修复与最后Web检查 | [Spec评审](spec-review.md)发现AC009明确422被误作未知；两项红灯后修复，`web/review-regressions.txt` 四文件131项通过，最后拒绝优先清理的 `unavailable-classification-final.txt` 两项通过；`review-typecheck-final.txt` 与 `review-build.txt` 通过。批准/拒绝的原确认范围仍冻结 |
| 独立双轴 | [Standards](standards-review.md) 0硬性/0主观问题；[Spec](spec-review.md) 1项已修复、0未解决。两轴最终增量复核通过 |

`browser-initial.txt` 是错误码文字定位错误，HTTP 前四组已真实通过，但该运行整体失败。`browser-recheck.txt` 则发现真实 390px 横向溢出：新的冲突审阅类遗漏已有块布局规则，警告框默认横向 flex 撑宽页面。修复沿用既有块布局和换行规则，保留完整宽度断言，没有裁切内容；最终 `browser-verified/scope-review-geometry-390.json` 记录实际宽度和内部滚动区域。两次失败截图与正文保留在原目录。

工具环境事件：初次 `ac004-red.txt` 是 Go cache 沙箱权限失败，不算行为红灯。一次浏览器复验的自动审批在截止前没有返回，工具明确记录未执行；在已有授权范围内获批的重试才启动浏览器，该轮实际布局失败仍保留。另一次写本目录 README 的自动审批截止超时，同一补丁按工具允许重试一次后成功。没有绕过审批限制。

## 复用范围和临时结构

[#93 的交付证据](../2026-09-11-approval-roles/README.md)保留角色管理页、角色 CRUD、账号会话隔离及正式 Compose 的六条升级部署路径；本张修改了审批动作和新控制表，因而重新验证其引用、实时成员/账号/ADMIN 竞争、当前新装/升级等价和就绪探针，不能仅因角色文件未改就声称全可复用。[#88 多表最终证据](../2026-09-11-release-final-cleanup.md)作为多表前置来源；本张按新授权契约适配实际审批调用方，保留原发布、回滚、分页和恢复断言。

审批写事务先获得现有全局账号维护锁，再遵守发布请求→发布单→明细的顺序；账号资格和角色变动与审批提交因此有一致顺序。该锁也会串行化无关发布写入及账号活动，是明确的吞吐权衡，未引入后台补偿、旁路或无锁资格窗口。

HTTP、Web 与应用层已去除旧 APPROVER 的实际审批门槛与授权旁路；旧存储值和账号授权编辑仍随 #98 迁移退出。历史不可变授权记录保留当时解释，不把删字面量当作改写历史的理由。无旧发布单数据迁移、无自动全表角色、无环境数据清理。未实现 #95～97 通知。

最终源码逐文件标识见 `source-manifest.json`（SHA256 `1a0981f7207cc05e409ccbf69d777c985120bcb42752a97af00e3f294dac2fb1`），证据逐文件标识见 `evidence-manifest.json`。00001～00006 的12份已发布 SQL/manifest 与固定基点逐字相同，见 `released-migrations-unchanged.json`。父规格、工单关闭及 Notion 由主协调者处理；提交与远端标识由交付消息核对。

旧浏览器的首次正式 runner 使用生产构建，其源码快照见 `browser-regression-source.json`；后续仅 AC009 明确失败恢复及当前提交安排显示发生生产变化，前6脚本没有触发该错误或资格变动路径，其证据继续适用。最后7组与剩余5脚本通过正式 Go 系统入口使用当前源码。旧账号入口第一次因证据目录不存在而在启动浏览器前失败；建立新目录后才重跑，见 `browser-review-final.txt` 与 `browser-remaining.txt`，没有生产代码绕行。最终生产构建也已重新验证。
