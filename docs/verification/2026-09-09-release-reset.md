# #79 / AC-021：隔离开发／测试旧发布单重置

规格：[字段交互 #71](https://github.com/asherzj/relational-config-center/issues/71)，负责用例：[独立维护 #79](https://github.com/asherzj/relational-config-center/issues/79)。验证于 2026-09-09，MySQL 8.4；每个测试新建独立 Testcontainer，退出销毁。未连接、重置或授权操作用户既有数据库。

## 真实维护进程与发布连续性

测试入口：[release_reset_integration_test.go](../../admin/cmd/admin/release_reset_integration_test.go)。`TestReleaseResetPreservesRecordsAndContinuesPublication` 经正式 HTTP 创建、独立审批、发布和完结一张 ADD，再保留一张已审批未发布的 MODIFY；调用真实构建的 `release-reset` 进程。

| 校验项 | 重置前 | 重置后 |
| --- | --- | --- |
| `rcc_release_orders` | 2 | 0 |
| `rcc_release_requests` | 8，含历史幂等结果 | 0 |
| `rcc_release_targets` | 1 | 0 |
| `rcc_publication_commands` | 1 | 0 |
| `rcc_refresh_notifications` | 1 | 0 |
| 业务行 `kept.label` | `published value` | `published value` |
| 记录有效版本 | 41 | 41 |
| 空 key 维护代际 | 40 | 40 |
| 表发布版本／游标 | 1 / 1 | 1 / 1 |

同一目标重复重置成功。随后通过正式草稿／审批／执行路径再次修改 `kept`，返回 Record Version **42**、Table Version **2**、Command Cursor **2**。这验证实际下一次发布继续分配，业务内容不会被重置操作回滚。

## 拒绝与原子恢复

`TestReleaseResetRefusesUnverifiedTargetsAndSchema` 在固定隔离目标分别验证：缺少目标、production 环境、未声明停写、地址不符、库名不符、实例 UUID 不符均失败；缺表、非事务 MyISAM 表、额外列、删除触发器、另一不可见 schema 的入向级联外键、缺少 PROCESS 元数据权限均拒绝。每次恢复测试结构后核对全部五表的原计数不变。日志不输出数据库密码。

`TestReleaseResetInterruptedTransactionRollsBackAndRetries` 构造历史快照结果与 NULL 在途幂等请求，持有主单行锁，使维护事务的第五条 DELETE 等待；MySQL PROCESSLIST 确认已到达最后 DELETE，前四条删除尚未提交。另一连接仍看到全部原始记录。向真实维护进程发送 SIGINT，进程返回失败并提示保持停写和同目标重跑；释放锁后确认五表计数全部与原值相同。相同参数重跑成功，五表全部为零。

## 执行结果与边界

- TDD red：首条真实 MySQL 用例在正式发布 fixture 建立后因 `cmd/release-reset` 尚不存在而失败；不是依赖环境失败。
- 实现后首条 green 通过；完整 `go test -count=1 -tags=integration ./cmd/admin -run '^TestReleaseReset' -v` **通过**，3 个主测试，完整运行 32.173 秒。
- `make test`、`make build`（含新维护二进制）及 `git diff --check` **通过**；构建产物 `release-reset --help` 输出独立命令用法。
- 原始过程日志保存在任务临时目录，不进入仓库；此文仅保留无凭据的可复核结果。
- 独立命令不加入 HTTP、迁移或启动流程。没有新增持久表、兼容开关或业务执行路径；版本及游标保持原表职责。
- 维护者必须停止并排空全部写入和旧请求重试。`--environment` 与 `--writers-stopped` 是显式声明；地址、库名与实例 UUID 会实际校验，命令不自动证明外部进程已停写。

使用说明与中断后的恢复步骤见 [独立旧发布单重置](../admin-release-reset.md)。全项目最终集成由汇总任务记录，不能将本工单的定向集成结果表述为全套 MySQL 已通过。

## 双轴评审修订

Spec 与 Standards 两轴未发现阻塞问题。Standards 指出维护与发布入口重复了完整的直接 TRIGGER 权限 SQL；已窄提取为同包 `directTriggerGrant`，SQL 保持原样，包括 `partial_revokes`、数据库／表名大小写与账号名称匹配，两个调用者仍分别保留原有错误映射。没有引入新的授权机制。

提取后再次执行 `make test` 通过；真实 MySQL 复验以下 6 个主测试全部通过（73.035 秒）：`TestReleaseFreezeMetadataVisibility`、`TestReleaseFreezeMetadataGrantNameIdentity`、`TestReleaseFreezeMetadataCaseInsensitiveNames`，以及全部 3 个 `TestReleaseReset*`。`git diff --check` 再次通过。
