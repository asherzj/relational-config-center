# #101 Spec 第3轮

固定基点56dff5360e94d904ab85c17cea5cc575919a29b1；评审/root/spec_backend_review。前两轮后端问题已解决，HTTP/真MySQL17项回归已经核验通过；仍有1项行为缺陷和1项文档不一致。

1. P1，一致性决策4：ReleaseTemplatesPage.tsx:73无条件使用旧unresolvedWrite，但89确认框显示当前操作。停用响应丢失→取消→点删除→确认，实际重放停用；换模板也可能重放旧目标。未知结果恢复要明确绑定原目标和动作，不能让新确认复用旧请求。补取消、换动作、换目标后的真实交互证据。
2. P2，AC-004应急可用性：ADR0026:11及schema-migrations文案称至少一个启用应急模板，实际schema_migration_validation.go:106要求固定default_emergency_v1。修正文档，不误导其他自建应急模板能替代默认必备模板。

已核验：原响应重放、后续状态不受影响、同编码新建不被旧删除误删、五种写入结果保存失败整体回滚、当前授权/并发赢家审计；monitor_list=[]、表单固定原基线、新创建换键、普通删除后只读ready。

唯一待核验：最终浏览器与证据索引，特别后台真实刷新新版后旧表单仍携原版本、同账号恢复/跨账号隔离、未知操作取消后的恢复。已写浏览器源码不等于已跑通过。

快照SHA256前缀：MySQL模板3e74ec2054fe；ready e1812d8af0e5；Page cbf2b5299d70；Form a9e0aeb39689；HTTP集成0b10adf8c8da。未发现其他临时层退出遗漏。
