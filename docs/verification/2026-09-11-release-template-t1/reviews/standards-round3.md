# #101 Standards 第3轮

固定基点：56dff5360e94d904ab85c17cea5cc575919a29b1。独立评审：/root/review_101_standards。2项P2、1项P3待修，全部已交给实现代理。

- P2，ReleaseTemplatesPage.tsx:43/52 切换模板没有重置 create/replace mutation 错误，旧创建错误可能串入下一模板并遮挡其新错误。按操作会话重置或隔离错误。
- P2，schema_baseline_integration_test.go:301/459 历史接管测试误用当前迁移数量。历史基线固定1..5，含零版本共6行；当前含候选8的构建接管后为pending，只在显式up之后为current。独立历史版本/数量并实测，不改接管边界。
- P3，ReleaseTemplatesPage.tsx:100/103 未知生命周期结果提示丢失已有Request ID；确认窗口和取消后的恢复提示均需保留。

此前成功确认残留、成功导航拦截、未知结果重试改变原动作的问题已清除。后端新增事务、依赖方向、账号归因、安全错误边界与12项坏味道基线未发现其他规范违规。已核对后端证据九项哈希全部MATCH。

快照SHA256前缀：Page940beb3ffa28；Forma9e0aeb39689；历史接管测试a155c3d186b9。未重复执行测试，待修复及历史接管回归、最终浏览器与证据索引核验。
