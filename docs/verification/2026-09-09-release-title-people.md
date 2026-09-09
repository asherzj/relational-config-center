# #61 发布单标题与人员信息验收记录

固定基点：`f513859c41e643e2b4d6ee37684dae67ad920e53`。本记录覆盖 AC-010 和 AC-011 的标题、永久人员身份、当前显示名以及 Web 接线。

后端公开 HTTP 验收在真实 MySQL 8.4 上验证：创建标题写入并可从详情和有界列表摘要重读；空白和 101 个 Unicode 字符拒绝，100 个 emoji 接受；标题与明细使用同一草稿版本 CAS，过期版本不能覆盖新标题或明细，提交后两者一起冻结；复制继承可编辑原标题；普通回滚生成并按 Unicode 字符截断“回滚：原标题”。普通 VIEWER 可通过单据专用 `GET /api/v1/release-orders/:id/people` 解析当前姓名，同时无权读取账号角色列表；申请、审批、发布和历史继续保存永久 Account ID。

Web 组件验收覆盖数据页单条及批量入口的默认标题、空标题禁用、已有草稿合并保留原标题、失败后使用原请求键和标题重试、草稿编辑 100/101 emoji 边界、列表摘要、详情标题，以及申请人、历史 actor 和发布人的当前姓名、首字头像与永久 ID 回退。标题字段以 Unicode 码点判断并暴露 `required`、`aria-invalid` 与字段级错误关联；人员姓名读取从成功转为失败时清除缓存姓名、回退永久 ID，并显示稳定错误码、Request ID 和独立重试入口。发布写入成功会同时刷新单据与人员缓存，使首次参与审批或发布的人员姓名立即出现。完整 Web 测试为 28 个文件、300 项通过，`typecheck` 和生产构建退出 0；`make test`、`make build` 以及 `pnpm --dir web test:dev` 的 2 项开发时检查也全部通过。

最终 `make test-browser` 由真实浏览器经 Vite 同源代理访问认证 Admin API 和一次性 MySQL：`TestAccountBrowserSystemPath` 112.37 秒，Go 包 116.084 秒，6 个子套件全部通过并正常清理容器。它覆盖标题错误语义、人员姓名 success→failure 回退与重试、首次审批人刷新，以及账号搜索、发布状态同步和原请求恢复。

最终正式 `make test-browser-acceptance` 以 Chromium、Firefox、WebKit 运行完整 `all` 套件，445 秒退出 0 并验证清理。17 个套件共 222 个命名检查全部通过，页面错误为空，初始夹具前后完全一致且数据库 post-check 存在。三个 recovery 流程读取的账号首屏均达到 25 条且有下一页；WebKit 本轮目标账号不在首屏，按完整用户名搜索后唯一命中。另一个隔离边界探针注册 30 个候选并选取 UUID 排序最大的账号，确定性证明首屏 25 条、有下一页、目标不在首屏且搜索后可见。机器可读结果见 [浏览器汇总](2026-09-09-release-title-people-browser-summary.json)、[浏览器运行记录](2026-09-09-release-title-people-browser-run.txt) 和 [总体验收数据](2026-09-09-release-title-people.json)。

浏览器验收保留了以下失败和修复轨迹：

- 第一次截图证据运行在业务脚本前因目标目录不存在报 `ENOENT`；创建目录后以相同产品和断言重跑通过。
- 新标题输入改变了弹窗键盘顺序，Chromium 的旧测试预期先被纠正为实际顺序；WebKit 仍会按原生按钮焦点偏好跳过动作，产品焦点管理随后统一逐项推进顶层弹窗焦点。三引擎无障碍套件最终各 7 项通过。
- 发布单列表返回后会重挂载并清空筛选，数据量增大时旧脚本找不到目标；脚本现在在每次返回后重新提交原筛选。
- 草稿取消曾在写入后等待始终存在的标题，随即 reload 会中止尚未完成的请求；脚本改为等待精确“已取消”状态，再重载检查持久历史。
- 账号角色脚本曾只查默认 25 条首屏；它现在按本次新注册的完整用户名检索并断言唯一命中。`accounts.mjs` 内所有发布写入完成信号也同时检查业务标题和精确状态。

本机证据运行于 macOS arm64。macOS 系统代理不会被 Firefox 自动按 `NO_PROXY` 绕过，最小诊断曾稳定产生 `NS_ERROR_NET_RESET` 且服务端未收到请求；仅访问隔离 loopback fixture 的 Firefox 启动分支显式设置 `network.proxy.type=0` 后稳定通过，未修改系统代理。账号进程重启的 Python/HTTP 定向测试则以进程级 `NO_PROXY=127.0.0.1,localhost` 绕过 loopback 代理。Linux CI 由现有 runner 的 Playwright `--with-deps` 安装路径负责；本记录不把本机 macOS 结果表述为 Linux 实机结果。

按 `web/DESIGN.md` 与 impeccable 的边界、文本折行、移动宽度和身份层级检查了以下真实截图：

- [桌面发布单详情](2026-09-09-release-title-people-desktop.png)：1440px 页面展示自定义标题、申请人当前姓名、头像、完整永久 ID、长字段表及六条历史，阅读顺序清楚且无重叠。
- [390px 审批历史](2026-09-09-release-title-people-mobile.png)：标题、申请/审批/取消人员和永久 ID 在窄屏内正常换行，操作与长明细未产生横向溢出。
- [390px 回滚详情](2026-09-09-release-title-people-rollback-mobile.png)：派生标题、发布人、永久 ID、最终数据库结果及回滚历史完整可读；自动化同时断言文档宽度不超过视口。

完整 MySQL 回归的第一次运行耗时 1900.391 秒，汇总为 494 PASS、3 FAIL（含父级）、0 SKIP，因此不计为通过。一处是跨表批量夹具把顶层标题误放入 `DraftItem`，严格 JSON 解码返回 400；删除明细内误加字段并保留外层标题后，批量边界文件 9 个顶层用例在真实 MySQL 上 81.471 秒通过。另一顶层失败是既有账号进程重启测试继承 macOS HTTP/HTTPS 代理后读取 loopback CSRF 得到 `RemoteDisconnected`；进程级 `NO_PROXY` 诊断转绿，未放宽契约，也未修改 Python `account-session.py`、对应后端集成测试或系统设置。修正后的最终完整回归耗时 2001.928 秒，497 PASS、0 FAIL、0 SKIP；见 [MySQL 汇总](2026-09-09-release-title-people-mysql-summary.json)。

Impeccable 静态 detector 对本单八个 Web 产品文件返回空问题列表。独立双轴复审最终为 Spec 0 个发现、Standards 0 个硬性发现；所有中途 P2 已修复并复审通过。Standards 保留一条已接受的非阻断 P3 建议：`release_orders.go` 与 `publication.go` 的匿名标题/明细/执行摘要结构可在未来出现第三个同类摘要时再共同封装，本单不为此打断已验证的后端冻结。
