Spec 首轮：0 项源码/规格发现，必需结果待补证。

固定 base `0edb62af`；已核对 source02 manifest 与完整 diff 的 SHA256 均匹配。

- AC-025、AC-027 要求“节点状态来自真实实例与事件”。ReleaseOrdersPage.tsx:140 已统一使用保存阶段；旧总览及专用 CSS 删除。新增取消/拒绝浏览器断言（release-instances.cjs:153）与回滚断言（release-rollback-flows.cjs:168）覆盖真实终止状态，不虚构发布或完结。
- AC-026 要求“Goose 为唯一迁移来源”“无自动数据删除”。夹具准备（prepare-release-fixtures.cjs:25）先取得合法管理员，再加载历史 SQL，仅经公开接口配置新增关联；runner（browser-acceptance.sh:424）检查配置后及重启后 ready。没有新增产品写入口或迁移兜底。9→11、恢复、数据保留和只读 ready 已有专门用例设计，无需重写。
- AC-027 要求“每项 AC 有证据……文档及项目记录同步”。历史 AC-001～024 的归属及验证入口可衔接；最终适用性仍须结合本票结果索引核定。

目前全量 MySQL、完整浏览器/三引擎、Compose、余下 Go 检查及最终视觉与项目记录证据尚在补齐。此前失败日志应继续保留，并明确映射窄复验。以上是结果待补证，不是覆盖设计缺失；收到固定证据索引后再作最终放行判断。
