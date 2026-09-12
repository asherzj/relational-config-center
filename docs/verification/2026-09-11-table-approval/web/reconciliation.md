# #94 Web 验证说明

本目录保留独立 Web 子任务的实际终端输出。测试接缝为正式 React 路由与公开 HTTP 边界；真实 MySQL / 浏览器由主实现者串行运行，浏览器证据在上级目录，不将 HTTP fixture 测试称为真实服务证据。

- `table-red.txt` → `table-green.txt`：原页面缺少表审批入口；实现后跨完整角色分页选择，并以独立版本及幂等键保存。
- `approval-red.txt` → `approval-green.txt`：原全局 APPROVER 门槛阻止 VIEWER 角色成员；实现后只确认本人表，部分通过不显示整单已批准。
- `scope-red.txt` → `scope-green.txt`：原流程没有保存资格冲突；实现后在发布单版本不变而成员变动时，原表范围、审批版本与意见跨刷新保留，显式最新审阅才重建新请求。
- `targeted.txt`：4 文件 99 项 PASS。包括表分配冲突保留选择、读取最新后明确保存、丢失响应原键重试；旧 APPROVER/ADMIN 无表资格时不显示新审批动作；原发布日志/草稿/复制/恢复路径。
- `all.txt`：曾运行一次全量，原结果 **399 PASS / 5 FAIL，另有 1 个未处理环境错误**，不能称该次全绿。其中草稿去向 summary fixture 缺少新增 approvals/approval_context，已按正式契约补齐；真实 TCP 测试因沙箱拒绝绑定回环端口超时；其余为两个既有大表测试并行负载超时及一项后续会话场景失败。
- `regression-recheck.txt`：对受影响 ManagedDataMutationPage 和 CombinedQueryForm 两文件取消文件并行后，43 项全部 PASS；保留原业务断言，没有延长超时或删除测试。原后续会话失败亦在该次通过。
- `tcp-recheck.txt`：获自动审批允许临时本机回环端口后，真实 TCP 响应中断测试 PASS。
- `final-targeted.txt`：5 文件 **108 项全部 PASS**，包含最终表审批、发布单、账号角色文案和请求日志/API 回归；按主协调者要求未再次全量运行。
- `final-typecheck.txt`、`final-build.txt`：最终 TypeScript 检查与生产构建均退出 0，包括截图核对后补充的抽屉可见真实表名。
- `install.txt`：从已有 pnpm 缓存离线冻结安装，未改锁文件。

已完成所有已知受本张审批契约影响的旧浏览器 fixture 适配：`release-approvals.cjs`、`release-batches.cjs`、`write-recovery.cjs`、`release-rollback-reason.cjs`、`complex-fields.cjs`、`operation-coverage.cjs`、`field-display.cjs`、`release-rollbacks.cjs`、`browser-accessibility.cjs`、`release-multitable.cjs`、`accounts.mjs`。专用 reviewer 使用 VIEWER 与公开审批角色/表绑定；直接 approve/reject 在准备新请求时以实际 reviewer 读取确认范围与审批 revision，保留调用方明确提供的原发布版本、意见与原有故障/幂等断言。`accounts.mjs` 的旧 APPROVER checkbox 与存储值断言仍保留，同时给相互审批的两个账号配置实际表角色；它不再依赖旧全局值获得资格。`table-approvals.cjs` 与新 `TestTableApprovalBrowserSystemPath` 提供本张独立入口。已知契约回归由本张精准组收口，不留到 #98。

初次真实浏览器已走通前四组操作，后在成员范围冲突处因脚本等待不存在的错误码文字失败。已依据 ErrorState 实际渲染修正为断言真实 HTTP 409 / release_approval_conflict，再检查正式中文提示，保留全部范围/意见断言；未将初次运行称为通过。脚本后续失败会保存 failure.json、各页面截图与正文。

已用 view_image 查看初次的 `table-assignment-conflict-390.png` 和 `table-approval-partial-desktop.png`：手机角色长名称/永久 ID 可换行，底部保存与关闭可达；选择与服务器最新分配分开显示；桌面进度仍为待审批且清楚显示 1 / 2 表；资格来源、操作者、时间与意见可读；页面沿用中性灰与石墨操作。发现抽屉 eyebrow 仅屏幕阅读器可见，已在本张抽屉正文补充“配置表”与真实表名，等待后续浏览器截图复核。

第二轮真实浏览器通过实际 HTTP409/错误码和原意见恢复，但在390px最新范围审阅处发现真实页面溢出；`browser-final/failure-surface-2.png` 宽644px，截图显示标题竖排、右侧卡片越过页面边界。原因是新 `release-conflict-review` 类仍受 `.inline-alert` 的横向 flex 布局控制，没有继承已有 `release-recovery` 的块布局与长串换行。已将相同的局部块布局规则应用于冲突审阅类；不裁切内容，不改变差异表自身横向滚动，不放宽整页宽度断言。脚本增加 `scope-review-geometry-390.json`，记录视口、整页、外层与内部滚动区的真实尺寸，等待主协调者串行复跑验证。`layout-build.txt` 为本次局部布局修正后的生产构建结果。


`RCC_E2E_SUITE=approval-contract-regression RCC_E2E_ENGINES=chromium make test-browser-acceptance` 是本张精准真实服务复验入口，由主协调者串行执行。新分组在一次正式 runner 启动中覆盖上述 11 个受影响脚本，复用已有公开账号/API fixture、SQL 故障测试环境、schema 启动、数据库 postcheck 与资源 cleanup。Chromium 固定用于原固定引擎脚本；支持多引擎的可访问性、回滚、账号恢复、多表与回滚原因脚本按 `RCC_E2E_ENGINES` 迭代。即使只选择其他引擎也会安装这些固定脚本所需的 Chromium。原 `all` 选择范围保持原样；这是按实际调用契约划定的回归组，不是跳过已知失败。

`browser-callers-static.txt`：全部 27 个 Web 浏览器/共享脚本通过 Node 语法检查，正式 runner 通过 bash 语法检查；用只打印调用的替身验证精准组单 Chromium 为 11 次调用、Chromium+Firefox 为 16 次、单 WebKit 仍安装 Chromium；与 HEAD 比较原 all 的双引擎调用清单完全一致。`fixture-envelope-check.txt` 是内存请求边界检查，确认独立 reviewer 上下文读取没有覆盖旧 expected_version/意见，确认表数组不会随后续范围变动自动变化。这些不冒充真实浏览器/HTTP/MySQL 证据。

已核实 `web/e2e/fixtures/release-rollbacks.sql` 和 `field-display.sql`：两者只种业务表、业务数据与 rcc_table_policies，没有旧发布单 header 或审批历史。脚本通过公开接口创建、提交、审批和执行生成真实历史，本轮无需伪造审批事实或增加生产兼容。

AC009 新增正式浏览器断言覆盖同一张管理员自申请单：空分配且没有其他 ADMIN 时逐表显示无独立审批人，真实提交返回 422 / release_approver_unavailable，中文提示可继续处理且保留草稿版本；加入独立成员后提交，移除成员时保留在途审批进度与版本，再取消并重新申请同目标验证释放。主协调者已运行并在 `../browser-unavailable-red.txt` 记录真实红灯：HTTP 422 与错误码断言通过，等待中文提示失败。随后仅为 `release_approver_unavailable` 补中文映射；`unavailable-targeted.txt` 中两项表审批页面定向测试通过（其余 71 项按名称筛选未运行），`unavailable-typecheck.txt` 类型检查退出 0。浏览器完整新场景与精准旧 11 组仍由主协调者串行复验，当前未宣称这些浏览器复验已通过。

最终归并：七组正式页面已在 `../browser-review-final.txt` 的 TestTableApprovalBrowserSystemPath 通过，包含中文提示与明确拒绝不保留未知请求；同日志另一个旧入口缺少证据目录的环境失败保留。11个旧脚本的首次正式runner前6通过，第7回滚跨两个publisher比较动态revision失败；同一publisher的完整header前后比较修正后，剩余5脚本在 `../browser-remaining.txt` 全部通过，明细见上级 browser-reconciliation.json。首次6脚本源码快照保留，后续AC009专属修复不影响这些通过场景。

Spec审查发现明确422误作未知，补两项公开Web边界红灯后，client/useReleaseWrite清除明确未提交（包括未知后原键拒绝），提交窗口显示最新审批安排，审批确认仍冻结原范围。`unavailable-classification-green.txt` 虽以green命名，实际两项仍在末尾遇到重复关闭按钮定位失败；不得称通过。改为底部关闭按钮后，`review-regressions.txt` 四文件131项通过，最后优先清理分支的两项在 `unavailable-classification-final.txt` 通过。`review-typecheck.txt` 的新增测试ByRoleOptions.exact错误保留，修正后 `review-typecheck-final.txt` 通过；`review-build.txt` 为最终生产构建。
