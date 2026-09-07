# 可靠性工作包阶段 5：跨浏览器、键盘与窄屏验收

日期：2026-09-07。分支：`codex/management-reliability-20260907`。阶段起点：`49fc66f`。

## 结论

管理页面现在把 Drawer、Change Set、确认框和离开提示作为同一个可嵌套的 modal 栈管理。打开任一 modal 后，顶层之外的页面分支会进入原生 `inert`，焦点离开时会回到顶层；只有顶层响应 Tab、Shift+Tab 和 Escape。关闭顶层后，焦点优先回到仍可用的触发控件；若该控件已经隐藏、卸载或禁用，则回到仍打开的下层 modal。`html` 与 `body` 在最后一个 modal 关闭前保持滚动锁。

最终在 Chromium `151.0.7922.34`、Firefox `153.0` 和 Playwright WebKit `26.5` 各完成 7 组 production Web → Bearer Admin → 独占 MySQL 8.4 验收，共 21 组引擎检查，全部通过，三个结果的 `pageErrors` 都为空。Firefox 与 WebKit 的组合运行以状态 0 结束，并确认容器和 volume 清理；每个引擎的未知结果用例清理后 SQL 剩余行为 0。

## 红灯与最小修复

修复前的真实 Chromium 回路位于 `../../../.records-reliability-20260907/stage5-modal-chromium-red-real/`。运行时间为 07:45:35Z–07:45:57Z，production build 和真实 Admin/MySQL 已启动，页面检查得到：

```json
{"inert":false,"focusStayedInTopModal":false,"bodyClasses":["drawer-open"]}
```

该次以状态 1 结束并完成资源清理。这里的背景焦点检查使用 `HTMLElement.focus()`，目的是直接检验原生 `inert` 是否阻止程序化聚焦；它不代表键盘 Tab 手势。组件紧凑回归同时暴露了 CSS 隐藏控件会被当成循环端点、独立 ConfirmDialog 不会锁滚动；后续补审还验证了嵌套 modal 关闭时，如果原触发按钮已经变为 disabled，旧逻辑会把焦点留在 `body`。

修复集中在共享 focus hook：

- 按实际打开顺序注册 modal，原生 inert 覆盖顶层背后的兄弟分支；不改写元素原有 inert 状态。
- 捕获顶层的键盘和 `focusin`，仅顶层处理 Tab/Escape；焦点异常离开时回到可用的初始控件或 dialog 表面。
- Tab 列表排除 disabled、负 tab index、inert、`hidden`、`aria-hidden`、`display:none` 和隐藏的祖先。
- 恢复焦点前重新检查连接、可见、启用和 inert 状态；不可恢复时聚焦下层 modal。
- 由 modal 栈统一给 `html`、`body` 设置 `modal-open`，覆盖独立确认框及嵌套关闭过程。

紧凑组件绿灯共 9 项，包含背景 inert、程序化失焦拦截、隐藏端点、独立确认框滚动锁、disabled 初始焦点、嵌套关闭恢复及触发控件中途 disabled 的降级路径。首次真实修复验证 `stage5-modal-chromium-green-2/` 已得到 `inert:true`、焦点保留和 `modal-open`。

## 三引擎真实操作

每个引擎都从真实点击打开页面控件，并断言打开后的实际焦点；套件没有先对 Drawer 或 Change Set 人工调用 `focus()` 来掩盖初始焦点。完整路径如下：

| 场景 | 真实操作和结果 |
| --- | --- |
| Drawer 焦点与背景 | Shift+Tab 绕过附加的隐藏/disabled 夹具到“取消”，Tab 到“关闭”；人为制造失焦后，再按一次真实 Tab 回到顶层。背景分支为 inert，`html`/`body` 均锁滚动。 |
| 未保存导航 | 浏览器 history back 打开离开提示，取消后留在当前记录；再次 back 并确认后离开，forward 返回原 history entry。下层 Drawer 在提示打开时为 inert，关闭后焦点恢复。 |
| 320 长 Drawer | 视口为 320×568 CSS px；长字段没有造成 document 横向溢出，可选 id 默认未勾选。鼠标悬停滚动容器后执行真实 wheel：Chromium 889→1249、Firefox 881→1199、WebKit 890→1250；滚动后逐个确认底部操作进入视口并真实点击。 |
| 320 Change Set | 长差异由内部容器承担横向和纵向滚动，document 横向溢出为 0；从打开后的实际焦点执行 Shift+Tab/Tab，逐个确认“放弃本次编辑”“返回修改”“确认并执行”可达，再按 Escape 关闭。 |
| 嵌套离开提示 | Change Set 之上的 LeaveDialog 打开时，下层为 inert；仅顶层处理 Shift+Tab、Tab、Escape，关闭后焦点回到 Change Set，页面仍保持滚动锁。 |
| CR 拒绝与恢复 | 页面提交包含 CRLF 与独立 CR 的原始草稿，真实 MySQL 以 400 拒绝另一非法字段；HTTP content 与草稿的 UTF-8 HEX 相等。返回修改后仍处于 CR 保护，用户显式转换 LF 才解锁编辑。 |
| 390 未知写入 | Playwright 先让 `route.fetch` 完成真实 201/MySQL commit，再把页面收到的响应替换为 503。页面只读锁定，长 snapshot 自有溢出，三个底部动作逐个可达；请求切片恰有一次写入，SQL 只读核对恰有一行，清理后为 0。 |

窄屏截图随仓库保存，分别代表三种引擎和三种长内容表面：

- [Chromium 320px 长 Drawer](2026-09-07-reliability-stage5-chromium-drawer-320.png)
- [Firefox 320px Change Set](2026-09-07-reliability-stage5-firefox-change-set-320.png)
- [WebKit 390px Write Recovery](2026-09-07-reliability-stage5-webkit-write-recovery-390.png)

## Runner 与复现

新套件由现有独占 runner 启动 production Web、Bearer Admin 与唯一 MySQL 8.4 容器。可指定单个或多个引擎：

```bash
RCC_E2E_SUITE=browser-accessibility \
RCC_E2E_ENGINES=chromium,firefox,webkit \
RCC_E2E_ARTIFACTS=/absolute/new-empty-directory \
scripts/browser-acceptance.sh
```

`RCC_E2E_ENGINES` 仅接受 `chromium`、`firefox`、`webkit`。本地没有设置时，默认 `all` 和定向套件仍只安装、执行 Chromium，便于快速定位；显式设置时，定向套件安装所列引擎，`all` 安装 Chromium 与所列引擎并去重。Linux CI 已设置三引擎，并由 Playwright 官方 installer 的 `--with-deps` 同时安装系统依赖。每个引擎写入自己的结果、HTTP 证据、日志和截图目录。

已有导航或规则套件也可分别定位，不必启动其余业务套件：

```bash
RCC_E2E_SUITE=unsaved-changes scripts/browser-acceptance.sh
RCC_E2E_SUITE=rule-clarity scripts/browser-acceptance.sh
```

最终 Chromium 证据来自 `stage5-cross-engine-final/browser-accessibility/chromium/result.json`。同一组合运行随后因 Firefox 不接受原 `ClipboardEvent` 构造参数而停止；Chromium 自身的 7 项结果已完整写入且 `ok:true`。这是跨引擎测试夹具问题，不是产品失败。粘贴夹具改用普通 paste Event 并只读注入 DataTransfer 后，`stage5-firefox-webkit-final/` 于 08:23:02Z–08:23:32Z 完成，Firefox 与 WebKit 各 7 项通过，runner 状态 0、`cleanup verified:true`。

## 可验证边界

- WebKit 结果仅代表 Playwright WebKit `26.5`，没有安装或操作 Safari，不能表述为 Safari 发行版覆盖。
- CR 用例使用合成 paste Event 和注入的 DataTransfer，以绕开浏览器构造器差异并验证应用的 selection/raw state 逻辑；没有访问操作系统剪贴板。
- 未知写入使用 API 响应故障注入：真实数据库已提交 201，页面才收到替换后的 503。SQL 核对为真实 MySQL 只读查询；它不是网络设备级断线。
- Firefox/WebKit 的 beforeunload 探针仅分发可取消的浏览器事件，未宣称看到原生用户代理对话框。既有 Chromium `unsaved-changes` 套件另有原生 prompt 观察；本阶段保留该保护，没有为跨引擎通过而删除它。
- 320/390 是直接设置的 CSS pixel 视口宽度，覆盖窄屏重排和 200% 对应宽度目标；没有实际改变浏览器 UI zoom，因此不宣称做过原生 200% 缩放操作。

机器可读的红绿摘要、版本、数值与上述边界保存在[阶段 5 portable JSON](2026-09-07-reliability-stage5-browser-accessibility.json)。完整动态端口、HTTP 请求、失败页面、所有九张截图、运行时间和清理记录留在 worktree 同级 `.records-reliability-20260907`，仓库内证据不包含 Bearer Token 或数据库密码。

本机 macOS 三引擎 21 组已经完成。CI 配置已将默认 `all` 扩为 Chromium、Firefox、WebKit；本阶段报告写入时，修改后的 Linux 三引擎 job 尚未运行，因此不把本机结果表述成 Linux 跨引擎结果。

根代理补验：`stage5-unsaved-baseline` 于 08:32:52Z–08:33:31Z 完成原有 14 项真实 Chromium 未保存保护验收，包括原生 reload/tab-close 取消、快速历史导航、重复规则拒绝和 Change Set 草稿保留。`pageErrors` 为空，fixture 完整内容相同、清理确认成功。可用 `RCC_E2E_SUITE=unsaved-changes` 或 `rule-clarity` 单独复验原有套件。

完整共享工作树的 Web 回归为 22 文件 / 175 项及 typecheck 通过，日志 `stage5-root-web-all.log`。其中 2 项属于根代理尚未提交的主键故障恢复补审；本阶段提交不包含这 2 项，不能将 175 计为本阶段提交自身的 CI 统计。共享 modal 的 9 项定向回归均包含在本阶段提交中。

最终定向组件回归为 1 文件、9 项全部通过。`bash -n scripts/browser-acceptance.sh`、新 E2E 的 `node --check`、portable JSON 解析、CI YAML 解析与 `git diff --check` 均通过。完整 Web 树与原 14 项未保存导航基线由父代理在最终共享树上汇总，避免把同时进行的后端补审用例数写成本阶段提交自身的固定计数。

首次 Linux 三引擎 CI `34101408974` 中，原有套件、复杂字段 18 项以及 Chromium/Firefox 各 7 项均通过；WebKit 完成前两项后，在 320px Drawer 发现标题关闭按钮、textarea 和“查看 Change Set”操作的右边缘被视口裁切。仓库内保留了[Linux WebKit 红灯截图](2026-09-07-reliability-stage5-linux-webkit-drawer-320-red.png)；result 和完整 artifact 位于 `../../../.records-reliability-20260907/stage5-linux-failure/`，artifact `10010788674` 的 SHA-256 为 `02de9180e41a47eee3561cd72a4081b63766b849ba38df13a1a048b81a64b7d0`。根因是 Drawer 只有行轨道，隐式列的 min-content 可被长标题和不换行按钮撑出固定视口；现改为 `minmax(0, 1fr)` 显式列，并允许 header/body/footer 收缩、长标题换行、关闭图标保持尺寸。窄屏 footer 操作另有可收缩换行的 120px 基准，并移除关闭操作的自动左边距。验收记录 Drawer/header/body/footer 的矩形与 scrollWidth、textarea 矩形、Drawer 两个操作和 Change Set 三个操作的矩形，逐个滚入视口并确认 document 无横向溢出，再真实点击进入下一层。修复后的 Linux 结果须以后续 CI 为准。

修复后定向 Playwright WebKit 真实 MySQL 回路 7/7 通过，证据为 `stage5-linux-drawer-fix-webkit-green/` 和[320px 绿灯截图](2026-09-07-reliability-stage5-webkit-drawer-320-green.png)。Drawer、header、body、footer 的矩形均为 `0..320`，四者 `clientWidth=scrollWidth=320`；textarea 为 `18..302`，两个 Drawer 操作为 `18..156` 与 `164..302`，Change Set 三个操作全部位于 `28..308.8`，document 横向溢出为 0。真实 wheel 从 890 到 1250，实际点击继续进入 Change Set、数据库拒绝与未知结果路径；postcheck 和资源清理通过。该绿灯来自 macOS WebKit，Linux 修复结果仍须以后续 CI 为准。
