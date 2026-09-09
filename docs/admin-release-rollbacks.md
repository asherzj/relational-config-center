# 原单内整单回滚

当前 PUBLISHER 或 ADMIN 可在普通发布单 `SUCCEEDED`（已发布待完结）期间预览并一次确认整单恢复，不限定原发布人，无需新审批或必填原因。成功后原单变为 `ROLLED_BACK`，保留原申请、原标题、申请人、审批历史和实际发布结果。`COMPLETED` 与 `ROLLED_BACK` 都不能再回滚。首次切换由 [#82](https://github.com/asherzj/relational-config-center/issues/82) 交付现有单表路径；多表、草稿占用和原因补填由 [#81](https://github.com/asherzj/relational-config-center/issues/81) 后续切片交付。

## 预览和一次确认

先调用 `POST /api/v1/release-orders/:id/quick-rollback/preview`，正文为 `{"expected_version":"4"}`。返回原单 ID、版本、表名、全部恢复明细和 `preview_digest`。预览不创建单据、不写业务行或历史；摘要绑定原单版本、恢复内容和当前表结构/变更规则。

审阅后调用 `POST /api/v1/release-orders/:id/quick-rollback`，携带当前会话、CSRF 与 `Idempotency-Key`，正文例如：

```json
{"expected_version":"4","preview_digest":"<刚审阅的摘要>","reason":""}
```

`reason` 可省略或为空，提供时最多 2,000 UTF-8 字节。HTTP 200 返回同一原单，版本推进且状态为 `ROLLED_BACK`。`publication` 保留原发布实际结果，`rollback` 保存倒序恢复的实际结果；`executions` 最多两条成功摘要，类型分别为 `PUBLICATION` 和 `ROLLBACK`。原 `POST /:id/rollback` 不再创建审批回滚草稿，返回状态拒绝，前端不提供该入口。

Web 在同一个确认窗口中展示“当前值 → 恢复值”，原因选填；取消不写入。恢复成功留在原单，可切换“申请差异”“原发布结果”“恢复结果”。正向审批人员与事件保持真实，不为回滚补造审批。

## 恢复规则与保护

| 原发布操作 | 恢复操作 |
| --- | --- |
| ADD | 删除原发布实际生成的 ID |
| MODIFY | 恢复真实发布前的业务值 |
| DELETE | 按删除后保留的记录并发版本恢复原 ID 和业务值 |

恢复按原实际执行顺序倒序进行，仍共用 `CommitPublication` 执行器。唯一值依赖、ENUM 删除身份、SQL NULL、JSON、历史 TIME 时长和自动审计字段继续按真实数据库语义处理。自动操作人/时间使用当前执行人和数据库时间，生成列重新计算。错误索引与恢复预览的倒序明细一致。

预览和执行均核对当前业务行、记录版本、冻结表结构及规则；外部 SQL 即使没有推进 RCC 版本，也不能被静默覆盖。原单必须仍完整持有全部目标，包括真实自增 ID 和已删除身份。回滚期间不先释放目标，成功后才同事务释放。

业务数据、记录/表版本、实际明细结果、成功执行摘要、Command、通知、原单状态/历史与幂等结果在同一 MySQL 事务提交。任一步失败，业务值、原发布、主单和占用均保持原样，不写失败执行记录。执行后的真实业务值不能恢复旧值时，返回 `rollback_restore_mismatch` 并回退整单。完结和回滚竞争只允许一个终态。

同键同内容重推返回原业务结果，同键异内容拒绝；当前权限和账号隔离仍适用。成功发布的旧键在回滚之后仍返回当时的发布结果，当前详情则显示已回滚。当前 Web 暂保留既有请求恢复组件，#86 将按最终规格替换异常交互，#88 清理其剩余外观。原因事后修改由 #87 交付。

## 存储与过渡边界

新安装使用 `deploy/mysql/init/001-schema.sql`，结构升级应用 `014-original-order-executions.sql`。`rcc_release_orders.document` 只保存流程、冻结元数据和摘要；`rcc_release_details` 每项保存所属表、顺序、申请及两次实际结果；`rcc_release_executions` 只保存成功执行摘要，不嵌入 commands。不存在独立回滚单或执行结果明细表。

Command 与通知都有 `execution_id`；执行身份由原单号和成功类型组成，通知主键为 `(execution_id,table_name)`，同一原单的两次执行不会覆盖。通知状态仍为 `NOT_CONNECTED`，不代表下游已收到。

T1 暂保留单表 `table_name` / `frozen`、`items` 和 `PublicationResult` 聚合读取外观，全部由新表组装，无旧数据读取或双写。详细退出责任见 [T1 过渡清单](design-notes/multitable-release-tickets/t1-transitions.md)。不迁移旧发布单内容，也不自动清理环境数据。
