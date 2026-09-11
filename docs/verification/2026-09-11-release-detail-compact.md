# 统一变更入口与发布单详情验收

## 范围与版本

- 基线：远程 main `56dff5360e94d904ab85c17cea5cc575919a29b1`。
- 隔离分支：`codex/swap-config-navigation`；工作树 `/private/tmp/rcc-swap-config-navigation`。
- 需求与验收编号：[发布单详情紧凑布局](../design-notes/release-detail-compact.md)。共享规则：[Web 设计规范](../../web/DESIGN.md)。
- 导航顺序、统一入口名称、发布单紧凑页头/信息/操作/流程条/审阅、未变化字段中性背景均在本次范围内；没有改变路由、接口或发布状态机。

## 工程检查

| 检查 | 实际结果 |
| --- | --- |
| 完整 Web 测试 | 38 文件、420 项中 419 通过；`CombinedQueryForm` 的 100/101 集合值容量测试在全并行下超过原有 20 秒限制。保留该次失败结果。 |
| 超时文件单独复测 | `pnpm exec vitest run src/features/managed-data/CombinedQueryForm.test.tsx --maxWorkers=1`，13 项通过，12.32 秒；没有修改测试超时或产品容量逻辑。 |
| 取消焦点回归 | 新断言复现“关闭取消抽屉后不回到更多按钮”；稳定触发按钮及菜单关闭聚焦修复后通过。 |
| 修复后受影响回归 | 详情与人员组件 74 项通过；包含名称/完整 ID 展开、复制失败、生命周期、权限、未决请求与重放等既有路径。 |
| 类型与生产构建 | `pnpm build`（包含 `tsc --noEmit`）通过。 |
| 浏览器脚本适配 | 12 个受影响脚本 `node --check` 通过；已同步新文案、状态徽标、更多菜单与折叠信息定位。 |
| 变更检查 | `git diff --check` 通过。 |

全量与定向测试证据分别位于 `/private/tmp/rcc-compact-web-tests.log`、`/private/tmp/rcc-compact-query-recheck.log`、`/private/tmp/rcc-compact-delivery-tests.log`；焦点修复前后见 `/private/tmp/rcc-cancel-focus-before.log` 与 `/private/tmp/rcc-cancel-focus-fixed.log`。完整集合通过证据来自首轮加针对性复验，不把首轮标为全绿。

## 真实浏览器

使用仓库隔离 MySQL、临时账号、真实 Admin 与生产 Web 构建；通过临时脚本只运行本次相关路径。

第一轮已验证 Chromium 151 在 1440px 桌面、390px 手机上的导航顺序及统一入口名称、双向跳转/选中、手机导航关闭、详情唯一主标题/状态/页头操作、基本信息与人员 ID 折叠、取消确认、当前草稿追加链接。

真实 MODIFY 草稿中：`category` 提交同值、`note` 同为 NULL、`state` 未提交的两列计算背景均透明；`name` 的原值为浅红、申请值为浅绿。页面无横向溢出或浏览器异常。业务夹具前后校验一致，临时进程、容器与卷清理成功。原证据位于 `/private/tmp/rcc-final-detail-browser-passed/`。

最终布局补验：Chromium、Firefox、WebKit 在同一隔离环境中均验证桌面姓名单行、取消抽屉关闭回焦点、手机真实 SUBMIT 事件的人员与时间。390px 手机流程条内部宽度和滚动宽度均为 356px，没有横向溢出。每个引擎创建的单据均已取消，临时资源清理成功。WebKit 的键盘验收从程序定位“更多操作”开始，之后激活、关闭、Esc 使用真实按键；没有宣称验证其全页 Tab 遍历。

截图：[桌面详情](2026-09-11-release-detail-compact-browser/desktop-detail.png)、[手机真实阶段人员](2026-09-11-release-detail-compact-browser/mobile-progress.png)、[完整字段差异](2026-09-11-release-detail-compact-browser/unchanged-fields.png)。三个引擎的布局结果保存在同目录 `*-layout.json`。截图后的菜单处理修复没有改变布局，复用此批视觉证据。

连续菜单重开曾另行暴露 Esc 不回焦点的问题：自定义 `onCloseAutoFocus.preventDefault()` 跳过了 Radix 内部关闭状态清理。移除该覆盖，只在打开取消抽屉前聚焦稳定触发按钮，让现有 Drawer 保护焦点。完整连续链测试先失败后通过，证据 `/private/tmp/rcc-cancel-reopen.log`、`/private/tmp/rcc-cancel-reopen-fixed.log`。真实浏览器进一步定位到退场动画保留了 FocusScope：在动画结束前立即重开时不再执行初始聚焦。仅对详情更多菜单关闭状态禁用退场动画，使其立即移除；打开动画及其他菜单行为不变。测试不增加等待或强制焦点来绕过。最终连续链在 Chromium、Firefox、WebKit 全部通过：关闭取消抽屉回“更多操作”→立即 Enter 重开并聚焦取消菜单项→Esc 回触发按钮→再次 Enter 进入取消→Esc 回触发按钮，全程无刷新、额外等待或中途焦点修补。三个引擎均确认加载最终资源 `index-2_A9Khg5.js` / `index-D-AD8zfN.css`；结果保存在截图目录 `*-menu.json`。单据取消、夹具一致性及临时资源清理全部通过，此前连续重开问题已关闭。

## 评审与视觉检查

- Standards：发现 1 项菜单转取消抽屉后的焦点恢复问题，已用先失败后通过测试修复并复核关闭；无其他重要发现。
- Spec：功能无偏差；手机带真实阶段人员的布局证据已补齐。
- 已核对业务标题与状态层级、主次按钮、折叠信息密度、紧凑四阶段、相同值中性背景、差异红绿背景、窄屏表格局部滚动。首轮截图中的英文人员名称尾字换行通过移除按钮负外边距修复；轻量真实组件与最终三引擎均确认姓名单行、长姓名换行和 UUID 完整查看/复制。

历史验证文档保留原时点文案。没有执行生产发布或合并；本次提供隔离分支上的本地实现与证据。
