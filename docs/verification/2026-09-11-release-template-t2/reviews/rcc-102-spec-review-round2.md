# #102 Spec 增量复审（第2轮）

评审者：/root/spec_backend_review。工作树 `/private/tmp/rcc-issue-102-table-release-templates`；固定基点 `8b79faa3b720f7e2a6837aa43ba7952b0ee76028`。来源：已读取的 GitHub #100/#102 正文、标签和空评论，以及 `docs/design-notes/release-template-specification.md`、`release-template-ticket-plan.md`。只复核受影响增量，未运行测试，未读取 Standards 报告。

## 原发现追踪

1. 已修，AC-006/当前模板效果：`web/src/features/table-policies/TableReleaseTemplatesDrawer.tsx:35–40` 在原操作成功后失效并重新读取关联，干净表单同步真实读取，编辑中的输入/基线不被后台更新替换；当前说明取读取结果（51）。`queries.ts:42` 不再把历史重放响应写为当前表规则缓存，而是失效详情查询。新增组件用原成功v2、当前v4及下一次请求v4验证，而非仅断言刷新函数被调用。
2. 已修，AC-008及一致性决策2–4：关联表单41、`TablePolicyDrawer.tsx:154` 将确定版本冲突与仍未知结果分开；前者解锁并保留输入，显式读取新版后新键提交。403仍保留原包。关联组件实际验证503→409→读取v3→新键/v3，表规则组件还验证读取失败不能用旧缓存清除原错误。

未发现上述修复之外新的规格偏离；不作最终0阻塞结论。协调者已告知 `TablePolicyDrawer.tsx:78–83` 的显式读取与写入互斥竞态正在修复，本次快照尚无统一忙碌保护，待稳定增量及针对性证据复核。

## 证据状态

直接读取日志确认：Web四文件70例通过（5.26s）；浏览器第4次8项通过（用例23.88s、包24.863s），包括真实提交后丢响应、真实注销/同账号登录后原请求保留。迁移README记录11顶层用例有效日志、失败历史与最终输入快照；新关联四组原始绿日志前轮已核对。

唯一待收口清单：上述互斥修复及其最终Web/浏览器快照；仍运行的T2/T1/受影响HTTP整批结果与旧ConcurrentAccounts契约补正重跑；最终证据索引。当前不能把部分绿日志写成整批通过。

## 本次SHA256

- TableReleaseTemplatesDrawer.tsx: `ab75832189717f5d8f9a4a6d970dfa588088110b7dfa3dd57b3646a397959703`
- TablePolicyDrawer.tsx: `0da4bfb2008138130cfe855d555e03dbbf7c0b6bdb4fa26e849fbbdb8b97846e`
- queries.ts: `b03c2f4432e32ad2a391b3587333cac755d0491f05b0619bcae1525515907ad5`
- TableReleaseTemplatesDrawer.test.tsx: `28e8a55d3f00d2b9403af5d6e2e07ab95ef1d52ce974f14a5a427d23a9fe82ff`
- TablePoliciesPage.test.tsx: `7014c507abb0d3d3967a1c2baba473d6bcd5c3e5b5c07bd93a052ff194b5861f`
- mysql/management_requests.go: `ff42bef81d6f25d91a8060f2678c9f91c3128fe441bab8b83865c44533d71038`
- mysql/table_release_templates.go: `d71e3c8b8b9280afecb3f36550801d4f7f5bfa88fe08be18c7227de364d42251`
