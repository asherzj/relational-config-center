# 表发布流程设置

管理员在表规则目录选择「发布流程」，为每种发布方式选择一个模板。不同表可以共享模板。此处维护后续实例的配置；#102 尚未接通流程实例、应急执行或原单回滚。

`GET /api/v1/table-policies/{table_name}/release-templates` 返回 `associations`，每项包含 `table_name`、`type`、`template_code`、`template_name`、`template_enabled`、关联自身的 `enabled`、十进制字符串 `version` 以及 `creator/modifier/created_at/updated_at`。身份审计采用永久 Account ID。读取失败返回依赖错误，不当作空配置。

`PUT /api/v1/table-policies/{table_name}/release-templates/{STANDARD|EMERGENCY}` 需要当前 ADMIN 和 `Idempotency-Key`，正文为：

```json
{"template_code":"default_emergency_v1","enabled":true,"expected_version":"1"}
```

缺失的常规关联首次保存使用版本 `"0"`；已有记录使用读取到的版本。成功返回完整关联。关联与模板类型必须一致；启用关联只能选择有效模板。应急关联禁止停用，不提供解绑或删除入口；有效切换在一个事务内替换模板身份并递增关联版本。常规关联可停用，整张表规则仍可按原规则停用。仍被任何关联引用的常规模板删除返回 `release_template_in_use`，不会产生悬空引用。

版本冲突返回 `table_release_template_conflict`，类型／目标不可用返回 `invalid_table_release_template`，应急保护返回 `emergency_association_protected`。原请求结果与关联、版本和审计一起保存；同一账号同键同内容重推返回第一次保存的结果，同键异内容返回 `idempotency_conflict`。当前权限在每次重推时重新校验。界面保留原目标、内容、版本和请求键供用户手动重推，不自动业务重试、不增加独立只读结果确认。

表规则本身的创建、替换、启用和停用也遵循原请求契约：`POST /api/v1/table-policies` 带请求键创建未启用规则，成功版本为 `"1"`；`PUT /api/v1/table-policies/{table_name}` 在原有规则分配正文上增加 `expected_version`；`POST .../{table_name}/enable` 和 `POST .../{table_name}/disable` 正文为 `{"expected_version":"1"}`。版本冲突返回 `table_policy_conflict`。这些管理命令不改变发布规则冻结、字段分配或并发管控键的既有语义。

原请求重放成功只确认该次操作，界面随后读取当前配置，避免把历史结果显示为当前状态。干净关联表单同步最新读取；编辑中的选择与原版本保持不变。若原请求重推收到确定版本冲突，该次请求没有提交，界面允许显式核对最新版本后以新请求保存；会话失效或权限不足不会解除未知原请求。恢复读取与写入互斥，读取失败即使存在缓存，也保留输入、原版本和错误。

数据库只增加 `rcc_table_release_templates` 这一张关联配置表，连同 #101 的模板定义表共两张。每表规则每类型唯一，模板身份及类型由复合外键保护；完整节点仍只保存在模板 `node_list`。新建表规则与默认应急关联同事务，启用校验或补齐应急关联；初始化为既有规则补缺失默认值，不覆盖管理员已选关联，也不恢复已停用或已删除的常规模板。

当前迁移为从 #101 固定基点 `8b79faa3b720f7e2a6837aa43ba7952b0ee76028` 顺延的候选 `00009`，仅用于一次性隔离验证。已发行 1–5 与历史接管边界 5 保持不变；外部已交付 6/7 由 T3 接入后统一调整未发行候选并重建真实 MySQL 累计清单。Admin 就绪只读检查完整结构、发行记录及每个受管表的有效应急关联，不隐式修复。
