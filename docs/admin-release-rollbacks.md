# 发布回滚

## 审批回滚（T7 / #54）

当前 EDITOR 可从 COMPLETED 普通发布单申请反向草稿；SUCCEEDED 表示已发布待完结，可由当前 PUBLISHER/ADMIN 快速回滚，或明确完结后再申请普通回滚。成功反向结果虽然也是 COMPLETED，但不能再次申请回滚。调用 `POST /api/v1/release-orders/:id/rollback`，携带 `Idempotency-Key` 和 `{"expected_version":"5","reason":"恢复原业务配置"}`；原因必填且最多 2,000 UTF-8 字节。返回 201 及新的 DRAFT，申请不会写业务行或取得在途目标。反向单仍由申请人提交、另一位 APPROVER 审批、PUBLISHER 执行；ADMIN 也不能自批。

反向草稿标题由服务器生成“回滚：原标题”，按 Unicode 字符截断至 100 个字符，并随只读回滚意图固定。原标题和永久发布单 ID 都保留在原单；标题只帮助人在列表与详情中识别意图。

原单的 `rollback_order_id` 指向最新反向单，`rollback_pending` 表示该申请仍在途；反向单的 `rollback_of_id` 指向原单。无回滚关联时可省略这些字段，省略 pending 等价于 false。列表仍是有界摘要，包含这些关联字段；完整差异与历史通过详情读取。`history.related_order_id` 关联每次申请、取消、拒绝与成功；旧申请及原发布结果不会删除。

## 恢复规则

| 原发布明细 | 反向明细 |
| --- | --- |
| ADD | DELETE 原 Command 的实际 id，以原发布后的 Record Version 为基线 |
| MODIFY | MODIFY 恢复原 Command.before 的业务值，以原发布后的 Record Version 为基线 |
| DELETE | ADD 恢复原 id 和业务值，以删除后保留的墓碑 Record Version 为基线 |

反向明细按原实际 Command 的逆序执行：例如原单先 DELETE 旧行再 ADD 新行复用唯一值，回滚先 DELETE 新行再 ADD 旧行。差异、结果、错误明细索引都采用反向单自己的顺序；原单 Command 和历史顺序保持原样。

来源始终是已验证格式、Schema 摘要和校验和的真实 Command，不是客户端上传的 before。当前 Schema/Policy、字段类型、操作权限、在途目标与原记录版本仍须通过准备、提交及执行检查。后续修改、删除重建、维护版本推进及新约束都可能拒绝恢复。已删除 ENUM 主键只在原主键定义和字符比较语义仍相同时使用原删除保存的比较身份，普通缺行 ENUM ADD 的能力没有扩张。

原来或当前的自动操作人/时间及原来的生成列不作为普通恢复输入；当前自动字段使用本次执行人的永久 Account ID 和数据库时间，当前生成列重新计算。历史 TIME 可恢复 MySQL 保存的带符号时长，普通用户上传时间的现有格式不变。无法按当前字段类型表达的历史值会明确拒绝，不截断或修改基线。

允许的目标行 BEFORE 触发器仍需满足正式发布能力检查。执行后还逐项比较实际最终业务值与待恢复的真实旧值；如 `SET NEW.quantity=NEW.quantity+1` 导致恢复 10 后实际仍是 11，返回 `422 rollback_restore_mismatch` 并回退整个事务，不虚标原单已回滚。当前数据库类型转换导致无法完整恢复时同样拒绝。

反向草稿内容固定，服务端拒绝 `edit` / `copy`（`422 rollback_locked`），界面也不提供编辑、添加明细、复制或选择反向草稿作为批量追加目标。普通 preview 只能展示另一个普通申请的当前值，不能改变已保存回滚单的原版本约束。取消或拒绝后从原单重新申请回滚，仍以原发布后版本为基线，不继承审批。

## 一致性、重试和容量

同原单至多一张在途有效反向申请（`409 rollback_conflict`）；原单版本仍控制申请竞争。原单行锁协调申请与反向动作；反向动作先锁原单、再锁反向单，后续提交保持既有业务表/目标/版本锁序。获取不可变原单指针在事务外完成，不提前建立执行事务的旧一致性快照。

执行成功在同一个 MySQL 事务中保存：反向业务行、记录版本、Command、Table Version、反向单 COMPLETED/EXECUTE、原单 ROLLED_BACK/关联历史、目标释放、通知与原请求成功结果。任意步骤失败，原单仍 COMPLETED，反向单仍 APPROVED 且保留占用。取消和拒绝也与原关联的释放同事务提交。

所有动作沿用账号、操作、请求键和规范请求摘要。同键同内容返回原业务结果；同键改内容为 `409 idempotency_conflict`。已回滚原单的旧 execute 键仍返回原来的 SUCCEEDED 快照，当前详情则显示 ROLLED_BACK。Web 成功后重新读取当前详情和关联单，不把旧结果当作当前状态。未知结果保留原键、原单号、期望版本和理由，刷新/重新登录后按账号隔离恢复；明确冲突后保留原意，先看当前状态再确认重建。反向提交恢复不会调用普通 preview 来更新记录基线。

一单仍为同表 1～1,000 项，整单一次提交，明细/结果每页最多显示 20 项。HTTP 1 MiB、普通提交字段 64 KiB 和完整文档/结果 8 MiB 门禁不变。回滚动作没有重新上传全部历史值，因此服务器生成的历史字段不套用上传的 64 KiB 门禁，但仍受当前类型、完整结果及默认正式 4 秒事务期限约束。超过最终文档预算仍整单拒绝。

普通 SUCCEEDED 保留 64 KiB 的后续动作空间；人工完结可消耗其中 4 KiB，无关联 COMPLETED 仍保留 60 KiB。申请关联可消耗其中一部分，但 pending 原单始终保留 4 KiB 终止事件空间和 1 KiB HTTP 空间；取消、拒绝或最终回滚可消耗终止余量，即使取消后的原单仍为 COMPLETED。新的申请必须再次通过 pending 门禁，不承诺无限历史容量。不会接受一笔因新增关联占满空间而无法取消/拒绝的申请。

无新增 Schema 迁移、临时恢复 SQL 或兼容开关。通知仍只持久化 `NOT_CONNECTED`，没有 worker 或 Server/Client 投递。本单证据见 [T7 验收](verification/2026-09-08-approved-rollback.md)。


## 免审批快速回滚（T4 / #63）

当前 PUBLISHER 或 ADMIN 可以对普通 SUCCEEDED 单执行整单快速回滚，不限定原发布人。普通单完结后、任何反向结果、已回滚单都不能使用此动作。EDITOR、APPROVER 和 VIEWER 不因此取得发布权限；每次预览、执行及原请求恢复仍验证当前账号权限。

先调用 `POST /api/v1/release-orders/:id/quick-rollback/preview`，正文为 `{"expected_version":"4"}`。返回 `order_id`、`expected_version`、`table_name`、`items` 和 `preview_digest`，明细顺序和含义沿用上面的恢复规则。此接口不创建单据、不写业务数据或历史；预览只展示整单当前值与恢复值，不能选择部分明细。摘要绑定本次原单版本、实际恢复明细与当前 Schema/规则执行语义，不能拿其他单或其他恢复内容的摘要执行。

审阅后调用 `POST /api/v1/release-orders/:id/quick-rollback`，带 `Idempotency-Key`，正文包含 `expected_version`、原 `preview_digest` 和必填 `reason`（最多 2,000 UTF-8 字节）。HTTP 200 返回新的 COMPLETED 回滚结果，标题为“回滚：原标题”。实际执行人写入结果的 `applicant_id`、`publication.publisher_id` 和唯一 `QUICK_ROLLBACK` 历史；原因与双向关联同时保留。服务端不创建或补造 APPROVE 事件。原单变为 ROLLED_BACK，保留原发布结果并新增关联历史；反向结果不能再次完结、回滚、编辑或复制。

预览和执行都读取锁定的实际业务行，与原发布 Command.final 的规范行校验和比较，并核对记录版本和冻结的 Schema/规则语义。即使外部 SQL 未递增 RCC Record Version，只要实际当前值已变化，就以 `409 record_version_conflict` 拒绝；当前表结构或执行语义变化以相应错误拒绝。执行重新核对预览摘要，旧摘要不能悄悄改成新意图。目标占用必须完整且仍由原单持有；缺失或其他单占用都不能抢占或补建。

执行从锁定原单到业务恢复完成始终使用原单已有的 Active Targets，没有先释放再重新占用的窗口。反向业务行、记录和表版本、Command、通知、两张单据及历史、原目标释放、幂等成功结果都在同一事务内提交。实际最终业务值必须等于待恢复旧值，否则整个事务回退。完结与快速回滚、不同请求的快速回滚、同请求重试竞争时至多产生一个终止结果；失败不留下部分恢复、额外单据或丢失占用。

Web 先展示整单“当前值 → 恢复值”，读取失败或原因空白时不能确认。取消不发写请求。网络中断或响应丢失后保留原单版本、预览摘要、原因及请求键；同单的完结和新快速回滚暂停，跨刷新仍以原键和原正文恢复已提交结果，不再次预览或重复执行。明确版本、状态、目标或语义冲突后保留原意，只有主动读取最新状态并重新审阅恢复预览，才能确认使用新键重建；若原单已完结，则不提供快速回滚重建。

快速回滚消耗 SUCCEEDED 已预留的 64 KiB 后续动作空间，保留完整文档/结果 8 MiB 与默认 4 秒事务边界。合法已发布单不会因为新增终止历史而无法收尾；接近容量上限的终止和原键恢复均有真实 MySQL 验证。Server/Client 分发仍为 NOT_CONNECTED。验收见 [T4 证据](verification/2026-09-09-quick-rollback.md)。
