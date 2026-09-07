# #33 / 发布单 T2：记录版本验收

固定基点：`1ea7f4441605141b59b30b38b6f92751ab4221c4`（T1）。独立分支 `codex/issue-33-record-versions`，独立工作树 `.worktrees/release-order-t2`。规范为 GitHub #33 全文及 #48 的 AC-007～AC-011；未修改主 checkout 的既有改动。

## 已执行验收

| 用例 | 证据 |
|---|---|
| AC-007 一致快照 | `TestRecordVersionLegacyBaseline` 返回独立 `record_versions:["0"]`；`TestRecordVersionSnapshotAndIndependentResources` 在真实 RR 会话中与另一 HTTP 写入交错，旧数据/旧版本一起保留，新查询取得新数据/新版本。 |
| AC-008 竞争 | `TestRecordVersionRealConcurrentWriters` 真实并发初始化/修改只有一个 200，另一个 409；修改/删除最多一个生效。`TestRecordVersionCompareAndSwap` 旧版本修改和删除均拒绝。不同记录不等待另一记录的持有事务。 |
| AC-009 删除重建 | `TestRecordVersionAddDeleteRecreateAndRollback` 新增 1、删除 2、重建 3；旧版本拒绝，CHECK 失败不改变行与版本；已删除单独返回 404。 |
| AC-010 身份/初始化/回滚 | 真实 `utf8mb4_0900_ai_ci`、`utf8mb4_unicode_ci`、`utf8mb4_0900_bin`、DECIMAL、TIMESTAMP fixture 验证等价主键共享版本与不同主键独立；首次缺失版本竞争、控制存储失败、维护基线超过 2^53、非事务业务表拒绝、迁移重跑及缺表/非事务控制表就绪失败。 |
| AC-011 HTTP/Web | 版本必填 422，旧版本 409；API 契约拒绝缺失、无效、长度不匹配的版本元数据并保持无损字符串。Web 测试验证保留输入和旧差异、只读查看最新值、显式重建后另一次确认。真实浏览器在另一 HTTP 写入后取得 409，按同一路径重新确认，MySQL 最终内容和永久操作人均核实。 |

测试先行证据：初始查询测试因缺少 `record_versions` 失败；版本必填测试因无版本仍返回 200 失败；新增/重建测试因版本仍为 0 失败；控制表就绪测试因缺表仍 Ready 失败；非事务业务表测试发现失败 ADD 残留；Web 冲突测试因 PATCH 未带 `expected_version` 失败。各切片加入实现后转绿。字符/DECIMAL 测试发现非字符 WEIGHT_STRING 输入限制，改用真实存储值的规范字符输出后转绿。

## 验证命令

- `make test`、`make build`：全部 Go 模块通过，包含 Domain/Application/HTTP 依赖方向机器检查。
- `pnpm --dir web test:run`、`pnpm --dir web build`：最终 215 项通过；包含非数字/越界/长度不匹配的版本元数据，以及恢复期间新版本被发现后再次读取失败仍保持确认门禁。
- `go -C admin test -tags=integration ./cmd/admin -run '^TestRecordVersion' -count=1`：本单定向真实 MySQL 验收通过（82.304s）；TIMESTAMP 补充用例通过。
- 既有单行变更定向完整回归通过（198.121s），调用方显式读取版本再写入。
- `make test-browser`：真实 Chrome → 同源 Vite → Admin → MySQL 通过（43.171s），包含原有账号、角色、未保存保护与规则说明。新增记录冲突路径保留输入并显式重建；桌面和 390px 下差异区、错误提示与确认操作可访问，已截图检查。脚本辅助查询初次遗漏 CSRF 被正确拒绝，补齐后全链路通过。
- 完整 `make test-integration` 首轮定位三处测试接入遗漏：历史升级未执行新迁移、访问日志的 DELETE 缺少版本、故障注入账户没有建触发器权限。按正式升级/请求契约修正，并仅让故障注入使用测试容器 owner；应用仍使用原账户。三项定向已通过，完整重跑成功退出（Admin 系统/集成包 1096.965s，HTTP 包与其余所有包均通过）。
- 双轴固定基点及不可变索引树评审后，修正了非法版本文本绕过契约错误、最新值区挤压差异区、登录恢复新版本后再次读取失败解除门禁的问题；新增红测转绿并重跑最终 Web/浏览器。最终复审树 `dec7aeefd679c69631fa1b8a4628c647ea8e346d`：Standards 硬性违规 0、主观建议 0；Spec 未解决发现 0。两位只读子代理独立执行；完整 MySQL 重跑成功后核对日志，无失败项。此后只补齐验证记录及后续接口说明。

Docker 验证使用 `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`，真实 MySQL 8.4；未用 skip 替代依赖验证。

## 架构与临时结构

`rcc_record_versions` 为唯一、正式的记录版本存储；读事务持有数据与版本快照，写事务持有版本推进与业务写入。数据库事务和身份比较没有泄漏到 HTTP/Application；HTTP 消费 Application 合约。对应公用行为契约与保护表真实负向用例已补齐。

旧单行写 HTTP/Web 仍是 T2 临时验收入口，由 T5 #52 删除并把长期验收迁移到发布单入口；记录版本存储和核心并发用例保留。不存在行的已知 ADD 目标身份解析交给 T4 在同一 MySQL 身份能力内补齐。没有新增发布专用版本副本、Feature Flag 或历史清理入口。

ADR-0021、Admin 领域术语、Web README、技术基线及迁移说明已同步。表重建、外部 SQL、数据库/排序规则/算法升级须按 [维护代际](../admin-record-versions.md) 停写并提高整表基线。角色、规则快照、live Schema、Auto Fill 和永久操作人继续由既有规则校验。

## 收尾记录

完整集成、最终 Web 215 项、真实浏览器、构建和双轴复审均通过。提交/远端分支、Notion PM-047 首期记录并发及 PM-020 下一阶段的局部更新，由 #33 完成评论记录并可交叉核验。历史 Revision、Policy 并发和自动合并仍未纳入本期。本单不合并 main、不部署，也不关闭父规格 #48。
