# 账号能力合并后的可靠性验收

日期：2026-09-07。本记录区分历史检查点和合并后的实际执行结果；最终 CI 结果另见本轮交付记录。

## 合并范围

账号 [MR #44](https://github.com/asherzj/relational-config-center/pull/44) 于 08:58:53Z 合入 `main` 的 `a9f583002fad3b5670cfb9905bf49c38c147d6fd`。可靠性分支此前的 HEAD 是 `175fa25a1f692c348043ae5cbdde4ddab036eee1`。本次合并解决双方交叉修改的 26 个文件，保留以下行为：

- 业务请求通过真实 Cookie 会话及 CSRF 验证，不恢复共享 Bearer、免认证或 FixedOperator。
- 旧账号请求的响应不能进入新账号工作区。切换账号清除缓存和草稿；同账号恢复保留草稿及未知写入锁，重新读取后仍需人工确认，不重放写入。
- 后端在本次 INSERT 的事务内确认可寻址 ID；显式 ID 规范化后再次定位，自动 ID 使用本次驱动 Result。非事务引擎在 INSERT 前拒绝，触发器不得使返回值指向旧行。
- HTTP 路由保留原始路径，在参数层只解码一次；复杂 ID、字面 `%2F` 均经真实账号及 CSRF 验证。
- 抽屉使用原生 `inert`、嵌套焦点与滚动保护；账号恢复隐藏工作区时，不让隐藏抽屉抢走账号页面焦点。

## 已执行检查

| 检查 | 合并后证据 |
| --- | --- |
| 全部 Go 模块 | `make test`、`make build` 通过，包含 `account-maintain` 命令 |
| Admin 单元及 race | `go -C admin test ./...`、`go -C admin test -race ./...` 通过 |
| 完整真实 MySQL integration | 09:26:09Z–09:43:52Z；165 个顶层测试、含子测试共 372 项通过，0 失败、0 测试跳过；`cmd/admin` 1060.897 秒 |
| 账号身份与主键专项 | 8 组真实 MySQL Account 用例通过；2 组真实注册、Cookie、CSRF 与复杂路径测试通过，无跳过 |
| 账号系统浏览器路径 | `make test-browser` 通过，41.752 秒；真实 Chrome 覆盖注册、刷新、重开、会话丢失、同账号草稿恢复、隐藏抽屉键盘隔离、业务写入、退出及存储检查 |
| Web 全量 | 生命周期导航修复后 typecheck 通过；25 个文件、243 项全量测试通过 |
| 复杂字段浏览器专项 | `RCC_E2E_SUITE=complex-fields make test-browser-acceptance` 的 42 项通过，原 fixture 全字段相等、临时资源清理通过 |
| 最终生产构建组合 | 10:00:51Z–10:03:38Z，六套件与三个浏览器共 131 项通过，认证、fixture 和资源清理均通过；见[交付记录](2026-09-07-reliability-final-delivery.md) |

三个没有测试文件的 Go package 会产生 package 级 `skip` 事件；它们不属于上表的测试跳过。完整 integration 为实际执行，不以入口编译代替数据库验证。

账号系统浏览器路径还实际执行了 14 项未保存保护和 6 项规则说明。生产构建 runner 的独立认证检查得到：匿名读取 401、注册 201、认证读取 200、缺 CSRF 查询 403、认证查询 200、退出 204、退出后 401。

## 真实故障与适配

旧浏览器脚本已改为每套件公开注册一个临时账号，在内存中复用 Cookie。页面自行发送 CSRF，脚本不向页面请求注入认证 Header；只有直接 APIRequest 操作显式读取当前 CSRF。账号随专属数据库销毁。

复杂字段验收临时开启 MySQL general log 时，先过滤五张账号表的 SQL，再持久化业务事务轨迹，避免保存会话 token hash。故障注入的 SELECT allowlist 保留这五张账号表，使真实活动报告、限速与会话清理仍可执行，仅禁止目标业务表回读。

真实停库后的只读核对实际收到 `auth_unavailable` 503，响应编号仍在页面展示；恢复后仅重试读取，原始写入不重放。若活动报告导致工作区隐藏，脚本通过真实“重新检查登录状态”入口恢复同账号，不屏蔽活动请求。09:48Z 之前通过的停库样本没有触发工作区隐藏；账号恢复本身由独立系统浏览器路径覆盖。

合并后的中间红灯保留在本地记录中：

- 旧 fixture 未提供账号上下文或使用旧按钮期望。组件测试已匹配实际 Cookie 工作区行为。
- 全量并发时“新增记录”先以 disabled 渲染；旧测试找到按钮就点击，点击被忽略。四处入口改为等待 enabled 后点击。
- 生命周期请求结果未知后，原确认框会关闭并进入只读核对；测试不再要求原按钮仍存在。表分配人工恢复和目录筛选断言等待对应 DOM 结果发布，不能把点击或输入事件完成等同于 React 渲染完成。
- 组合验收另外发现真实导航问题：已结束的激活请求进入只读核对时触发离开提醒，遮挡核对按钮。根因是请求已结束但当前 render 的 pending 尚未更新；修复使用既有的内部结果导航入口，先保留未知结果与目标锁，再完成这一次导航。新增回归先红后绿，4 个文件 41 项定向测试通过；随后 Cookie 真实浏览器 28 项写入恢复全通过，脚本不自动关闭提醒绕过该问题。

## 复验与记录

```sh
pnpm --dir web typecheck
pnpm --dir web test:run
make test
make build
make test-integration
make test-browser
RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance
```

完整运行要求及 Colima 环境变量见 [Web README](../../web/README.md)。本地原始证据位于工作树相邻的 `.records-reliability-20260907/`，包括 `merged-admin-integration-final.jsonl`、`merged-web-final-green-success.log`、`merged-account-browser-system.log` 和 `merged-cookie-complex-final/`；这些路径不是仓库内的可移植链接。合并后的四项 Linux CI 及三引擎 artifact 必须单独核验；早于账号合并的 131 项 macOS 组合和阶段 CI 均不替代最终结果。
