# 发布草稿

T3 [#50](https://github.com/asherzj/relational-config-center/issues/50) 提供草稿创建、编辑、查询和取消。数据页 Change Set 的“保存为发布草稿”不会写入业务记录、推进记录版本或占用目标。旧“确认并执行”和记录写路由仍由 T5 #52 统一切换与删除；当前阶段没有审批、正式发布或分发成功承诺。T6 #53 删除单条明细临时上限。

## HTTP

所有接口沿用当前 Cookie、CSRF、同源检查、`Cache-Control: no-store` 和 `request_id`。VIEWER 可查看全部发布历史；创建、预览、修改需当前 EDITOR（ADMIN 包含该能力），修改仅限申请人。取消限当前有编辑能力的申请人或 ADMIN，原因必填。PUBLISHER 不包含 EDITOR。

| 接口 | 承诺 |
| --- | --- |
| `POST /api/v1/release-orders` | 创建唯一 DRAFT，返回 201；`Idempotency-Key` 必填 |
| `PUT /api/v1/release-orders/:id` | 整体替换草稿申请值，校验发布单 `expected_version` 和记录基线；返回 200 |
| `POST /api/v1/release-orders/:id/cancel` | `{ "expected_version": "1", "reason": "调整计划" }`，保留已取消历史 |
| `GET /api/v1/release-orders/:id` | 当前单据、全部字段差异、永久申请人及操作历史；不存在返回 404 |
| `GET /api/v1/release-orders` | `table_name`、`applicant_id`、`state`、`id` 精确筛选；`limit` 为 1～100，默认 20；按不可变单号升序、`after` 游标分页，返回 `orders` / `next_cursor` |
| `POST /api/v1/release-orders/preview` | 只读核实当前申请目标，返回 `table_name` / `items`；不保存草稿或幂等记录。用于先查看最新基线，再由用户明确采用 |

草稿输入只接受以下字段，客户端 `before`、差异字段、申请人、操作人及内部记录身份不能上传：

```json
{
  "table_name": "notification_templates",
  "items": [{
    "operation": "MODIFY",
    "id": "7",
    "expected_record_version": "3",
    "content": { "subject": null, "body": "" }
  }]
}
```

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

原值、差异类型及历史从持久草稿读取，不依赖之后的实时 Schema/Policy。当前草稿格式沿用受支持列的 JSON String 表示；不支持的 BINARY/BLOB 等列在准备前由规则/Schema 检查拒绝，非法 UTF-8 快照也拒绝，绝不静默替换后保存。最终态二进制 Codec 与正式执行由 T5 实现。

## 幂等与恢复

所有持久写入以永久账号、操作（含目标单号）和 8～64 字符的 `Idempotency-Key` 为范围，规范 JSON 请求摘要与原业务结果一并持久化。对象键顺序不影响摘要。同键同内容重试返回原业务结果，包含原版本，即使该版本已前进；同键改内容返回 `idempotency_conflict` (409)。可执行动作重新按当前身份与当前单据状态计算；原业务结果不是当前详情的替代，界面随后重新读取详情。当前鉴权/所有权不能靠历史幂等结果绕过。

并发相同请求由数据库唯一键收敛；新状态动作锁定单据并校验版本。草稿、历史与幂等结果同一事务提交，失败全部回滚，不保留半完成请求。草稿保存没有业务行写入方法，也不分配在途目标。

Web 在请求发送前将原键、路径和申请内容保存在当前标签页的 sessionStorage，以 Account ID 隔离；Cookie/CSRF 不进入该记录。刷新或重新登录后可查看原申请并使用原请求恢复。未知结果后的 401/403 或一次详情未找到均不能删除原请求或自动换键；切换账号不能重放上一账号请求。结果确定成功后删除临时请求，未发出的普通编辑仍沿用现有未保存保护。

原请求写重试经过服务器去重后，若明确返回 `release_version_conflict`、`record_version_conflict` 或 `release_state_invalid`，说明该标识没有已成功业务结果；页面解除未知状态，但将原申请持久保留为待重建内容；刷新后的恢复卡片仍可查看完整原意，必须先读取最新单据/记录并明确确认重建，才能用新标识保存。鉴权失败或一次详情读取失败不能据此丢弃原标识。

待重建申请也受统一写入入口保护：普通编辑或新建不能覆盖同作用域中持久保留的冲突申请。只有查看最新基线后的明确重建动作可以授权下一次新键写入；取消恢复会明确显示实际取消动作与原原因。


## 存储和后续复用

停写升级时，在 007/008/009 之后运行 `deploy/mysql/migrations/010-release-drafts.sql`；新安装的 `001-schema.sql` 包含同一表定义。迁移可重跑，Ready 检查完整字段、InnoDB 和唯一键。新控制表全部受 `rcc_*` 通用表保护：

- `rcc_release_orders` 保存不可变单号、申请人、状态/版本及完整草稿文档（含操作历史）。
- `rcc_release_requests` 保存账号/操作/请求键、SHA-256 摘要与原结果，永久保留。

`application.ReleaseOrderSession` 只公开控制数据写入和规则/记录基线读取，事务由 MySQL `ExecuteReleaseOrder` 拥有。T4 在此基础上增加提交、冻结及目标占用；不能另造记录版本或应用字符串身份。`ReadRecordBaseline` 复用 `recordIdentityMetadata` / `recordWeightExpression`，缺行已知 id 通过 live 主键类型转换和同一 MySQL collation 权重解析，读取统一墓碑及整表维护基线。返回的 `RecordKey` 只用于内部持久化/后续占用，不公开给 Web。身份算法/数据库版本改变仍遵循 [记录版本维护流程](admin-record-versions.md)。

`make test-browser` 可通过 `RCC_E2E_OUTPUT=/absolute/path` 保留各脚本的截图证据；默认仍写入测试临时目录。
## 自增主键 0 的实际身份（T4 补充）

显式 `content.id` 通常是已知 ADD 身份，但 MySQL 自增列在未开启
`NO_AUTO_VALUE_ON_ZERO` 时会把 0 作为“生成新编号”。该输入返回
`422 release_auto_id_ambiguous`，请省略 id 后重建草稿；它不会占用虚构的目标 0。
开启该 SQL mode 时 0 才是合法已知身份，照常读取记录版本并参与提交目标唯一性。
此检查只用于 ADD，已有 0 记录的 MODIFY/DELETE 身份保持不变。
