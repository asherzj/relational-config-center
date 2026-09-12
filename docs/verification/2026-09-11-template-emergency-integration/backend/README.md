# #104 与既有通知、角色改造的后端集成验证

在 `/private/tmp/rcc-template-notification-integration` 验证正常合并中的实际工作区：HEAD `e7e19b022fc606c137fdf8df1899caf9a5702bb1`，包含正式 #98、#103 与 #97，再接入 #104 `f545adf24aadd34ee39afb2643a4819aee54c986`。未把 HEAD 名称当作未提交合并内容；每批独立 `*-input.json` 固定启动时全部 Admin 文件的 SHA256。

范围为 #104 的 38 个唯一顶层 HTTP 用例、既有 ReleaseNotifications / NotificationCenter 12 项及两项 main 边界：`TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish`、`TestReleaseHistorySurvivesExecutableRestartAndExternalChanges`。准确名称和全锚定命令位于 `01-http-matrix.json`，没有运行全仓或独立 Schema 迁移矩阵，也没有提前实现 #105 的通知及回滚模板行为。

## 实际结果

| 检查 | 结果与证据 |
| --- | --- |
| 52 项真实 HTTP / MySQL 原批 | `01-http-matrix.log`：51 PASS、1 FAIL，exit=1，540.33 秒；原失败不覆盖。 |
| 四个 Go 包的常规合同检查 | `02-go-contracts.log`：application、domain、http、mysql 全部 PASS；前三者中的 application/domain 及 mysql 使用 Go 有效测试缓存，http 实际执行 3.402 秒。 |
| Admin integration/browser 编译 | `03-admin-compile.log` PASS；`-run '^$'`，不声称执行了全部套件。 |
| 历史用例原样独立复现 | `04-history-isolated-red.log`：同一 `rcc_release_requests` 整表快照失败，exit=1。 |
| 实际 SQL 差异诊断 | `05-history-sql-delta-red.log`：旧行原字节全部一致，唯一新增目录请求身份/结果可核对；保留旧断言，实际仍 FAIL。 |
| 最窄夹具修复 | `06-history-verified-append-green.log`：同一历史用例 PASS，exit=0，20.03 秒。 |

因此当前有效结果是 **52 个唯一顶层 HTTP/MySQL 用例全部 PASS**，逐项映射见 `effective-results.json`。没有将子测试或独立重跑重复计数，没有将原批写成全绿。

所有批次记录 `TESTCONTAINERS_RYUK_DISABLED=true`、`DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`GOCACHE=/private/tmp/rcc-go-cache`，HTTP 测试使用各自新建的 `mysql:8.4` Testcontainer，逐一运行。每批运行期间的 Admin 输入均没有变化。

## 历史夹具修正及 SQL 证据

原历史测试在保存全表快照后，通过正式 HTTP 执行表规则停用。接入表规则版本和持久幂等结果后，该操作合法地向共享 `rcc_release_requests` 追加一条结果，原来的“从停用前起整表绝不增加”断言因此失败。

`05` 的实际 SQL 结果显示：排除完整主键 `(actor_id=本次管理员, operation='table-policy:disable', request_key='history-table-disable')` 后，**所有列和所有既有行**的排序字节串与原快照完全相等。精确主键查询同时取得唯一新增行，日志保留实际 actor、operation、key、digest、完整持久结果及公开 HTTP 响应；目标均为 `history_items`，版本 `3`，状态禁用，修改人对应本次管理员。

修复只改 `admin/cmd/admin/release_history_delivery_integration_test.go`。先断言所有旧行未变且没有其他新行，再解析并验证该新增持久结果与本次 HTTP 停用响应的目标、版本、启停状态及人员归属；此后把含该唯一合法追加的完整表纳入原有重启、迁移、读取和拒绝写入后的全表不可变检查。没有忽略整张表或整类操作，未删除任何原业务、版本、执行、授权、通知及历史断言。差异见 `fixture-change.patch`。

`final-source-applicability.json` 确认原批之后只有这个用例的局部夹具发生变化，最终全部 Admin 字节与 `06` 输入匹配；其余 51 项原批通过结果继续有效。四个合同包未受夹具改动影响；`03` 的 browser 编译路径未改，修正后的夹具已经在 `06` 实际编译并执行。

最后一次只读 Docker 检查见 `cleanup.txt`：仅保留运行前已有的 preview Admin 和部署 MySQL 两容器；全部本轮测试容器已经正常终止，随后将 DB 资源归还 root。此帮助任务未改生产代码、Schema 或前端，未暂存或提交。整体集成、真实浏览器、双轴评审和提交门禁由 root 继续完成。
