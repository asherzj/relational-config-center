# PR111 signal 候选正式 Browser 对照

**对象**：commit `a221562036136942ff5cbbeace95a44d13532900`，CI `34683913900`，正式 Browser job `103527409853`（无 ptrace）。结论：**FAIL**。

`case-results.tsv` 有 16 个互异且均为 selected 的 case：15 个 exit 0，`release-rollbacks.cjs@webkit` exit 1；期望 34 项中另 18 项未执行，无重复或额外项。失败发生在 `accounts.mjs@webkit` 之前，因此本次正式 job 没有提供 accounts WebKit 对照结果。

WebKit rollback runner 先输出全部 7 条业务 PASS，随后在 `release-rollbacks.cjs:333` 的最终 `assert.deepEqual(errors, [])` 失败：page error 数组实际为 `['/127.0.0.1:35863/api/v1/approval-notifications due to access control checks.']`。这是原始证据可确定的**最早失败断言**；page error 本身没有时间戳，不能从工件断言它具体在哪个业务步骤产生。没有 `TargetClosed`、page-crash lifecycle 或 OS signal 证据，不能把它与独立 ptrace job 捕获的 reviewer compositor SIGSEGV 合并为同一故障。

runner 退出 1，Make/Actions step 最终退出 2；`cleanup verified: true`。正式 Browser gate 仍失败，不得据 7 条中间 PASS 宣称 rollback 或完整 CI 通过，且未作 retry。

- artifact：`/tmp/rcc-111-signal-formal-artifacts`；上传 digest `2dc666562dc40c4beadd70fceda4bcc22bcaf0e4be573008de657a5cab16660c`（ID `10294514130`）
- raw log：`/tmp/rcc-111-signal-formal.log`，SHA-256 `3dc3969b302e667b2d4b4b93d100162cb96b5caa736cce80ae4434551af7401e`
- case matrix SHA-256 `aa60f80e47f847fdeb76cdc80d3b118641c166b9d76a52f6752f6981ca8d2f09`
- failing runner SHA-256 `aed46b09fef5f56158538a1221367838687b419a1337d2910bd8d6e3f771d370`
