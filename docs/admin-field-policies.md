# 表字段规则管理

父规格 #71，管理工单 #74，组合查询 #76。字段规则属于交互配置，ADMIN 在「表规则分配 → 字段配置」中维护。规则不增加字段执行权限，也不进入发布单的执行 Schema/规则快照；原有 Query Policy、Mutation Policy、真实 Schema、审批、发布和回滚机制继续生效。

## HTTP 契约

`GET /api/v1/table-field-policies/:table_name`：已登录账号可读取真实字段和当前交互配置。路径须为普通业务表；受保护的 `rcc_*` 返回403，表不存在返回404。读取不要求表规则启用，以便管理员维护停用中的表；保存要求已有表规则分配。

`PUT /api/v1/table-field-policies/:table_name`：ADMIN、同源与CSRF校验；正文为 `{"policies":[完整规则,...]}`。所有字段须来自该表当前真实 Schema，字段名不得重复。整个列表预检后在一个事务中替换，未列出的规则被删除以恢复未配置状态；空数组明确清除整表字段交互配置。Web停用使用 `enabled:false`，保留配置而不删除。请求不接受额外字段。

完整规则字段：

| 字段 | 值与约束 |
| --- | --- |
| field_name | 真实字段名，不可手写创造字段 |
| display_name | 1～200字，不得纯空白；Web新建时取真实字段名 |
| description | 最多500字，可为空字符串 |
| display_order | 0～4294967295整数；显示消费按此值再按field_name排序 |
| is_visible / is_queryable | 列表显示与查询交互开关 |
| query_operators | 明确运算符数组；可查询时至少一项，不重复、不推导 |
| ui_type | text、textarea、number、boolean、date、datetime、select、radio；省略时text，无auto |
| ui_options | 对象，`options:[{label,value}]`，可选字符串`min`、`max`、`step` |
| editable_on_add / editable_on_modify / is_required | 仅引导Web录入，不作为业务HTTP执行授权 |
| default_value | **属性不存在**表示无预填（数据库SQL NULL）；**JSON null**表示显式空值；字符串表示无损预填值，`""`为空字符串 |
| enabled | false停用并完整保留配置 |

运算符复用 exact、contains、open_range、closed_range、in、not_in、is_null、is_not_null；contains仅文本，范围遵循现有真实类型能力，空值运算符仅允许NULL的字段。真实类型不支持或未知运算符拒绝保存。数组最多1024条字段规则、静态选项最多1000项，HTTP请求体仍为1MiB。查询最多256条 AND 条件，集合每条最多100值，范围按一条条件计数。

`ui_options.options` 仅适用于select/radio，值为无损字符串；选项名称1～100字，实际值不重复且与真实字段类型/文本长度兼容。radio至少一个选项，预填必须在目录中；select允许合法目录外预填、自定义业务值。number支持min/max/step；其他控件不接受这些约束，范围和步长按真实数字类型校验，min≤max，step>0，预填须满足范围及相对min（未配置min时相对0）的步长。数值比较采用任意精度有理数，Web不经浮点数转换业务值。

数据库非NULL且无默认值、生成、自增或现有Mutation Policy自动填写来源的字段，不能在启用规则中关闭新增编辑。默认值只用于新增；预填NULL与非NULL或必填冲突、预填空字符串与必填冲突均拒绝。

## 读取结果与下游消费

结果为 `{"table_name":"...","fields":[...],"query_capacity":{"max_conditions":256,"max_values_per_condition":100,"queryable_fields":实际数量,"supported":true}}`。每项含 `field_name`、`column_type`、`nullable`、`generated`、`auto_increment`、`has_default`、`policy`、`audit`、`state`、`warning`、`effective`。`audit`携带creator/modifier/created_at/updated_at；创建人与创建时间在再次保存时不变。

| state | policy | effective与行为 |
| --- | --- | --- |
| missing | null | 默认text＋exact、真实名称；不等同读取失败 |
| disabled | 完整停用规则 | 默认text＋exact；原规则可重新启用 |
| active | 完整启用规则 | 已通过当前真实Schema配置校验的规则 |
| incompatible | 保留原规则 | warning明确失配原因，effective回退默认text＋exact |

字段已经消失时不会制造一个可写字段。原始policy可含未知控件或运算符，Web管理界面保留并显示未知项以供修正；effective仅输出已知控件/运算符。JSON列SQL NULL正规化为无选项/无运算符；它不会推导运算符，启用且可查询但无运算符的配置标为incompatible。default_value保持SQL NULL与JSON null的区别。

Schema漂移后允许原样停用既有规则（或保持原样停用），该恢复例外不允许修改规则内容；新建非法停用规则、编辑失配规则或重新启用仍须完整校验。

Schema或配置读取失败返回503 `field_policy_unavailable`；请求上下文截止返回504 `field_policy_timeout`，绝不转换为missing；错误包含Request ID。非法配置返回422 `invalid_field_policy`和`error.field_name`，错误定位到对应字段。身份或角色失败沿用401/403。结构性非法JSON返回400。

完整替换沿用last-write-wins。同表写入通过表规则分配行锁串行执行，每条字段规则使用唯一键upsert，保留ID、creator和created_at。写入中途数据库失败回滚整个集合。保存结果未知时先GET只读核对；Web锁定再次保存并保留本地输入，核对不自动覆盖输入、不自动判定前次提交成功或失败。管理员审阅后可以返回编辑，再次完整保存仍可能覆盖另一管理员的新配置。

## 组合查询容量

`query_capacity` 是保存预检、读取能力和 Web 提交验证共用的事实。数量按真实 Schema 的全部 `effective.is_queryable` 字段计算，包含未配置、停用以及失配回退的默认可查询字段，不只计算规则行。超过256时 GET 仍返回字段，`supported:false` 供管理员修正，Web 明确说明并阻止组合查询；PUT 在原子替换前以422 `invalid_field_policy`拒绝超限结果。关闭查询可降低数量，停用规则会恢复默认可查询并重新计数。业务查询仍仅执行既有 Query Policy 与 Schema 校验，不把字段交互配置变成后端授权。

选择256作为平台安全上限：最坏的256条 IN 各100个值共25,600值参数，加分页参数仍低于 MySQL 65,535 个占位符的限制（[MySQL 官方问题说明](https://bugs.mysql.com/bug.php?id=5194)）。真实MySQL 8.4验收以257列业务表证明256个有效字段（其中一个双边范围）和256×100 IN均可一次执行；测试请求113,570 字节（约111 KiB）。容量不承诺任意长度的值均能放入请求：HTTP 1 MiB、IN/NOT IN每条100值、Page Size 200、Offset 10,000以及现有超时继续执行。超过请求体的输入仍按原契约拒绝。

正式 Web 直接展示可查询字段，按display_order及field_name排序；只提交填写或显式选择的字段，全部以AND连接。未填写不参与，0和false有效，空字符串须显式选择空选项或勾选，NULL使用无值运算符。单一运算符隐藏选择器；运算符决定单值、集合、区间、无值形状，控件仍取ui_type。形状相同时切换保留输入，形状改变清空，折叠与查询失败保留当前输入。select允许自定义实际值且不修改选项目录，radio仅允许选择；首次空选项与未选择明确区分。读取失败显示错误与重试，已确认Schema不兼容显示警告及默认文本回退。

## 升级

新库及已有 Goose 库使用 `schema-migrate up`，由追加的 00003 建立字段策略表。尚未接管且已完成 013 的库，在停写维护窗口执行历史 `014-table-field-policies.sql` 并完成其他适用步骤，再按[接管手册](schema-migrations.md)显式 `baseline`；状态为 current 后再部署 Admin/Web。Admin 只读检查包括字段策略表在内的完整结构、版本和尝试状态，异常给出 `schema_not_ready` 及维护指引。00003 与历史 014 结构等价，仅新增控制表，不清理业务行、字段配置、发布单、请求幂等结果、占用、版本或通知。

字段交互管理、记录录入与组合查询已接入；历史实时显示由后续工单接入。本次没有双写、Feature Flag、显示快照或占位接口。
