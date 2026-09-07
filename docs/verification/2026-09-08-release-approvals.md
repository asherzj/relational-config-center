# T4 / #51 验收记录

固定累计基点：`b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c`（T3）。独占分支 `codex/issue-51-approval-targets`。本记录由实际测试结果补齐，未列为通过的检查不代表已完成。

| 用例 | 自动化证据 |
|---|---|
| AC-018 提交重验与冻结 | `TestReleaseSubmitFreezesIntent`、`TestReleaseSubmitRevalidatesBaselineAndRules`、`TestReleaseFreezeTracksExecutionSemantics`、`TestReleaseFreezeMetadataVisibility`、`TestReleaseFreezeMetadataGrantNameIdentity`、`TestReleaseFreezeMetadataCaseInsensitiveNames`、`TestReleaseExecutionSchemaHoldsMetadataLock` |
| AC-019 目标竞争/零残留/释放 | `TestReleaseTargetsCompeteAndCancelReleases`（Résumé/RESUME 真主键等价）；`TestReleaseWorkflowAtomicityAndCompetition`（结果存储失败回滚）；`TestReleaseAutoIncrementZeroIdentity`（生成型 0 拒绝，NO_AUTO_VALUE_ON_ZERO 下真实 0 占用） |
| AC-020 他人审批/必填意见/自批禁止 | `TestReleaseApprovalCurrentRolesAndHistory`；Web 仅 APPROVER 流程 |
| AC-021 拒绝/重新核实/复制 | `TestReleaseRejectedCopyRechecksBaseline`；浏览器拒绝→读取新基线→复制 |
| AC-022 当前权限取消与释放 | 已批准取消/在途取消的上述 HTTP 用例；浏览器申请人取消已批准单 |
| AC-023 状态 CAS 竞争 | `TestReleaseWorkflowAtomicityAndCompetition` 同版本批准/拒绝/取消三方竞争 |
| AC-024 原版本/原键成功重放 | `TestReleaseApprovalCurrentRolesAndHistory`；浏览器真实提交后丢审批响应，刷新同键找回 |
| AC-025 历史批准不因撤权变化 | `TestReleaseApprovalCurrentRolesAndHistory` 当前角色撤权后拒绝新请求，旧审批保持 |

有效红测按切片得到 submit/approve/copy 路由缺失、等价记录同时成功提交、审批意见在明确冲突刷新后丢失；对应实现再转绿。Web 现有草稿恢复回归同轮保留。

MySQL 元数据实验曾发现 ALTER COMMENT 重写生成表达式/CHECK 的字符集；生产实现未删语义字段。稳定正例分别证明普通字段表描述、规则名称描述、无关行写入、自增游标和 ANALYZE；默认值/生成式/索引/目标触发器变化会改变摘要。权限 fixture 实际授予全局 TRIGGER 再部分撤销目标 Schema，证明隐藏触发器时提交拒绝，恢复直接表授权后原键提交成功。

正式浏览器入口增加 `web/e2e/release-approvals.cjs`，与已有 accounts/unsaved-changes/rule-clarity/release-drafts 同由 `make test-browser` 执行。最终正式重跑实际退出 0，权限修复后 Admin 包 68.775 秒，系统路径 67.85 秒，T4 子流程 10.04 秒。先前最终尝试在 accounts 的角色抽屉 `animation.finished` 因 transition 取消而失败；修复仅接受该等待的 `AbortError`，继续等待替代动画，原角色、布局和历史断言完整保留。Standards 已补核此增量和正式日志。最终 [390px 页面截图](2026-09-08-release-approvals-390.png) 已查看，展示冻结差异及复制、提交、批准、取消的永久历史。

T5 负责真正业务发布、最终行 Codec、附带写入能力检查和旧直写入口删除；T6 删除单条限制。本单不宣称整体发布闭环或 Runtime 分发完成。接口、权限、迁移和 T5 边界见 [审批说明](../admin-release-approvals.md)。


两轴初审修复：Spec 发现生成型自增 0 被当作目标，已通过实际红→绿修复；Standards 的加载状态、事务锁定命名和迁移入口说明已修复。两轴对固定基点完整差异及后续权限修复增量的代码复审，未解决问题均为 0。

收尾权限审计又以真实 MySQL 发现旧授权元数据普通 ci 比较可误用大小写不同 Schema、表或账号的授权；各负例确认目标触发器实际隐藏但旧提交返回 200。修复按数据库对象大小写模式和完整账号身份精确比较，并保留特殊字符用户名的真实元数据格式。完整权限切片 27.671 秒通过，包含大小写不敏感实例和合法授权后同键恢复。此前在跑的旧版本完整 MySQL 套件主动停止，其日志不作为最终通过证据；修复后重新冻结并跑正式完整入口。

Web 234 项测试（23 文件）、typecheck 和 build 均实际退出 0，随后没有修改 Web 产品代码。Go 全模块测试包含依赖方向、草稿事务禁止业务写入口及新增发布动作公共路由/历史不可删契约检查；权限增量后的最终 `make test` 和 `make build` 均实际退出 0，正式浏览器也已按上述结果通过；最终 `make test-integration` 已实际退出 0，Admin 包 1340.976 秒，HTTP 包 128.666 秒，其余包全部通过。最终后端 `git diff b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c -- admin deploy` 的 SHA-256 为 `800eee0264b65cb929f14e0e85d757e728a9654ecb928684db1f894ae3520d15`，所有最终后端检查均基于此冻结内容。

集成超时预算从每包 25 分钟调整为 30 分钟，CI job 从 30 调整为 35 分钟。依据是 T3 最终 Admin 1226.943 秒，加上本单新的真实 MySQL fixture 与执行波动；没有删除或跳过测试，遇到真实死锁仍须单独定位。


最终证据索引（本次执行日志）：

| 正式检查 | 结果 | 日志 |
|---|---|---|
| `make test` / `make build` | 实际退出 0 | `/private/tmp/t4-final-go-test-v2.log`、`/private/tmp/t4-final-go-build-v2.log` |
| `pnpm --dir web test:run` / typecheck / build | 234 项通过，均退出 0 | `/private/tmp/t4-reviewed-web.log`、`/private/tmp/t4-reviewed-typecheck.log`、`/private/tmp/t4-reviewed-web-build.log` |
| `make test-integration` | 全部包通过，实际退出 0 | `/private/tmp/t4-final-mysql-v2.log` |
| `make test-browser` | 全部正式路径通过，实际退出 0 | `/private/tmp/t4-final-browser-v2.log` |
| Standards / Spec 独立只读复审 | 各 0 项未解决 | 固定基点如上；已修复初审及权限身份增量问题 |

旧中止的完整 MySQL 日志和动画取消失败日志仅记录诊断过程，不替代上述最终通过结果。推送 SHA 与工单外部收尾记录见 [GitHub #51](https://github.com/asherzj/relational-config-center/issues/51)。
