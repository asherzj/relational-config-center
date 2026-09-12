# #110 Accounts WebKit signal diagnostic CI 核验

**对象**：commit `a221562036136942ff5cbbeace95a44d13532900`，CI `34683913900`，job `103527409970`。结论：**诊断 job 按首个真实失败停止；装置取得有效原生信号，但这不是根因修复。**

## 执行范围与业务失败

只启动 iteration 1。`case-results.tsv` 中 `accounts.mjs@webkit` 唯一 selected、exit 1；此前 4 个 release-workflow case 为 not-selected，iteration 2–12 未创建，符合 first-failure stop，未 retry。真实 accounts 场景已完成 persistent reopen；首次发布确认点击 533ms 成功，随后 reviewer 在“确认批准”click 时失败（`accounts.mjs:435`）。lifecycle：52,199ms `page-crash`（reviewer）、52,200ms `test-failure`，52,366ms 才 `cleanup-start`，故不是 cleanup 关闭。该轮未完成 21 checks/真实发布 SQL；全局 runner exit 1，但 Docker/MySQL cleanup verified，`database-final.txt` 恢复基线汇总。

## 原生时间线

Playwright 将三次启动映射为 trace：初始 persistent `webkit-7683`、reviewer `webkit-7806`、重开 persistent `webkit-7874`。后两份无异常 signal；前者在 08:43:38.701Z 正常关闭。

reviewer trace 在 **2026-09-12T08:44:00.047340Z** 记录 PID 7852 `ThreadedCompositor` 的 `SIGSEGV {si_code=SEGV_MAPERR, si_addr=NULL}`，随即再次记录同一 fault；没有 `kill()`/`tgkill()` 调用，也没有外部 sender PID（硬件 fault siginfo 不提供 sender）。**08:44:15.405960–15.423868Z**，该 WPEWebProcess 线程组以 SIGSEGV/core-dumped 结束，leader PID 7839；MiniBrowser PID 7815 于 **08:44:15.423925Z** 收到该子进程的 `SIGCHLD`，`waitid` 明确 `CLD_DUMPED/SIGSEGV`。由初始关闭事件校准，lifecycle page-crash 约为 **08:44:15.423Z**、test-failure 15.424Z、cleanup-start 15.590Z。MiniBrowser 随后在 cleanup 正常 exit 0，说明父 launcher 的 0 不能否定 renderer 崩溃。

这是未注入的真实业务 crash，排除了外部 kill 这一观测分支；但 ptrace 会扰动时序，且没有栈/core，不能证明未插桩故障必由同一机制触发，也未定位 NULL fault 的代码根因。

## 装置与恢复

实际 Playwright 入口为 `webkit-2336/pw_run.sh`；before/after SHA 均 `a85baad3…c63b`，mode 均 `755`。三份 trace均来自该 wrapper，artifact 上传成功。raw log 无 `restoration failed`，结合 trap 逻辑表明恢复与删除 backup 均返回成功。上传范围不含浏览器 cache，因此 artifact 不能直接展示 `.rcc-110-original` 的事后缺席；这是删除证据边界，不能用 artifact 目录内“未发现”代替。

## Gate 与证据

信号取证目标通过，业务/最终交付门禁仍失败；不得把本次 SIGSEGV 记录称为根因修复。不得 retry。后续仍须删除临时 job、两脚本、生成 runner/cache wrapper，并由正式八项 CI 验收。

- artifact：`/tmp/rcc-110-signal-ci-artifacts`；上传 digest `a482d46b8f67123ebd355a8a55cd12eb2afc27c3e15daf0059c7f13d65ff3e3c`（ID `10294413650`）
- raw log：`/tmp/rcc-110-signal-ci.log`，SHA-256 `b911c54a98e28a25bc49b3e3c3b591f6deb7484f04df3f397893ab677b30773b`
- reviewer trace `webkit-7806` SHA-256 `6f8df7ccdaa47b37c17e8481f84091f65ccf652cc017a988d8a14c5c0ca630c7`
- lifecycle SHA-256 `2beaaedb5c029a380168ffae7ce898de704de34a3962de74ff8c3f82a5b6c3e0`
