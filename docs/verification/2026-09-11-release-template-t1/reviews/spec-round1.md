# #101 Spec 首轮评审

固定基点：56dff5360e94d904ab85c17cea5cc575919a29b1；工作树：/private/tmp/rcc-issue-101-release-templates。预提交工作区含未跟踪文件，未运行全仓测试。评审：/root/spec_backend_review。5项发现，最高P1。

1. P1，AC-001/003：mysql/release_templates.go:182 把空 monitor_list 复制到 nil，HTTP 输出 null，但 Web 契约要求数组，目录、详情与创建响应均解码失败。返回非nil空数组，补公开响应契约。
2. P1，AC-004：mysql/schema_migration_validation.go:104 强制 default_standard_v1.enabled=1，合法停用默认常规模板导致 readiness 失败。移除常规模板必须启用要求，验证正式停用后仍就绪。
3. P1，AC-005：ReleaseTemplatesPage.tsx:39 使用实时 detail.data.version，表单仍持首次打开旧内容；后台刷新或同账号恢复后旧内容携新版可覆盖其他管理员修改。版本与表单基线一起固定，仅显式核对后更新。
4. P1，一致性决策3–4：ReleaseTemplatesPage.tsx:57 使用 WriteRecovery、禁止原保存并要求独立只读核对，若未创建且404无法解锁。改为保存原身份及正文的手动原操作重推，覆盖编辑、启停和删除丢响应。
5. P2，AC-002/005：ReleaseTemplatesPage.tsx:31 的创建键页面存活期间不更新，第二次新建复用旧键报异内容冲突。新意图新键，未知结果重推保持旧键与正文。

证据待核验：普通账号拒绝、真实审计、丢响应、上述页面版本竞争、合法停用后的只读就绪；真实MySQL及浏览器当时尚在进行。未发现需另报的临时结构退出项。

## 首轮快照 SHA256

- admin/internal/infrastructure/mysql/release_templates.go: 5e307c7787bc2d0ec51b2087e383bade10c5d59b9b6c8a0ea5f146dbbb81915f
- admin/internal/infrastructure/mysql/schema_migration_validation.go: d0786a7c07036057cf7b1e193d8249402661a17550034759a4edc21c9918faab
- web/src/features/release-templates/ReleaseTemplatesPage.tsx: 0125cc31a13da6f0abc6f305435dcc309eeb281f47b1edd14ffbc255cbc74e31
- web/src/features/release-templates/ReleaseTemplateForm.tsx: 686c4bef91b4adae4b7b930b7d06f07016d9e49d7ea5a5b8785c7656145554e1
- admin/cmd/admin/release_template_integration_test.go: 08636b919fc2aaa5539286ad19e6af330387409b26e602dad9659018fbebe102
