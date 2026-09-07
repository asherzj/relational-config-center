# 阶段 2：写入故障、恢复与重复提交验收

本阶段以 `7c0266a` 为代码基线，覆盖 Web → 生产构建预览代理 → 开启 Bearer 验证的 Admin → 独立 MySQL 8.4。所有 SQL 故障与临时触发器只作用于 runner 创建的唯一容器；没有修改账号任务数据库、旧服务或生产数据库。Git 提交、Linux CI、MR 与项目管理记录由父代理继续办理。

最终本地验收通过：Web 20 个测试文件、158 项测试；TypeScript 与 production build 通过；真实浏览器组合验收 48 项通过（原有 14 + 6，本阶段 28）。阶段 2 记录的 `pageerror` 与路由故障注入异常均为空。测试 fixture 前后逐字节一致，runner 确认服务、容器和卷清理完成。Linux CI 尚待父代理提交后执行。

## 已复现的问题与行为修正

| 原始红灯 | 证据与原因 | 修正后的行为 |
| --- | --- | --- |
| ADD 在 Admin 返回 201 后丢掉客户端响应，按钮仍允许再次确认 | `route.fetch()` 真正执行请求后 `route.abort()`；独立 SQL count=1，浏览器 POST=1，`确认并执行.disabled=false`。写入错误没有区分明确拒绝和结果未知。 | 保留输入和 Change Set，明确显示“提交结果尚未确认”，锁住该次直接确认。网络、响应契约、5xx 均不被当成确定未写入。 |
| HTTP 响应头已收到，但 body 读取失败逃逸为普通 TypeError | `Response` + 失败 `ReadableStream` 的回归期望 ApiError/status=201/Request ID，原来全丢失。 | 在 `response.text()` 这一真实读取边界包装 ApiError；另用真实 loopback TCP 服务先发送 201/Request ID，再截断 body，验证保留已收到的状态和请求编号。 |
| 同一 React 更新批次连续确认生命周期操作会调用两次 runner | 共享 hook 的最小回路调用 `execute()` 两次，原来调用数=2；不依赖 DOM、网络或自动重试。 | 使用同步 in-flight 引用锁；真实浏览器双击、Enter 和提交中后退时，生命周期请求数与数据库 UPDATE 数均为 1。此处不声称旧版已在真实浏览器重复落库。 |

目录的 Replace Draft、Set Status、Update Metadata 在写后还会读取详情；503 可能出现在已经写入之后。Create Query/Mutation Policy 直接返回创建的 record，没有这一步写后 Get，但仍可能丢失成功响应。目录 `*_policy_not_found` 也可能来自写后回读，保守归为结果未知；只读核对时的已知 404 则是当前不存在这一观察，不用于证明原写入成败。

明确、已知的 4xx 校验拒绝仍可修改后提交。真实 ENUM 非法值返回 400，SQL 写入数=0，修改输入后第二个请求成功，SQL 写入总数=1。未知异常或未知错误契约不冒充确定拒绝。

## 可操作的恢复路径

- 新增、修改、规则草稿、规则名称描述以及表分配：只读核对显示字段和值或记录表，NULL、空字符串及字符串大整数保持可分辨。读取成功后仍不自动解锁；用户明确选择“我已核对，返回修改”，保留草稿，再走原有预览/确认。
- 已知记录 id 的数据核对使用 exact id、page size=1。新增响应丢失且没有 id 时，不推测自增值，仅按查询规则的默认分页读取第一页，并说明分页未找到不能证明失败。max page size=5 的真实用例验证没有硬编码 20 越过规则上限。
- 同一草稿第二次提交仍未知时，旧核对结果不能用于本次解锁；旧 snapshot、成功标志与迟到的其他目标响应均不会显示为当前结果。读取失败后不提供人工恢复按钮；已知资源 404 只允许用户在观察到当前不存在后自行结束核对。
- DELETE 结果未知时不再声称“尚未执行删除”或“取消删除”。核对后的结束动作关闭预览并重新查询。
- 生命周期锁按规则编码保留。A 的未知结果不阻止 B；重新操作 A 仍需核对。明确结束生命周期核对后返回重新读取的规则目录。
- 表启用/停用的明确结束动作关闭旧详情并刷新表目录与数据库表发现。创建/替换的编辑会话保留草稿。目录与表详情终点均增加组件红灯回归，并在真实浏览器确认旧详情已脱离 DOM、没有追加写入。
- 写入成功响应已收到但后续回查失败，沿用原有“已执行，回查未完成”结果。“重新回查”只查询，不重复写入。

## 真实故障矩阵

`web/e2e/write-recovery.cjs` 使用数据库 AFTER INSERT/UPDATE/DELETE 触发器记录本次实际提交的行操作数；事务回滚时触发器记录也回滚。每个场景联合断言浏览器写请求数量、独立 SQL 最终值/记录数，以及实际写入数。触发器只用于一次性验收，不进入应用或初始化 Schema。

| 场景组 | 用例数 | 核对内容 |
| --- | ---: | --- |
| ADD 成功后丢响应、坏 JSON、错误 id 契约、改送 503 | 4 | 每例 HTTP 写请求=1、实际 INSERT=1，草稿/Change Set 保留，只读核对不追加写入 |
| MODIFY / DELETE 成功后丢响应 | 2 | 真实最终值或记录消失，UPDATE/DELETE=1，核对目标 id 为字符串 |
| 两次未知尝试与 page size=5 | 1 | 人工恢复保留原输入；第二次未知必须重新核对；两个不同草稿各新增一行 |
| 成功后回查失败再恢复 | 1 | 已成功结果与回查错误分开，重新回查后显示真实行，写入仍为一次 |
| 双击、Enter、提交中后退及取消原生关闭 | 1 | 请求已实际完成但响应延迟，浏览器留在页面，ADD 只写一次 |
| 真实 ENUM 校验拒绝后改正 | 1 | 第一次 0 写入，改正后第二次请求新增一行 |
| 真实 MySQL 停机/恢复 | 1 | Admin 进程不重启；读取恢复前仍锁定，只重试只读核对，无自动重发写入 |
| Query / Mutation 创建、替换、元数据、激活、弃用、删除 | 12 | 每个真实写入后故障，实际目录写入=1；元数据用写后 503，其余丢响应；只读 GET 不追加写入 |
| 生命周期双击、Enter、提交中后退 | 1 | 一个浏览器写请求、一个数据库 UPDATE |
| 表分配创建、替换、启用、停用 | 4 | 真实分配/状态改变一次，只读 GET 核对与明确人工恢复 |
| **合计** | **28** | 与已有未保存修改 14 项、规则说明 6 项组合运行 |

5xx 故障有两种证据，不能混淆：其一是在真实写入完成之后替换客户端响应，用于证明 Web 的未知结果语义；其二是真实停止 MySQL 再恢复，原写入实际没有提交。没有声称精确复现了 MySQL COMMIT 包丢失，或目录写后 Get 在服务内部发生故障。

## 测试装置的必要修正

原 runner 使用 Docker 自动分配 host port。最小探针证明 stop/start 会让端口从 `127.0.0.1:32774` 变成 `127.0.0.1:32775`，导致 Admin 仍指向旧端点，不能把这种持续 503 算作产品恢复故障。现在先选择随机空闲 loopback 端口，再显式绑定；绑定失败立即失败，重启测试断言端口不变。没有修改 Colima profile 或 prune 共享资源。

恢复测试还使用显式 `browser.newContext()` / `context.newPage()`，避免 `browser.newPage()` 的便捷拥有关系在关闭页面时直接结束 context；等待已发生的 DOM 脱离来判断详情关闭，不以 URL 变化替代 UI 完成。

runner 只接受 `RCC_E2E_SUITE=all` 或 `write-recovery`；未知值在创建服务/容器之前退出 2。写入故障套件具有独立的 360 秒外层上限，现有套件仍默认 180 秒。随机凭据继续只经运行时环境/stdin 流转，并在收尾扫描输出与 Web dist；容器、卷、服务和测试 fixture 均在收尾验证清理。

## 验证命令与证据

```sh
pnpm --dir web typecheck
pnpm --dir web test:run
RCC_E2E_ARTIFACTS=<new-empty-output-directory> make test-browser-acceptance
RCC_E2E_SUITE=write-recovery RCC_E2E_ARTIFACTS=<new-empty-output-directory> make test-browser-acceptance
```

原始三个红灯输出已从持久会话日志提取到 `stage2-original-red-evidence.json`。环境重启使最初 `/private/tmp` 工件消失后，已按保存的工具调用顺序恢复代码，未盲目重放旧服务操作。新证据全部保存在持久目录：

`/Users/asher/Projects/relational-config-center/.worktrees/.records-reliability-20260907/`

最后组合运行目录为 `stage2-all-final`，对应 `run.txt`、三个 suite 的 `result.json`、`fixture-before.tsv` 与 `fixture-after.tsv`。完整 Web 日志为 `stage2-web-final.log`；终点红灯分别为 `stage2-lifecycle-finish-red.log`、`stage2-table-finish-red.log`；未知 suite 的 exit 2 负例为 `stage2-invalid-suite.log`。

仓库内保留[本阶段 28 项请求与 SQL 证据](2026-09-07-reliability-stage2-browser.json)和[成功响应丢失后的恢复界面](2026-09-07-reliability-stage2-response-lost.png)。最终实际停机用例的只读状态序列是 `[503, 503, 503, 503, 200]`；原写请求数为 1、实际写入数为 0，Admin 未重启。

## Linux CI 发现的提交与导航时间窗口

提交 `93e6436` 的 Linux run [`34089085562`](https://github.com/asherzj/relational-config-center/actions/runs/34089085562) 中，Web 和 Go 成功，Browser 在生命周期快速确认后的后退验证失败。失败工件为 `browser-acceptance-34089085562-1`（ID `10006254189`，SHA-256 `c3dc96e5aa6abd8c73a2a54b8203eb036466dbdca5022fada2a4f78f9e8ebba5`），已下载到持久记录目录 `stage2-linux-failure`。截图中详情已经关闭，而写请求仍在等待响应；写请求为 1，未出现离开提示。

根代理使用当前生产构建、真实 Chromium 和延迟响应替身建立了约 3 秒的紧凑回路。同样的原生双击与后退在第三次尝试复现；去掉 Enter 后第二次仍复现，历史索引正常。浏览器记录表明，请求已发出时按钮仍显示“确认激活”，React Query 尚未发布 pending 状态。离开保护此前只读取这一异步状态，所以存在短暂空档。

离开保护现在同时读取各写入入口已有的同步 in-flight 标记，从发出请求的同一事件开始生效；完成回调明确清除提交提示。修复后同一原生浏览器回路 20 次全部拦截，始终只有 1 个写请求。对应组件回归固定 mutation observer 的 pending=false，直接验证请求发出后后退必须被阻止，保留红/绿日志。

阶段 3 的真实 Mutation Draft 删除还发现另一端的时间窗口：DELETE 已返回 204、SQL 已不存在，但成功导航可能被尚未清除的旧 pending 拦住。成功删除现在沿用表单已有的 `afterSave` 导航机制，避免对已完成的操作再次弹出离开确认。回归先固定 observer pending=true 并复现导航失败，再验证成功后直接回到目录。测试路由包装器也改为传递当前 children，避免 `rerender` 后仍使用初始 pending 值而误报通过。

修复后的 `stage2-navigation-recovery` 于 06:24:03Z 完成：28 项真实故障矩阵全部通过，pageErrors 为空，种子内容逐字节一致，资源清理通过。相关生命周期、数据工作流和未保存回归 28 项通过，生产构建通过；共享工作树当时完整 Web 回归为 161 项，其中包含阶段 3 尚未提交的 1 项新增模型测试，不能把这个数量当成此补丁独立提交的测试数量。修复提交后的 Linux CI 继续由父代理核验。

## 边界

本次锁和草稿属于当前 Web 会话，不提供刷新、新标签或其他客户端之间的服务端幂等保证；人工决定再次提交仍有重复写入风险。没有实现持久草稿、幂等平台、并发版本检查或账号功能。当前值相同、不存在或分页缺失都不被称为本次提交的成功/失败证明。

真实浏览器验收使用锁定的 Playwright 1.62.1 / Chromium 151.0.7922.34；其他引擎、长结果与窄屏可达性由后续阶段专项覆盖。TCP body 截断是实际 HTTP 客户端边界测试，不冒充浏览器 → Admin → MySQL 全链路截流。Linux CI 由父代理提交本阶段后再验证，不能用本地通过替代。


后续 CI 排查还修复了确认放弃后立即后退被旧草稿再次拦截的问题，详见[导航时间窗口报告](2026-09-07-reliability-stage2-navigation.md)。该问题已在确定性组件回路和真实 Chromium 复现，修复后相关 29 项与原生回路 20/20 通过。
