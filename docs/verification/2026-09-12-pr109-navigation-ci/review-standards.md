## Standards

审查对象：基点 `73364a352caf5258d48f9391ed7a95d62bec6d53`；最终 manifest SHA-256 `e3f1709e…81e24`（120 项）；源码 manifest `7be7bc79…c865d`；`release-multitable.cjs` `b00e0299…0575`。

- **Hard violations：0。** `web/e2e/release-multitable.cjs:150-157` 等待新单“复制自”挂载，展开“基本信息”，按来源单 ID 的精确可访问名称点击真实反向链接，再沿既有正向链接返回新单。这符合 `admin/CONTEXT.md` 的 Reprepared Release Order 来源关系及 `web/DESIGN.md` 的详情/真实交互约定；权限、产品代码、timeout、十项业务断言、诊断 helper 与 `page_errors` 断言均未改变。
- **Subjective smells：0。** 五增一删直接表达双向验收路径；未见 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。
- **Unresolved：2（非源码 Standards 阻塞）。** 500ms 真实 HTTP 200 延迟对照中，旧 hard `goto` 三轮各取消三条新单/通知读取，新链接三轮取消为零；因此验收导航中的该请求取消路径已修正。实验未同字复现第四 CI 的 access-control page errors，未确认 WebKit 底层原因；另一 20 秒间歇点击仍未定位，不能称两类底层故障均已修复。
- **Local gate：通过。** 120 项外层 manifest、118 项内部 `SHA256SUMS`、源码绑定均逐项匹配。固定源码共享全套在 Chromium、Firefox、Linux arm64 WebKit 各完成十项检查、`page_errors: []`、双向复制关联、9 MiB 恢复、数据库 post-check 与 cleanup；构建/typecheck、`node --check`、`git diff --check` 和 timeout-cleanup 通过。
- **Post-push Linux CI：待补。** 本地 Chromium/Firefox 为 macOS，WebKit 为 Linux arm64；GitHub 全 Linux amd64 的完整 34 次正式流程尚未执行，第四 CI 的最后九次也曾因前序失败未运行。

结论：Standards 通过；hard 0，subjective 0，local pending 0，post-push CI pending 1。
