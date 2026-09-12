# Issue #112 — Spec 轴审查

**对象**：base `a306c94f484a5a4796f727bc4a37c192d9538bef` → tree `97622c8a06d662794867217e98eba3147c3f334f`；候选源码 SHA-256 `2474e03eb60a292e6b5c76cc92cc428b7cacc807db250c72f33a557cfbff5d82`。

## Findings

未发现阻塞本地提交的 Spec 偏差。

- **(a) 需求缺失 / (b) AC 证据**：AC-R01 原文要求“建立真实红反馈或明确边界的诊断证据”。原 CI 16/34、15 PASS、rollback WebKit 在 7 项业务检查后因唯一 page error 失败已冻结。受控 hard-navigation 红例对真实 `/approval-notifications` GET 得到 200 后仅延迟浏览器交付，1,557 ms 后导航、再 6 ms 报 `Load request cancelled`，用例 exit 1；同为 5 秒 held-200 的 UI-link 绿例无取消、无 page error、7 项检查及 runner/SQL/cleanup 通过。原 CI 文案 `due to access control checks` **未同字复现**，低层机制和自然时延仍未知，不能称精确根因复现。临时 probe 的可执行源码未归档，故“仅延迟交付”只能由 README、事件 JSON 与 Admin 200 日志交叉佐证；这满足“明确边界”用途，但不足以升级为根因证明。
- **(c) 越界 / 可疑实现**：唯一源码差异是 3 增 1 删：从详情页点击真实“返回发布单列表”，用 `competingDraft.id` 锁定行，再点精确业务标题并等待目标 URL。它验证实际记录，未改产品、权限、SQL、timeout、retry、force click、7 项检查或最终 `errors === []`。未见范围扩张或断言降级。
- **(d) 临时退出**：活动 route/listener/delay/assert 已从候选删除；仅证据目录保留惰性的连接 preload。首轮 `final-three` 的 Firefox 双标题 strict-mode 失败被如实保留；增加 row-ID 限定后的 `final-three-v2` 才是首个完整候选结果，并非同源重试。

## Gate

冻结清单 377/377 哈希通过（`SHA256SUMS` SHA-256 `d31df54d52de9ea8faa581d365106b8fdc58f3a09ed659044b8ce875969b1d9a`）；379 个 delta path 均为该源码或证据。`final-three-v2` 为 Chromium/Firefox/Linux arm64 WebKit 各 7 PASS、`browser_errors: []`、SQL 后检一致、cleanup true。**本地提交 gate：通过。最终交付 gate：待接入后 Linux amd64 完整 34-case Browser 与既定 CI 有效通过；当前不得宣称整体 CI 已绿。**
