# Standards 增量复审 10

固定差异：`git diff 2738eaf3507a484ee188cf96cc53939352243493 dd0e60fa1862538cc772a81a8b8b6667ed85bc9f`，仅 `web/e2e/release-multitable.cjs`。来源快照 `source-candidate10.json`（727 文件）SHA-256：`faa18c3c6f2d65d27bd82d205f35b582386dc70e1face2919f2a46ebd86e8250`。

## Findings

- 成文规范违规：**0**
- 主观坏味道：**0**

新增 `reloadIdentity` 在每次刷新前建立精确的真实 `GET /api/v1/auth/session` 响应等待，核对 200、永久账号 ID 与完整角色集合；随后要求受保护工作区处于 ready 且可见，并从页面与账号菜单核对用户名和角色文字。四次角色/账号边界复用同一 helper，减少重复，并强化 `web/DESIGN.md` 的账号隔离、当前权限及会话恢复约定。

IndexedDB open 故障仍通过真实失败注入触发；只有预期错误提示未出现时才写诊断，记录当前 URL、调用次数、注入函数和当前页面文字后重新抛出原错误。正常通过路径不产生该诊断，也未删除原错误提示、无业务 PUT、输入保留、无关查询可访问等断言。

9 MiB 输入容量、真实请求正文与持久值、撤权、账号切换、原 key/正文恢复、页面错误收集等既有断言均保留。删除临时全量事件监听没有移除任何业务证据；会话证据改由针对性响应屏障和结构化 `session_checks` 提供。`git diff --check` 无格式问题，生产代码未变化。

## Residual risk

本次未运行测试、数据库或浏览器。协调者已确认 Web7 的 398 项测试、2 项 dev 检查、typecheck 与 build 通过；当前 all34 与 Compose6 串行结果仍在执行，不属于本轴结论，不能预报为通过。
