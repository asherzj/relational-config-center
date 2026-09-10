# 发布草稿

T3 [#50](https://github.com/asherzj/relational-config-center/issues/50) 提供草稿创建、编辑、查询和取消。T5 已把数据页 Change Set 唯一确认改为“确认并保存草稿”，不会写业务记录、推进记录版本或占用目标。提交、独立审批后由 PUBLISHER [正式执行](design-notes/publication-contract.md)；旧记录写路由已删除，分发仍未接入。T6 [#53](https://github.com/asherzj/relational-config-center/issues/53) 将同一路径扩展为同表 1～1,000 项 ADD/MODIFY/DELETE；单项是集合长度为 1 的情况。

## HTTP

所有接口沿用当前 Cookie、CSRF、同源检查、`Cache-Control: no-store` 和 `request_id`。VIEWER 可查看全部发布历史；创建、预览、修改需当前 EDITOR（ADMIN 包含该能力），修改仅限申请人。取消限当前有编辑能力的申请人或 ADMIN，原因必填。PUBLISHER 不包含 EDITOR。

| 接口 | 承诺 |
| --- | --- |
| `POST /api/v1/release-orders` | 创建唯一 DRAFT，返回 201；标题必填，`Idempotency-Key` 必填 |
| `PUT /api/v1/release-orders/:id` | 整体替换草稿标题与申请值，校验发布单 `expected_version` 和记录基线；返回 200 |
| `POST /api/v1/release-orders/:id/cancel` | `{ "expected_version": "1", "reason": "调整计划" }`，保留已取消历史 |
| `GET /api/v1/release-orders/:id` | 当前单据、全部字段差异、永久申请人及操作历史；不存在返回 404 |
| `GET /api/v1/release-orders/:id/people` | 仅解析本单申请、审批、发布和历史账号 ID 的当前显示名；VIEWER 可读，不授予账号列表或角色权限，找不到的 ID 省略 |
| `GET /api/v1/release-orders` | `table_name`、`applicant_id`、`state`、`id` 精确筛选；`limit` 为 1～100，默认 20；按不可变单号升序、`after` 游标分页，返回摘要 `orders` / `next_cursor`，完整明细通过详情读取 |
| `POST /api/v1/release-orders/preview` | 只读核实当前申请目标，返回 `table_name` / `items`；不保存草稿或幂等记录。用于先查看最新基线，再由用户明确采用 |

草稿输入只接受以下字段，客户端 `before`、差异字段、申请人、操作人及内部记录身份不能上传：

```json
{
  "title": "更新通知模板文案",
  "table_name": "notification_templates",
  "items": [{
    "operation": "MODIFY",
    "id": "7",
    "expected_record_version": "3",
    "content": { "subject": null, "body": "" }
  }]
}
```

`title` 去除首尾空白后必须非空，最多 100 个 Unicode 字符；emoji 等补充平面字符按一个字符计算。所有数据页新建入口预填“表名 配置变更”（超过 100 字符时截断），申请人可在 DRAFT 中修改；提交后标题与明细一起冻结。复制被拒绝发布单时继承原题并保持可编辑。标题不改变永久的 32 位十六进制发布单 ID。

修改已有草稿时还需顶层 `expected_version` 字符串。ADD 的已知 id 只放在 `content.id`，不要同时传明细 `id`；省略自增 id 时，返回 `id: null` 和空记录基线，表示尚无已知身份。首次创建已知 ADD 由服务器读取缺行基线；更新已知 ADD 必须显式提供该目标的记录版本，不能省略以采用新的墓碑/维护基线。MODIFY/DELETE 始终要求明确记录版本，DELETE 不接受内容。

`preview` 返回当前服务器原值、字段类型/可编辑性、缺行墓碑及维护版本。预览可以重新读取陈旧基线，但本身不修改已保存的草稿。界面先展示预览，用户点击“基于最新配置重建”后，后续保存仍携带新基线接受服务器 CAS 检查。换 ADD id 或再次修改输入会使旧预览失效。

内容错误为 422；缺记录版本为 `record_version_required`，陈旧记录版本为 `record_version_conflict` (409)，草稿版本为 `release_version_conflict` (409)，无权为 `permission_denied` (403)，不存在为 `release_not_found` (404)，无效状态为 `release_state_invalid` (422)。不兼容表/规则沿用现有稳定错误码。控制存储不可用返回 `release_unavailable` (503)，提交结果不确定为 `release_result_unknown` (503)。不存在物理删除或自动清理接口。

## 差异与历史

`items.before` 的 JSON null 表示没有原记录；已有行中值为 null 表示 SQL NULL。`content` 缺字段表示未提交，`""` 表示空字符串。每个 `fields` 项保存类型、可编辑/可空属性、`before_state` 与 `proposed_state`，完整包含未变字段：

| 字段状态 | 含义 |
| --- | --- |
| `value` | 字符串原意；空字符串仍为 value。JSON 列的字符串 `"null"` 是 JSON 字面量 null，与 SQL NULL 不同 |
| `sql_null` | SQL NULL |
| `absent` | ADD 没有原记录，或 DELETE 的目标状态不存在 |
| `omitted` | 此字段未提交；不声称数据库默认值已生效 |
| `automatic` | 实际发布时才由 PUBLISHER Account ID / 数据库执行时间填充 |
| `generated` | 数据库生成列；实际最终值待正式发布读取 |

原值、差异类型及历史从持久草稿读取，不依赖之后的实时 Schema/Policy。当前草稿格式沿用受支持列的 JSON String 表示；不支持的 BINARY/BLOB 等列在准备前由规则/Schema 检查拒绝，非法 UTF-8 快照也拒绝，绝不静默替换后保存。T5 的[最终行格式](design-notes/publication-contract.md)可编码二进制固定样例，但实时发布仍保留上述不支持字段的准备前拒绝。

## 幂等与恢复

所有持久写入以永久账号、操作（含目标单号）和 8～64 字符的 `Idempotency-Key` 为范围，规范 JSON 请求摘要与原业务结果一并持久化。对象键顺序不影响摘要。同键同内容重试返回原业务结果，包含原版本，即使该版本已前进；同键改内容返回 `idempotency_conflict` (409)。可执行动作重新按当前身份与当前单据状态计算；原业务结果不是当前详情的替代，界面随后重新读取详情。当前鉴权/所有权不能靠历史幂等结果绕过。

并发相同请求由数据库唯一键收敛；新状态动作锁定单据并校验版本。草稿、历史与幂等结果同一事务提交，失败全部回滚，不保留半完成请求。草稿保存没有业务行写入方法，也不分配在途目标。

Web 在请求发送前将原键、路径和申请内容保存在当前标签页的 sessionStorage，以 Account ID 隔离；Cookie/CSRF 不进入该记录。刷新或重新登录后可查看原申请并使用原请求恢复。未知结果后的 401/403 或一次详情未找到均不能删除原请求或自动换键；切换账号不能重放上一账号请求。结果确定成功后删除临时请求，未发出的普通编辑仍沿用现有未保存保护。

原请求写重试经过服务器去重后，若明确返回 `release_version_conflict`、`record_version_conflict` 或 `release_state_invalid`，说明该标识没有已成功业务结果；页面解除未知状态，但将原申请持久保留为待重建内容；刷新后的恢复卡片仍可查看完整原意，必须先读取最新单据/记录并明确确认重建，才能用新标识保存。鉴权失败或一次详情读取失败不能据此丢弃原标识。

待重建申请也受统一写入入口保护：普通编辑或新建不能覆盖同作用域中持久保留的冲突申请。只有查看最新基线后的明确重建动作可以授权下一次新键写入；取消恢复会明确显示实际取消动作与原原因。


## 存储和后续复用

停写升级时，在 007/008/009 之后运行 `deploy/mysql/migrations/010-release-drafts.sql`；新安装的 Goose 当前迁移 包含同一表定义。迁移可重跑，Ready 检查完整字段、InnoDB 和唯一键。新控制表全部受 `rcc_*` 通用表保护：

- `rcc_release_orders` 保存不可变单号、发布单标题、申请人、状态/版本及完整草稿文档（含操作历史和永久 Account ID）。
- `rcc_release_requests` 保存账号/操作/请求键、SHA-256 摘要与原结果，永久保留。

`application.ReleaseOrderSession` 只公开控制数据写入和规则/记录基线读取，事务由 MySQL `ExecuteReleaseOrder` 拥有。T4 在此基础上增加提交、冻结及目标占用；不能另造记录版本或应用字符串身份。`ReadRecordBaselines` 按同表成批读取，复用 `recordIdentityMetadata` / `recordWeightExpression`，缺行已知 id 通过 live 主键类型转换和同一 MySQL collation 权重解析，读取统一墓碑及整表维护基线。返回的 `RecordKey` 只用于内部持久化/后续占用，不公开给 Web。身份算法/数据库版本改变仍遵循 [记录版本维护流程](admin-record-versions.md)。

`make test-browser` 可通过 `RCC_E2E_OUTPUT=/absolute/path` 保留各脚本的截图证据；默认仍写入测试临时目录。
## 自增主键 0 的实际身份（T4 补充）

显式 `content.id` 通常是已知 ADD 身份，但 MySQL 自增列在未开启
`NO_AUTO_VALUE_ON_ZERO` 时会把 0 作为“生成新编号”。该输入返回
`422 release_auto_id_ambiguous`，请省略 id 后重建草稿；它不会占用虚构的目标 0。
开启该 SQL mode 时 0 才是合法已知身份，照常读取记录版本并参与提交目标唯一性。
此检查只用于 ADD，已有 0 记录的 MODIFY/DELETE 身份保持不变。

## 混合批量与公开预算（T6 / #53）

数据页的每次变更可以新建草稿，或选择本人同表已有 DRAFT；保存前重新读取目标草稿，并带其版本整体替换。多行删除必须逐行明确勾选，保存仍只修改草稿。详情的“添加明细”进入预选当前草稿/表的数据页；编辑器可选择任意明细修改或移除，至少保留一项。输入/记录冲突保留原意，任何非法项都不部分覆盖已保存草稿。

同一已知 MySQL 记录身份只能出现一次，数值、字符排序规则及 PAD SPACE 等价表示不能借不同操作/顺序绕过。省略自增 id 的新增不按内容相似合并。明细可选 `table_name` 只能等于顶层表名；不同表返回 `422 release_cross_table`。重复已知目标为 `422 release_duplicate_target`，无效数量为 `422 release_item_limit`。明细错误附带从 0 开始的 `error.item_index`，Web 显示从 1 开始的明细编号，可在编辑器定位；目标占用冲突也定位原请求序号。

| 预算 | 对外行为 |
| --- | --- |
| 单表明细 1～1,000 项 | 整单校验、冻结、审批、发布；不存在隐式分批提交 |
| HTTP 请求体 1 MiB（1,048,576 字节） | 所有 API 沿用 `400 request_body_too_large`，含流式正文；不放宽账号/查询请求 |
| 每个提交值或明细 id 64 KiB UTF-8，字段名 256 字节 | 超限 `422 release_field_limit`；表本身的类型、约束仍独立有效 |
| 单据/完整结果 8 MiB 编码 JSON | `422 release_result_limit`，包含完整历史、基线、最终行与幂等成功结果；小请求也可能因默认值/历史扩张被拒绝 |
| 后续动作空间 | DRAFT/PENDING_APPROVAL/APPROVED/SUCCEEDED 保留 64 KiB 终止/关联余量；人工完结可消耗其中 4 KiB，无关联 COMPLETED 仍为后续普通回滚保留 60 KiB；取消、拒绝及最终回滚状态可使用该余量，始终为 HTTP 动作元数据再留 1 KiB。不能因一次获批耗尽空间而锁死取消；T7 原成功单的关联也使用这份余量：pending 保留额外 4 KiB 终止事件空间，取消/拒绝反向申请后的 COMPLETED 与 ROLLED_BACK 可消耗它，详见[审批回滚](admin-release-rollbacks.md) |
| 全部发布单 POST/PUT 的事务/请求期限 | 正式值为 `min(8s, 正数 MYSQL_CONNECT_TIMEOUT / MYSQL_READ_TIMEOUT / MYSQL_WRITE_TIMEOUT) × 4/5`，默认 socket 5s 对应 4s；HTTP ReadTimeout/WriteTimeout 各 10s。超时整体回滚或返回提交待确认，不自动拆单 |
| 列表 | 默认 20/最多 100 个摘要，每项仅标题、ID、表、申请人、状态/版本、时间、item_count、operation_counts 与 allowed_actions；按原单号稳定游标分页，不返回 items、Publication、冻结定义或历史 |

持久文档逐张解码并验证已发布前后行，验证通过后才形成列表摘要；不会为降低列表体积跳过损坏检查。单据详情和原键恢复仍返回全部明细/最终结果；页面每次渲染 20 项且可定位任意序号，分页只影响展示。审批与执行明确包含整单。

浏览器发送前必须成功保存完整原键与请求内容；若 sessionStorage 与既有待恢复请求合计超额，尚未发送的操作给出明确提示，保留输入和已有原请求，不覆盖或自动清理其他请求。公开预算不承诺任意字段规模的 1,000 项都可通过；代表样本与实际字节/时间见 [T6 验收](verification/2026-09-08-mixed-batch.md)。


T7 [审批回滚](admin-release-rollbacks.md) 的反向草稿由已验证的原实际发布结果生成，内容不可编辑/复制，也不能作为数据页追加目标；取消或拒绝后从原单重新申请，原发布后记录版本仍固定。
