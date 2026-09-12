# PR109 multitable copy navigation — Spec review

**结论：local commit gate 可通过；Findings 0。**

## Findings（a–e）

未发现需求、验收证据、范围、兼容/退出条件或可疑实现问题。

- **行为符合性：** 复制成功并读回新单后，测试等待真实“复制自”元数据，展开“基本信息”，点击精确 source-ID 链接回原单，再点击原单上的 copied-ID 链接返回新单（`release-multitable.cjs:150-158`）。这实际验证双向关联，且保留 copied table/detail-ID 断言和原 10 项多表业务验收。
- **边界未放宽：** 仅以 SPA 业务链接替换一次测试内 hard `page.goto`；产品、权限、timeout、click diagnostics、API read、独立审批、发布/回滚、9MiB 恢复及最终零 page-error 断言均未改。没有临时产品兼容结构或额外退出条件。
- **源码映射：** `source.diff` 从固定 base `73364a…6d53` 精确重建单一源码的 5 增/1 删；source SHA-256 `b00e0299…0575`。source manifest 与 120 项完整 manifest 均匹配给定 SHA，全部文件哈希及内部 `SHA256SUMS` 通过。

## 验收证据

第四次正式 CI 的两个目标 click 已通过且 10 个业务 check 全过，最终仍诚实失败于 notification 与新复制单 GET 的 access-control page errors。

受控 500ms、真实 HTTP 200 延迟实验中，旧 hard navigation 三轮各取消两次 notification GET 和一次 copied-order GET，共 9 次；同条件链接导航为 0。两边 `page_errors=[]`，所以红绿只证明旧测试可中断在途读取，不是 CI 原文 page-error 复现，也不确定 WebKit 底层机制。

冻结源码的共享 Chromium → Firefox → Linux WebKit 运行均 10/10、`page_errors=[]`、两个诊断 click 成功；真实双向复制、25 项发布/回滚、独立审批、390px、9MiB 同 key/order 与账号隔离均保留。runner exit 0，build/typecheck、timeout-cleanup、数据库 post-check/cleanup 通过。

## Unresolved / 必需待补证

早先 20 秒 `not stable` click 超时仍未定因或修复。完整 34-case Linux amd64 post-push CI 尚待运行；其通过只能关闭当前回归门禁，不能证明两个间歇性 WebKit 现象的底层原因。
