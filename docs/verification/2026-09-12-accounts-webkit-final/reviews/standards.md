## Standards — #110 final cleanup

审查对象：base `5ac0d268e51814973b977371270da2f351fccbda`；固定 tree `71fc2720ae2eba9f1c28e2f21e7639be1fd36f2c`。

- **成文硬约束：0 违规。** `.github/workflows/ci.yml:151-163` 删除临时 Browser `DEBUG`，并完整删除 PR111 专用 native job；该文件 SHA-256 `f38f5927…54bb` 与 `d4f1202` 固定版本逐字节一致，恢复 Web、Go、四组 MySQL、Browser、Compose 共八个正式 job。`scripts/diagnose-accounts-webkit-native.cjs` 与 `scripts/diagnose-accounts-webkit-repeat.cjs` 均从 tree 删除；`scripts/` 无生成的 `.accounts-webkit-repeat.*`。正式 runner SHA `cfafd6d6…9681` 与 `d4f1202` 一致。
- **净源码边界：符合。** tree 中唯一永久源码差仍为已审 `web/e2e/accounts.mjs`，SHA-256 `27596293…fcd9f`；本 delta 未改变产品、权限、SQL、点击、timeout、断言或 runner。历史诊断 `.log`、空 TSV 列及原始尾空白作为冻结证据保留，不将原始字节改写成源码格式。
- **主观坏味道：0。** cleanup 聚合删除同一临时职责，没有 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。
- **证据完整性：通过。** 固定 tree 的 reviewed manifest 22 项与内部 `SHA256SUMS` 20 项逐项匹配；cleanup checks 记录 YAML/Node 与 authored source/docs whitespace checks 通过、runtime CI 引用 0、generated runner 0。Linux amd64 有界诊断第一轮 21 checks 通过、第二轮自然 page-crash 后立即失败并完成 cleanup；正常 SQL postcheck 未执行，故不是完整业务回归，也不证明底层根因已修复。
- **Cleanup 提交门禁：通过。** 所有规定临时设施已实际清除，可提交该 cleanup tree。
- **Final delivery：待补 1。** cleanup 提交后的最终 HEAD 必须取得恢复后的八项 CI；此前未变源码证据可复用，但不能替代最终 required statuses。

结论：hard 0，subjective 0；cleanup local gate 通过，final delivery 尚未通过，原生根因仍未知。
