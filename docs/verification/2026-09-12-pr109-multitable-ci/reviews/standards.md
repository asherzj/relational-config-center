## Standards

审查对象：基点 `d181beaa5956691368a81947a62990529eb30bbf`；源码 manifest SHA-256 `d1ff15ff7ad54a6e850d454cbec8296aba3a28155b3d1cb30ff5b9b60c004ecf`；`click-diagnostics.cjs` SHA `50ea46a0…a962`，`release-multitable.cjs` SHA `240d768b…c1d3`。

状态：通过诊断 candidate 的本地固定快照 Standards gate；不得称原 bug 已修复。

- **Hard violations：0。** `release-multitable.cjs:66-68` 只将原“查看 Change Set”点击包入诊断，未使用 force、重试、加 timeout 或改变产品交互；`web/DESIGN.md` 要求的真实可达点击与多表 Change Set 路径仍保留。`click-diagnostics.cjs:1-70` 记录有界 rect、rAF、visibility、focus、动画与失败，setup/stop/dispose 错误只进入诊断数据；原 click 异常保存后原样重抛。外围 catch 先写原异常，再做有界最佳努力页面状态与截图，诊断失败不替换原异常。原业务断言、产品代码和领域语义未变。
- **Subjective smells：0。** 诊断期限、探针停止和点击包装职责清楚；没有 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest 应报告项。
- **Unresolved code findings：0。** **原故障原因：未解决。** 20 轮 40 次点击均通过，只说明未复现；README 明确采样会扰动布局与帧时序，未把绿色诊断结果写成产品修复。
- **Local candidate gate pending：0。** manifest SHA-256 `2bcaf8d…357a2` 的 104/104 路径及内部 `SHA256SUMS` 101/101 通过，`source-sha256.txt` 绑定两份 v2 源码。合成移动按钮验证原 TimeoutError 传播、几何/帧采样和零 observer 残留；setup/stop/迟到 setup 边界均保留原异常。最终同源共享序列中 Chromium、Firefox、Linux arm64 WebKit 各 10 项 PASS、2 次诊断点击、`page_errors: []`、约 9 MiB 原请求同键同单恢复且清理完成。
- **Post-push Linux CI gate：1。** 正式运行先前在本用例后还有 9 个未执行 invocation；完整 all-suite Linux amd64 CI 尚未复跑，不得声称原问题已修复或 CI 已绿。

结论：hard 0，subjective 0，unresolved code 0，local pending 0；原原因未知，post-push CI 待补 1。
