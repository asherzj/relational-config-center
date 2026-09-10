# #101 Spec 第2轮

固定基点56dff5360e94d904ab85c17cea5cc575919a29b1；评审/root/spec_backend_review。只读源代码复核，未表示最终验证完成。

已修：monitor_list非nil空数组、表单固定基线版本、连续新建换键；独立确认入口已移除。

仍有3项P1：

1. AC-004及只读就绪：schema_migration_validation.go:104仍要求默认常规模板存在，正式DELETE允许删除它，合法删除使ready失败。日常常规生命周期不应等同结构损坏。
2. AC-005/一致性决策3：release-templates.ts:16起的编辑/启停/删除无请求键，后端只为create保存原请求结果。成功丢响应后重推变为409/404。应在业务事务保存原结果，所有写操作同键同内容重放，仍检查当前授权。
3. 一致性决策3–4：ReleaseTemplatesPage.tsx:40每次保存重读当前输入；46每次确认重读live detail并取反enabled。停用成功丢响应，刷新后重推可变成启用。首次发出时固定操作、目标、正文、版本和键，未知结果重推用原请求，新编辑意图另建请求。

待证据：版本组件测试GET一直返回1，没有真正触发后台从1刷新为2；需组件增强或真实浏览器竞争。最终MySQL/浏览器及文档待核验。

## SHA256

- mysql/release_templates.go: 3b90fdda1c47caf3e363c9eccff54aa7fb5be69c634939d59a5abe878d0d63c4
- mysql/schema_migration_validation.go: 9a88e40517f5993cededb7b5207bbbb61a07fa2322861d634ca1b824994b4f5a
- ReleaseTemplatesPage.tsx: 7504102013cb72be3ff3bfb1d76d60cb85fb61d5224a909f566ed189599da515
- ReleaseTemplateForm.tsx: fe4f8e8dcd1bb67c27706ed367b260e83880be4f7c813a2d7d60af7d1ba4bef1
- release-templates.ts: 5c7582905c77076cbe9cdf204b606dd08e5e3ad0a1ab73cf863a3221c8f65a0b
- queries.ts: 65ab1c28e8442eab0494be693e801f326866ebeeb33b1732bd726fd0807c4abe
