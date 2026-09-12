## Standards

审查对象：基点 `071ad8c0671f2827fd6edd7502d51eb0727e4025`；不可变快照 manifest SHA-256 `8b2203b16fa636a7dfc23501e8e51da8b8f10050193c89a27ed54ad066ae50f6`；源码 SHA-256 `d638254da29c8eeba909198c354cb92086950f0ad2e2230a113290e14e89418c`。

状态：通过本地固定快照 Standards gate。

- **Hard violations：0。** `browser-accessibility.cjs` 的 `managed()` 改用既有 `table_name` 深链，并核对选择器值、表专属新增抽屉、创建正文表名、发布单 `table_names` 与申请人 Account ID，符合 `admin/CONTEXT.md` 对 Managed Table、Release Order 和永久账号身份的定义。390px 恢复改走“打开导航→发布单→离页确认→新建草稿→保存”的真实可访问路径，符合 `web/DESIGN.md` 的手机导航、未保存保护和会话恢复约定；原正文/幂等键相等、独立审批、SQL、禁止直写及 `pageErrors: []` 断言均保留。
- **Subjective smells：0。** 移除 `repeatDraftSave` 转发 helper 后的内联步骤表达本用例特有的手机离页流程；未形成 Duplicated Code、Middle Man、Speculative Generality 或其余基线坏味道。新增身份断言集中守住同一错表风险。
- **Unresolved findings：0。** README 明确把错表 403 认定为正确拒绝，未声称已解释初始自动选表失效；也未用诊断尝试替代最终证据。
- **Local commit gate pending：0。** 92/92 路径哈希及 `SHA256SUMS` 全通过，`source-sha256.txt` 绑定候选源码。最终共享序列中 Chromium、Firefox、Linux arm64 WebKit 各 7 项 PASS，均 `pageErrors: []`、正文/键相同、SQL 成功且清理完成。
- **Post-push Linux CI gate：1。** 尚未重跑全部 34 个正式浏览器 case，且现有 Linux WebKit 为 arm64 远端浏览器、后端在 macOS；必须由正式 Linux amd64 CI 补证，当前不得声称全量 CI 已绿。

结论：hard 0，subjective 0，unresolved 0，local pending 0；post-push Linux CI 待补 1。
