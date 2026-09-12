# #110 最终 cleanup — Spec 审查

对象：base `5ac0d268e51814973b977371270da2f351fccbda`，不可变 tree `71fc2720ae2eba9f1c28e2f21e7639be1fd36f2c`。已读取全部 24 个增删改路径；`reviewed-manifest.json` 22 项均匹配 tree，内部 `SHA256SUMS` 全通过。workflow SHA-256 `f38f5927…54bb`，最终 accounts SHA-256 `27596293…fcd9f`。

## Findings / cleanup 提交门禁

无阻断项，可提交 cleanup 以触发最终验收。

- `.github/workflows/ci.yml` 与 `d4f1202` 及 main `a306c94` 字节一致：正式 Browser 的 `DEBUG`、PR111 临时 native job 和全部诊断引用均已删除，原八项 CI 恢复。两份 `scripts/diagnose-accounts-webkit-{native,repeat}.cjs` 已删除；tree 中无生成 runner。正式 `browser-acceptance.sh` 与 main 一致。
- 相对 main 的净运行源码只有此前已独立审查的 `web/e2e/accounts.mjs` 诊断补强；未改变产品、点击、断言、权限、SQL 或 600 秒限制。历史 probes 只在 verification 目录，不是运行或 CI 依赖。
- 新归档准确保留 run `34681965121`：第 1 轮 21 checks 通过；第 2 轮 line 439 自然 page-crash/exit 1，后十轮未执行，cleanup true；仅 6 个账号且无 next cursor。152 samples 的 OOM 计数为零，且无 native stack/signal。故证据没有把失败、12 轮或根因改写为成功。
- 四份被 ignore 规则覆盖的原始 `.log`，以及 TSV、lifecycle、resources、kernel 与独立 CI 报告，均与下载原件逐字节一致；原始空字段/日志尾空格被刻意保留，authored 文件的 whitespace 检查另行通过。

## 验收映射与限制

AC-F01 的原始失败、执行边界及环境已保留；AC-F02 复用未变 accounts 源的三引擎各 21 checks/真实 SQL/丢响应证据及正式 34-case 绿跑；AC-F03 明确这是有界诊断补强，根因仍未知、没有故障修复声明；本次 Spec 冻结审查满足 AC-F04 的本轴本地条件。

最终交付仍待 cleanup commit 的原八项 CI，尤其正式 Browser 34-case，及另一独立审查有效通过。既有 `2ec` 9/9 只能支持未变范围，不能替代最终 HEAD 状态；当前门禁不可宣告交付完成。
