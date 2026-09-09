# Goose 迁移管理讨论

日期：2026-09-09。状态：范围、规格和工单拆分已确认；GitHub #67 与 #68～#70 已发布，T1 #68 正在实现，数据库操作仅针对隔离测试库。

## 工作区与来源

- 用户请求：按 flow-feature 引入 Goose，并使用新分支和独立 worktree。
- 分支：`codex/goose-migrations`；开始时的远端主分支为 `94a88058f5fbe6a54d3ad13d7e04c29a78b33b5d`。
- GitHub Issues 是规格与交付工单的来源；Notion PM-050「部署迁移与生产化」是原项目级规划。
- 当前讨论只推进迁移管理；PM-050 中的其他生产化能力不因接入 Goose 自动成为本次范围。

## 已确认的决定

1. **D-001：接管当前基线。**校验存量库已达到当前主分支结构后登记基线；更旧的库先完成现有升级流程。不要求 Goose 直接升级所有历史版本。
2. **D-002：显式升级。**生产由独立维护命令或部署任务运行迁移，Admin 启动只检查版本与结构。
3. **D-003：向前迁移。**首期仅支持向前升级和修复，不提供 down；数据恢复沿用备份流程。
4. **D-004：统一新安装入口。**新库、后续升级及本地 Compose 统一使用 Goose；历史脚本仅保留既有升级职责。
5. **D-005：失败停止与显式恢复。**失败或中断阻止 Admin 就绪和本次部署继续，由维护者核查后显式重试；并发迁移有界等待同一把锁，超时退出。用户以“同意”确认 Q4、Q5。

相关架构决策见 [ADR-0024](../adr/0024-adopt-goose-at-a-verified-control-schema-baseline.md)。

## 已确认的交付材料

- [已发布规格](../specs/goose-migrations/spec.md)：验收入口、AC-001～AC-015、故障承诺与非目标。
- [已确认工单拆分](goose-migration-ticket-plan.md)：三个串行纵向切片、验收归属、阻塞边及旧初始化入口的删除责任。
- 无需运行原型才能回答的未决设计问题；本轮不制作原型。

## 已核查事实

- 当前 fresh schema 和历史升级分属两条入口：[初始化 SQL](../../deploy/mysql/init/001-schema.sql)、[历史迁移说明](../../deploy/mysql/migrations/README.md)。
- [Compose](../../deploy/docker-compose.yml) 在全新 MySQL 数据卷上运行初始化 SQL，随后执行独立的开发 fixture，再启动 Admin；没有 Goose 升级任务。
- [Admin 镜像](../../deploy/Dockerfile.admin) 包含 Admin 与账号维护命令；[旧 Policy 迁移命令](../../admin/cmd/policy-migrate/main.go) 使用独立维护连接、明确历史 Operator，执行回填与收缩。
- 历史迁移不是可以统一重放的链；当前说明明确新安装不重跑旧迁移，007 含一次性 DDL。
- ADR-0001 仍约定 Admin 治理既有业务表，业务表的建表、改表和迁移由部署者负责。
- 当前 Policy Catalog Ready 检查只验证表可以查询，不能充当完整的基线接管校验；既有升级测试已经有列、索引、CHECK 等结构签名与新库/旧库等价比对，可复用其验收方法。
- 001–007 含无保护的建表、改名、加列或删列；006 还含存储过程与客户端 DELIMITER。008 依赖同一会话的变量和 PREPARE/EXECUTE。009–012 可重跑但不会修复错误的既存表定义。
- 新版本账本应处于 `rcc_` 保护范围；记录的结构版本不能替代既有账号恢复、显式管理员授权、记录维护基线推进与发布所需权限。

## 已有验证入口

- [Policy 升级集成测试](../../admin/cmd/admin/policy_integration_test.go)：旧模型回填、收缩和 fresh/upgrade 结构等价。
- [账号交付集成测试](../../admin/cmd/admin/account_delivery_integration_test.go)：历史迁移、维护命令、不完整结构拒绝启动，以及已有账号、会话和业务数据保留。
- [维护连接集成测试](../../admin/cmd/admin/maintenance_connection_integration_test.go)：HTTP 配置或账号结构缺失时仍可运行维护命令。
- [角色升级集成测试](../../admin/cmd/admin/account_roles_integration_test.go)：中断恢复与重跑保留授权历史。
- [发布历史交付集成测试](../../admin/cmd/admin/release_history_delivery_integration_test.go)：升级、重跑和真实重启后的永久历史。
- [开发 fixture 集成测试](../../admin/cmd/admin/local_fixture_integration_test.go)：fixture 幂等及冲突拒绝。

## 官方能力与约束

- Goose 的 Provider 可按实例组织迁移与版本记录，可配置受保护的版本表名称；数据库锁需要显式配置。[Goose Provider 选项（v3.27.3）](https://github.com/pressly/goose/blob/v3.27.3/provider_options.go)
- MySQL 的 CREATE TABLE、ALTER TABLE 等 DDL 会隐式提交，已执行的建表不能由用户事务回滚撤销；迁移状态与中断恢复需要单独设计。[MySQL 8.4 隐式提交说明](https://dev.mysql.com/doc/refman/8.4/en/implicit-commit.html)

## 后续规格需要覆盖的可观察行为

新库初始化、当前库接管、结构不匹配拒绝、正常升级、重复运行、未知或超前版本、数据库不可用、并发锁超时、部分 DDL 成功后的中断、维护者恢复、Admin 就绪门禁以及新安装/升级结果一致。已整理为规格草案中的编号用例，已由用户确认；实现和验收证据按工单推进。

## 已发布工单

- 父规格：[GitHub #67](https://github.com/asherzj/relational-config-center/issues/67)。
- 串行依赖：[#68](https://github.com/asherzj/relational-config-center/issues/68) → [#69](https://github.com/asherzj/relational-config-center/issues/69) → [#70](https://github.com/asherzj/relational-config-center/issues/70)，已建立原生子工单与阻塞关系。
