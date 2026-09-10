# T3 / #84 接口与交接

同一发布单可保存一个数据源中的多表有序明细。主单 `table_names` 从实际明细推导，列表按任一明细表过滤后仍只返回主单一次；`detail_id` 在增量编辑、分页和排序中保持。提交冻结标题、全局明细与每表执行定义；发布按正序写入，原单回滚按严格逆序写入。结果按全局位置组装，并在各自明细行保存申请、发布、恢复三类事实。

`PublicationPlan.Tables` 按表保存 schema、执行定义、规则及摘要，`Items` 保存唯一业务顺序。MySQL 的同表批量读、记录版本推进和结果存储不能改变 Items 的业务 DML 遍历。每表 `table_versions` / `notifications` 进入成功执行摘要；结果、版本、命令、主单、目标、通知和请求结果共享外层事务。已知 ID 的同表重复由数据库等价身份拒绝，跨表同 ID 可共存；自增结果按该次 INSERT 的真实 ID 继续保留占用。

名称解析只读系统变量/标量表达式，随后按统一身份排序取得所有表保护锁，最后才读取字典和业务快照。草稿 SHARE 与发布/回滚 UPDATE 协议保留；同单父键先于已存在的完整子键，禁止空范围锁。创建表规则也用相同名称语义。Submit 必须继续检查原值、主键身份及完整补充键集合，不能以更新准备后的 Items 掩盖身份改变。

## 后续责任

- #85：基于逐项真实表名和完整目标集合完成复制/重新准备业务接线；复用当前多表准备和冻结能力，不另建执行器。
- #86：复用 `release-journal.ts` 的 IndexedDB 权威日志。`hydrateReleaseRequests` 恢复完整原 body/key，`rememberReleaseRequest` 事务完成后才可发送，`forgetReleaseRequest` 精确匹配账号/key。同 scope 未确认请求不可覆盖；跨窗口原请求确认后保留本窗口未保存输入。日志不可用只阻止发布写入；不自动发送、不退回 sessionStorage，不以新 key/空正文代替缺失原请求。最终重推/错误 UI 仍属 #86。
- #87：成功执行 ID 与 kind 是原因编辑的寻址入口；不修改原申请、原发布结果和执行事实。
- #88：删除主单 TableName/Frozen、首项表版本/notification 显示别名、旧反向单字段/拒绝路由、旧预算常量和错误枚举。FrozenDigest 仍是全局冻结摘要，不属于单表别名。没有旧环境数据转换或删除。

## 验证入口

`release_multitable_integration_test.go` 覆盖交错外键顺序、全局错误位置、原单完整恢复、三表通知、自增占用、1000/1001 分页整单和旧阈值以上 HTTP 大值；`TestPublicationAtomicPersistenceFailures` 跨两表在晚表版本/通知及控制存储边界注入真实 MySQL 失败；`TestReleaseFreezeMetadataCaseInsensitiveNames` 覆盖 mode1 两种拼写及大写规则创建；HTTP `TestReleaseDetailCapacityRouteContract` 覆盖声明长度和流式请求的窄路由预算。

浏览器 `release-multitable.cjs` 使用正式页面、真实 Admin 和串行隔离 MySQL，复用已有 main/T2 验收脚本，并补多表准备、分页排序、独立审批、三类结果、生命周期、大值响应丢失后的原请求恢复及存储失败路径。最终通过清单和异常归因见本票 verification 文档。
