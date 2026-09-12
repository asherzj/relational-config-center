## Standards — #110 bounded-repeat delta

审查对象：base `2ecfeef3e155a659358bb3a06d212f7f3fe63458`；固定 tree `3e7a2652ea7c13b2f3bfeaae3a61a2f1786f0bee`；9 paths。

- **成文硬约束：0 违规。** `.github/workflows/ci.yml:167-216` 只改 PR111 临时诊断 job：保留正式八个 gate、正式 Browser `DEBUG`、原 runner/accounts/900-sample monitor；生成同目录临时 runner 后执行。原 job 15 分钟、每 case 600 秒、失败退出与 always artifact/kernel 采集保持。EXIT trap 的 monitor kill/wait 和临时文件删除均容错；证据验证 case 退出 42 在 monitor/删除失败时仍为 42。
- **生成器：符合。** `scripts/diagnose-accounts-webkit-repeat.cjs:1-17` 要求原 accounts 调用精确出现两次且目标位于 `scripts/`，否则写文件前失败；只把这两处换成固定 12 轮循环。每轮仍调用原 `run_browser_suite`，使用独立 Node 进程与 `iteration-N` artifact，首个失败由原 `set -e` 停止。12 轮共享 runner 已启动的真实服务，但不保证在 15 分钟 job 上限内完成。
- **主观坏味道：0。** 临时边界、命名与退出条件明确；未见 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。
- **证据完整性：通过。** 固定 tree 的 reviewed manifest 10 项与内部 `SHA256SUMS` 5 项逐项匹配；生成器检查覆盖 12 次成功、第三次失败即停、600 秒参数、不同 PID/目录、源漂移和错误目录拒绝。unchanged-scope 绑定正式 workflow 外部、runner、accounts 与 monitor 未变。
- **提交取证 gate：通过；native evidence pending：1。** Linux amd64 真实 12 轮只能在提交后 CI 取得；通过或未复现都不能称原 crash 已修复。
- **Final delivery blocker：1。** 12 轮有界调查结束且最终受影响验证完成后，必须实际删除临时诊断 job、native sampler、repeat generator、生成副本及正式 Browser `DEBUG`；清理后另建快照复核。

结论：hard 0，subjective 0；当前 tree 可提交取证，最终交付未通过。
