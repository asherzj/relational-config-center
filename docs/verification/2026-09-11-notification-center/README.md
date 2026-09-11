# #95 通知中心四视图验证

分支 `codex/issue-95-notification-center`；固定基点 `2612bb354ef71d42e193648564b9f30bd9281eb4`。工单及父规格的正文、评论、标签归档为 `issue-95.json` / `issue-92.json`。本票仅交付 AC-012；#96 持久待办/未读/轮询、#97 生命周期提醒及 #98 完整最终矩阵不在此票预建。

## 当前验收证据

| 验收切片 | 有效自动化证据 |
| --- | --- |
| 四视图成员集合及提交事实 | `ac012-mysql-final.txt` 的 `TestNotificationCenterSubmittedViewsExcludeUnsubmittedDrafts` PASS：未提交草稿和直接取消草稿排除；本人已提交/别人已提交/提交后取消正确归属；有权未操作不算已处理 |
| 当前资格、历史与单行去重 | 同日志 `TestNotificationCenterCurrentEligibilityAndDurableHandledHistory` PASS：多角色多表同单一行、同账号两次真实批准仍一行；已处理表退出待办，剩余表可接手；角色人员离开/恢复令 ADMIN 默认资格进入/退出；无自审；拒绝及撤权保留真实处理归属，列表/详情当前范围与 revision 一致 |
| 筛选先于分页与账号隔离 | 同日志 `TestNotificationCenterFiltersBeforePaginationAndProtectsIdentity` PASS：前六个完整候选页均无资格，仍找到第七和第十个可见单；游标连续且无重复漏单，满末页无空后继；状态/表/单号筛选、非法 view/limit/state、无会话、无法用 applicant_id 冒充个人视图 |
| 不兼容旧单及真实读取失败 | `scope-and-regressions.txt` 的 `TestNotificationCenterRejectsIncompatibleHistoryAndDirectoryFailure` PASS：缺快照旧 SUBMIT 事实不会被当作未提交或伪造成功列表；与正式详情同为不可用。数据库边界移走成员表后返回503，恢复后重新读取真实列表，不推导默认权 |
| 受影响 MySQL 回归 | `scope-and-regressions.txt` 另六项全部 PASS：原发布目录当前授权/筛选、人员姓名读取、分表批准、快照成员/默认切换/历史发布、空快照与默认矩阵、账号停启与历史保留。本日志七项整体 PASS，73.457s |
| Go 包与架构/契约 | `go-checks-review-final.txt`：Application、Domain、HTTP、MySQL、cmd/admin 五个受影响包全部 PASS。既有 HTTP→Application、领域无 ORM、请求身份不存共享服务等机器约束继续通过 |
| Web 状态/接口与调用方 | `web/review-final.txt` 四文件102项 PASS：NotificationsPage、ReleaseOrdersPage、release API、WorkspaceAccess。`web/typecheck-review-final.txt` 与 `web/build-review-final.txt` PASS。覆盖默认正式路由、四视图/筛选/游标、加载/失败详情返回、列表错误保留数据与只读文案、刷新不重置页位置及原会话/动作调用方 |
| 正式 Web 系统关键路径 | `browser-review-final.txt` 五组全部 PASS，28.23s；`browser-review-final/result.json` 无 page errors。真实 Admin/MySQL、22张已提交多表单与另1张取消草稿，桌面1440和390px，键盘进入第二页详情、真实批准两表、返回原视图/筛选/游标并更新待办，全部视图/仅查看ADMIN、局部读取失败与重试 |
| 共享正式详情浏览器回归 | `browser-detail-regression.txt` 原 `TestTableApprovalBrowserSystemPath` 七组全部 PASS，38.59s，errors=[]：原发布单路由、逐表批准、资格冲突恢复、默认ADMIN、无独立审批人、390px与真实发布继续工作；未重跑全部11脚本矩阵 |
| 设计与独立评审 | 最终4张截图已人工查看，桌面/390px无页级横向溢出；表格只在有名称及键盘焦点的区域内滚动，操作列固定，标题/真实ID可换行；只读错误保留Request ID及code。`standards-review.md` 原P3字体已修，最终0违规/0坏味道；`spec-review.md` 原P2范围偏差已修，最终0未解决 |

新增只读事务使候选单据、已提交/处理事实与实时资格来自一个一致快照；分页通过内部候选游标跨过无资格批次，公开游标以最后可见单为界。提交、审批和执行写路径没有新增授权分支。读取不生成通知，不新增表或迁移；角色停用后历史处理归属以原决定事件保留。

## 原始失败与证据归并

- `ac012-submitted-red.txt` 是真实行为红灯：旧目录把草稿与未提交取消单混进全部审批；`ac012-submitted-green.txt` 及 `ac012-mysql-final.txt` 的第一项通过。
- `ac012-qualification-pagination.txt` 为 **1 FAIL / 1 PASS**，不记整轮通过。失败是测试设置重新启用角色时使用旧角色版本，公共API正确返回409；修正 fixture 使用前次保存结果后，当前资格测试在 `ac012-mysql-final.txt` 通过。分页测试原轮本已通过，其最终有效结果也在该日志保留。
- `ac012-history-red.txt` 与随后 `ac012-mysql-final.txt` 的旧 `TestNotificationCenterHistoricalSubmissionAndDirectoryFailure` 属于后来被撤回的额外兼容要求，**不作为最终通过证据**。按父规格边界改名改期望后，`spec-scope-red.txt` 保留特殊成功分支的真实200反例；移除分支后的有效通过是 `scope-and-regressions.txt`。这次变化仅影响无快照旧数据，前三项正常T2数据证据继续适用。
- `browser-initial.txt` 为 **FAIL**：首组已过，真实批准成功后验收立即统计旧缓存中的两行，未等待返回列表的后台刷新。脚本改为等待已处理行离开后再数行；`browser-verified.txt` 五组 PASS。之后只有字体与只读错误文案变化，最终适用 `browser-review-final.txt`；未改审批流程以适配测试。最终截图先回到页顶稳定后保存。
- `web/` 保留全部原始 red/green 文件名，不根据文件名判断结果。`detail-green.txt` 实际失败于测试地址探针 `<output>` 与加载状态共享 status 语义，随后限定 main；`typecheck.txt` 初次失败于测试查询选项。`targeted.txt` 102项与 `typecheck-final.txt` 当时通过。只读提示的 `read-error-red.txt` 为真实文案红灯；最终统一采用 `review-final.txt` / `typecheck-review-final.txt` / `build-review-final.txt`。

原始 stdout 中 testcontainers 的行尾空格及 pnpm 的空白尾行原样保存；不为格式检查重写日志。源码与契约文档的严格 `git diff --check` 通过，原始日志的空白提示单独排除。

## 执行环境与范围

Go、Node、pnpm及 Docker/MySQL 的版本见 `environment.txt`。真实 MySQL 均经现有 `startCurrentIntegrationMySQL` 和嵌入式 Goose 当前初始化；`DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`TESTCONTAINERS_RYUK_DISABLED=true`，Go统一`-p 1`，重型 MySQL/浏览器/证据核验严格串行，同一时刻最多一台测试自有 MySQL；没有操作用户 `deploy-mysql-1` 或 `rcc-main-preview-admin-1`，没有全局清理。

命令（cwd 为 admin；输出定向至上述独立日志，浏览器各轮用独立 RCC_E2E_OUTPUT 目录）：

```sh
go test -p 1 -tags=integration ./cmd/admin -run '^TestNotificationCenter' -count=1 -timeout=5m -v
go test -p 1 -tags=integration ./cmd/admin -run '^(TestNotificationCenterRejectsIncompatibleHistoryAndDirectoryFailure|TestReleaseDraftCurrentAuthorizationAndListing|TestReleasePeopleResolveCurrentNamesWithoutAccountAdmin|TestTableApprovalPartialProgressAndFinalApproval|TestTableApprovalSnapshotMembershipFallbackAndHistory|TestTableApprovalFallbackMatrixAndEmptySnapshot|TestTableApprovalAccountAvailabilityAndDurableHistory)$' -count=1 -timeout=10m -v
go test -p 1 ./internal/application ./internal/domain ./internal/interfaces/http ./internal/infrastructure/mysql ./cmd/admin
go test -p 1 -tags='integration browser' ./cmd/admin -run '^TestNotificationCenterBrowserSystemPath$' -count=1 -timeout=5m -v
go test -p 1 -tags='integration browser' ./cmd/admin -run '^TestTableApprovalBrowserSystemPath$' -count=1 -timeout=5m -v
```

没有修改发布/回滚写事务、账号授权、角色维护、结构定义与迁移。基点已集成的 [#94](../2026-09-11-table-approval/README.md) 迁移7、真实竞争及受影响生命周期验证继续复用；[迁移未变核对](released-migrations-unchanged.json) 核对00001～00007。未跑全仓完整矩阵，按父规格由 #98 统一进行。

交付源码由 `source-manifest.json` 固定，证据由 `evidence-manifest.json` 固定，均为逐文件 SHA256；核验包括实际磁盘内容及待提交暂存内容。只完成本票提交和当前分支正常推送；父工单、子工单关闭及 Notion 由主协调者完成。

## 后续接入

#96 可以继续使用 `view`/`orders`/`next_cursor` 与通知详情URL；当前审批资格仍唯一来自 `approvalContext`，个人当前范围/revision不能跨账号缓存。`ReadReleaseOrderList` 是一致只读快照，不能被用作补造漏失通知的写入口。现有候选按批次扫描，未引入查询索引；待审候选多且当前资格稀疏时读取成本会增长，优化时必须保持资格规则与分页语义。领域词汇表已有本次全部业务术语，无新领域定义；API说明与 web/DESIGN 已同步。
