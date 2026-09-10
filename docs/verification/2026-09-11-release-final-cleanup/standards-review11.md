# Standards 增量复审 11

固定差异：`git diff dd0e60fa1862538cc772a81a8b8b6667ed85bc9f e95958a841a827dddb27f0726eabb5f6c71f0bdb`，仅 `web/e2e/release-multitable.cjs`。来源快照 `source-candidate11.json` SHA-256：`7634288ed8875e6bb7ca3b625df1b500094e05c3179c8c9c4b28df9bb0dc3e9a`。

## Findings

- 成文规范违规：**0**
- 主观坏味道：**0**

注入从给 `indexedDB` 实例赋值改为覆盖 `IDBFactory.prototype.open`，与浏览器通过原型取得 `open` 方法的真实路径一致。测试先等待 `__rccIDBOpenCalls > 0`，再等待“浏览器无法读取原发布请求”，最后同时断言调用次数为正、`indexedDB.open` 与原型方法相同且函数名为 `injectedUnavailableOpen`。这些独立守卫证明故障实际执行，避免原生方法未被替换时误用页面已有状态形成假绿。

原 IndexedDB 故障、错误文案、发布单页面可访问、零业务 PUT、输入保留、无关查询可访问、9 MiB 正文及原请求恢复断言均保留。失败时诊断仍只在 catch 中写出并重新抛错；正常路径把紧凑的调用次数/注入标记写入结构化 evidence。新增测试标记是局部、具体且有退出用途，不构成主观 Primitive Obsession 或 Speculative Generality。`git diff --check` 无格式问题，生产代码未变化。

## Runtime 边界

本次未运行测试、数据库或浏览器。协调者已确认 Web7 的 398 项测试、2 项 dev 检查、typecheck、build，以及 browser24 三引擎 runner/guards 通过；all25 与 Compose11 尚未完成，不能预报为通过。
