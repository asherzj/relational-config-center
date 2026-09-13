# 无障碍浏览器失败证据

`RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance` 运行原有七项业务检查。原普通点击、15 秒元素超时、真实 CR/SQL/权限和清理断言保持；诊断没有增加点击前或期间的 DOM、RAF 或轮询任务。

各引擎目录中的 `lifecycle.jsonl` 和 `result.json.lifecycleEvidence` 记录 Playwright 宿主已观察到的 page crash/close、context close、browser disconnected、failure 和 cleanup-start。递增 pageId、创建阶段和固定 contextLabel 区分旧页与当前页。business phase 不因取证或清理而改写；`cleanupStarted` 区分正常清理后的关闭和清理前终止。没有终止事件仅表示未观察到，不能证明页面健康或解释根因。只采集最多 8 页、32 条事件；上限或日志写失败使 `complete=false` 并令验收失败。browser.close 完成或报错后封存记录，之后回调不再改变 JSONL 或 result 事件数组；这个封存时点之外的事件不在观察范围。事件不包含 URL、账号、页面文本或回调 payload。

业务失败后，并发尝试一次 LF 控件/抽屉几何、焦点、可见性和滚动 snapshot，以及原 `failure.png` / `failure-body.txt`。每项先等待内存捕获，2 秒期限内返回才同步写入文件；`result.json.failureArtifacts` 分别记录 captured、unavailable、timeout 或 write-failed。只有实际完成写入才记 `writeCompleted=true`。晚到或拒绝的浏览器 Promise 不触发写入，也不修改结果。同步磁盘 I/O 无法中断；完成时若已超过 2 秒预算，状态仍记 timeout，writeCompleted 如实记录文件已写完，不声称期限内捕获成功。snapshot 不读取文本、输入值或 URL；截图/正文仍是原失败页面工件，应按现有 CI artifact 访问和保留规则使用。

snapshot 仅描述失败后的单次窗口，不能外推到之前 15 秒点击期间。控件 absent、页面不响应或取证超时都留下未知边界；不能由可见、焦点或矩形数值认定或排除布局/滚动/生命周期原因。`failureArtifacts.complete` 仅表示这些捕获完成，不表示根因已知；`lifecycleEvidence.complete` 仅表示宿主事件记录没有已知遗漏。原业务异常在取证前写入错误输出并保留在 result.failure；取证失败不会把业务红灯改绿。

这些能力提供有明确限制的诊断证据，不是 WebKit 间歇性 LF 超时的产品修复，也不将绿色运行解释为故障已消失。专用重复采样 job、生成驱动和连续 DOM/RAF 探针不属于本套件最终能力；其他既有套件使用的共享 click-diagnostics 模块不受影响。
