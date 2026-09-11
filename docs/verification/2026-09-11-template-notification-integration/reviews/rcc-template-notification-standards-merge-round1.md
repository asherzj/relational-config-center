# 模板／通知依赖合并 Standards 初审

**当前有界合并增量：成文违规 0，主观发现 0。** 固定 ours `e2564842e72e96b6ffb1e86708978b11dd8a651b`、theirs `7334e1103f612b4735992ef63bf1a0b7643b40ae`；正在 merge、未提交。只审 12 个原冲突及 root 新增对接、顺排与小夹具，未重审父提交等同实现，未运行测试、未读 Spec。

核对结果：

- `admin/internal/application/notification_center.go:71`、`release_orders.go:860` 重建审批环境保留已保存发布方式、逐表流程和缺配置表；Summary/Header/HTTP 投影贯通。读取仍通过单一只读快照端口，符合 ADR-0013 及 `docs/architecture.md` 分层约定。
- `web/src/api/release-orders.ts:33` 保留当前通知与流程字段，原请求结果继续排除通知。`ReleaseOrdersPage.tsx:85` 的已读确认要求挂载后成功 GET，写响应不替代当前查询；返回通知来源、缺配置显式保存入口和逐表事实均保留，符合 `web/DESIGN.md` 通知中心、详情及原请求恢复规则。
- `quick_rollback.go:140–151` 同一事务保留停止正向节点、释放目标、记录结果通知与原请求结果。重新准备竞争夹具改用真实连接 ID 锁等待，原目标转移断言保留；历史 Schema 恢复仍固定 6→7，再升级 current。
- 正式 1–8 共 16 个 SQL/manifest 与 theirs 逐字节 MATCH；候选 SQL 仅从 ours 8/9 顺排为9/10。真实生成器读取 SHOW CREATE TABLE，累计28/29表，无前序表丢失；符合 ADR-0024 与 `docs/schema-migrations.md`。

**证据：** readiness 原红 exit1 保留，生成及同例绿已对应原日志；20 个唯一 HTTP 顶层 PASS、包210.115s、exit0，244输入均 MATCH。Web原166/9红与修复两文件13绿支持175有效口径；只有两夹具变化，167输入复核吻合。TS/build通过。

**待项：** 完整新装、升级、恢复及浏览器相交矩阵、最终索引仍待；本结论不等于最终发布验收。接入 #98 已发行9后，应再顺排未部署模板为10/11并重建累计清单；当前中间状态尚未纳入 #98/#104，不报未来冲突为本轮缺陷。

完整主观基线已应用：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest；工具已强制项跳过。

输入：`/tmp/rcc-template-notification-standards-merge-round1-inputs.json`，SHA256 `18b3ad5e763368a5279fad5d28bd82eb121135a64cfe650da5557080155f9225`，含实际源码、双亲、diff、正式迁移及原证据哈希。
