# 已批准发布单重新准备验证

固定基点：`f7a39bafe2e767b706a5f4f158e7972cdfe53af0`。

## 红绿切片

- 首个真实 HTTP/MySQL 用例从 `POST /api/v1/release-orders/:id/reprepare` 未注册开始为红；注册路由后又由角色中间件拒绝，补齐 EDITOR/ADMIN 权限映射后转绿。
- 成功切片验证原申请人核对最新配置后，源 APPROVED 单与目标占用一起取消/释放，新 DRAFT 继承标题且可编辑，双方保存 `REPREPARE` 历史；新申请人是实际操作者，新单提交后不能自批。
- 授权切片验证普通 EDITOR 不能处理他人单据、被撤销 EDITOR 的原申请人不能处理、ADMIN 可以处理且成为新申请人并仍不能自批。
- 原子失败切片使用真实 MySQL 触发器拒绝新单插入。HTTP 503 后源单、旧审批和目标均未变化，且没有半成品请求记录；移除故障后原请求成功，同键同正文精确返回同一新草稿，同键改正文返回 `409 idempotency_conflict`。
- Web 切片验证继续按钮在读取当前配置前禁用，请求使用最新记录版本；基线核对后由共享危险确认对话框说明旧单取消与目标释放，取消按钮初始聚焦且不发送请求，最终按钮使用 destructive 样式。响应丢失后跨刷新保留原正文与 `Idempotency-Key`，恢复同一草稿。

## 定向结果

- `go test ./internal/application ./internal/interfaces/http`：通过。
- `go test -tags integration ./cmd/admin -run '^(TestCurrentSessionUsesRolePermissionMatrix|TestBusinessAPIsRequireSessionAndCSRF|TestReleaseReprepare)' -count=1 -v`：通过，43.326 秒。
- `pnpm exec vitest run src/features/release-orders/ReleaseOrdersPage.test.tsx`：31/31 通过。
- `pnpm typecheck`：通过。
- `pnpm test:run`：28 个文件、302 个测试通过。
- `pnpm build`：通过，Vite 转换 2,123 个模块并生成生产资源。
- Impeccable 静态检测：对三个变更界面文件返回空问题列表。
- `make test-integration`：通过，500 PASS、0 FAIL、0 SKIP，2,032.998 秒；[汇总](2026-09-09-release-reprepare-mysql-summary.json)已随交付保存。运行前冻结的 145 个 Go、SQL、Python、module、Makefile 输入在结束时哈希不变。
- `make build`：通过，3.204 秒；原始汇总为 `/private/tmp/rcc-issue-62-final-build-v1.summary.json`。
- `pnpm --dir web test:dev`：通过，1.143 秒；原始汇总为 `/private/tmp/rcc-issue-62-final-web-dev-v1.summary.json`。
- `make test`：通过，23.965 秒；原始汇总为 `/private/tmp/rcc-issue-62-final-go-test-v1.summary.json`。
- `make test-browser`：通过，131.668 秒；原始汇总为 `/private/tmp/rcc-issue-62-final-go-browser-v1.summary.json`。
- `make test-browser-acceptance`：原完整快照通过，17 个套件共 223 项命名检查，451.376 秒，退出码 0；清理、fixture 行数和数据库后置检查均通过。[产物汇总](2026-09-09-release-reprepare-browser-full-summary.json)与[运行环境、退出及清理记录](2026-09-09-release-reprepare-browser-full-run.txt)已随交付保存。

## 独立审查

- v2 Standards 复审为 0 个 hard finding。两项接受的 P3 是后端 `copyOrder(..., reprepare bool)` 名称不能直观表达重新准备的副作用，以及复制与重新准备界面存在局部重复；本单不为此扰动已完成长回归的后端冻结。
- Spec 对 AC-009 核心行为没有发现。跨动作 pending intent 互斥属于 AC-016，已归入 #64，不作为本单完成条件。
- 截图时机单处增量的最终 Standards 复审为 0 个 hard finding、0 个新增主观发现；最终 Spec 增量复审为 0 个发现。原两项 P3 结论不变。

## 浏览器截图复验

- 原完整浏览器快照中的焦点、destructive 属性、取消无写、响应丢失原键恢复和三引擎行为断言均已通过。其桌面确认截图紧跟焦点断言，截到了 `DialogContent` 200ms `fade-in` / `zoom-in` 动画的中间帧，不能作为稳定视觉证据。
- 截图调用随后仅增加 Playwright `animations: "disabled"`，把有限动画快进到稳定末帧；产品实现和全部行为断言未变。原 17 套件结果对应修正前的脚本快照，不宣称已在新脚本哈希下整套重跑。
- 修正后定向重跑 Chromium 发布流程的 5 个套件、42 项检查，87.2 秒，退出码 0；清理、fixture 行数与数据库后置检查均通过。[命令汇总](2026-09-09-release-reprepare-stable-capture-command.json)与[定向产物汇总](2026-09-09-release-reprepare-stable-capture-summary.json)已随交付保存。
- [稳定桌面确认框](2026-09-09-release-reprepare-confirm-desktop.png)已人工查看：对话框表面不透明、层级清晰，旧单取消/目标释放/新草稿与重新审批说明完整，取消与 destructive 主动作可达且没有裁切。
- [390px 新草稿结果](2026-09-09-release-reprepare-result-390.png)已人工查看：标题、草稿状态、来源关系、历史和操作可读，没有页面整体横向溢出。

原完整浏览器门禁与截图参数修正后的受影响路径分别保留证据，避免把定向复验写成新脚本哈希下的全套重跑。

上述门禁、独立审查与稳定视觉复验均已完成，当前交付快照可提交。
