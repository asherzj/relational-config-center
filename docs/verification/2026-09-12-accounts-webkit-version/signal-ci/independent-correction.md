# `/tmp/rcc-110-signal-ci-verification.md` 更正说明

绑定原报告 SHA-256：`7b59211437f714b92551b89e0d582e724a0b154561e03571ad25a52b3e54b5e2`。原报告保持原字节，不以本文件覆盖。

## 更正 1：三份 trace 的异常归属

原文“后两份无异常 signal；前者在 08:43:38.701Z 正常关闭”有指代错误，并与下一段冲突。正确表述为：

> Playwright 将三次启动映射为初始 persistent `webkit-7683`、reviewer `webkit-7806`、重开 persistent `webkit-7874`。`webkit-7683` 与 `webkit-7874` 均无异常 signal，分别在 persistent reopen 和最终 cleanup 中正常关闭；**reviewer 的 `webkit-7806` 含 `ThreadedCompositor` SIGSEGV，并记录其 WPEWebProcess 以 SIGSEGV/core-dumped 结束。**

该更正不改变原报告的失败判定、原生时间线、ptrace 限制或 gate 结论。

## 更正 2：已完成的 SQL 边界

原文“该轮未完成 21 checks/真实发布 SQL”表述过宽。该轮确实未生成最终 21-check 成功结果，也未完成第二个 MODIFY 发布；但在崩溃前已完成首个 ADD 的审批、丢响应重放、真实执行与结果核验，并查询到实际行/version，随后还通过 API 完成并验证了一次 concurrent MODIFY。准确表述应为：

> 该轮未完成全部 21 checks，也未完成第二个 UI MODIFY 发布及最终成功态 SQL/结果验收；崩溃前已经完成并核验了前段真实 ADD 发布与 concurrent MODIFY。

因此不能把本轮记为 accounts/SQL 全通过，也不能说它没有执行任何真实发布 SQL。

## 其他核对

其余关键事实与文件哈希均匹配原始证据：iteration 1 首失败停止；reviewer SIGSEGV 在 lifecycle page-crash 与 cleanup 之前；无 `kill/tgkill` sender；launcher SHA/mode 前后一致；backup 删除仅有 trap 成功路径与无 restoration 错误的间接证据。未发现其他需更正的文字。
