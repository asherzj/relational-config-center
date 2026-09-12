## Standards

审查对象：base `a306c94f484a5a4796f727bc4a37c192d9538bef`；最终 manifest SHA-256 `c643d7e0…c8634`（93 项）；source manifest `86ea92b9…9b555`；`web/e2e/accounts.mjs` `27596293…fcd9f`。

- **Hard violations：0。** `accounts.mjs:117-141` 只写测试 artifact，并用 context/page/browser 生命周期事件及标签区分 `initial-persistent`、`reviewer`、`reopened-persistent`；`248-252` 在既有主动关闭前标为 `persistent-reopen`，`528-529` 在最终清理前标为 `cleanup`，因此正常关闭不会被误报为场景内提前关闭。`318-326` 复用既有且未修改的 `clickWithDiagnostics` 包裹原精确按钮点击，不改变 Playwright actionability 或 timeout。`513-526` 先记录原错误，所有新增截图、状态写入和诊断 deadline 失败均被吞并，随后重新抛出同一错误；诊断失败不会替换业务失败。原 21 checks、权限、SQL、持久化与请求重放语义未改。
- **Subjective smells：0。** 生命周期记录与点击观察职责命名明确、局部集中；未见 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。
- **Unresolved code findings：0。** `clickWithDiagnostics` 在被观察点击期间每 100ms 采样并运行 rAF 计数；它会轻微改变 renderer 调度，证据只能描述“带探针的本次运行”，不能据通过/未复现排除原始时序故障。失败 artifact 的两次等待各受 2s deadline 约束；底层操作不能被 Promise race 主动取消，但其结果不会覆盖原异常。
- **Local gate：通过。** 外层 93 项、`reviewed-manifest.json` 91 项、内部 `SHA256SUMS` 90 项及 source binding 均逐项匹配。固定源码在 Chromium、Firefox、Linux arm64 WebKit 各通过原 21 checks；确认点击为 852/958/971ms，无 crash/test-failure，真实 SQL before/after 一致、post-check 与 cleanup 通过。十轮 WebKit 有界循环 10/10，但未自然复现原故障。阳性对照主动关闭页面后，日志依次记录 confirmation 内 page-close、同一原 click 异常、test-failure、cleanup，证明诊断能区分受控关闭与正常清理；它不证明 CI 的自然根因。
- **Post-push CI：待补。** Chromium/Firefox 在 macOS、WebKit 在 Linux arm64；GitHub Linux amd64 的正式结果仍是独立边界。

结论：Standards 通过；hard 0，subjective 0，unresolved code 0，local pending 0，post-push CI pending 1。
