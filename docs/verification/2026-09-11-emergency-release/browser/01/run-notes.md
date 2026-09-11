# 首轮真实浏览器原始失败

`../01-run.log` 和 `../01-run.json` 保留真实 exit=1。该轮到达 `SUCCEEDED`，随后脚本仅等待页面已经存在的状态文字便检查重推次数，出现 `1 !== 2`；未等待原提交 POST 的响应完成。后续脚本改为等待实际 POST 200 及提交窗口关闭后检查原键、原正文和当前状态。

运行时 `docker ps` 只读观察到本轮容器 `31386c0f1612`，镜像 `mysql:8.4`，映射端口 `47819 → 3306`，与 `runtime-target.json` 的 `localhost:47819/rcc_test` 一致。随后尝试 `docker inspect` 时测试已经完成清理，命令实际返回 `error: no such object: 31386c0f1612`，exit=1；重定向产生的空 `container.json` 保留，不声称它包含成功检查结果。测试日志记录该容器停止、终止成功。

此轮使用自动归档器的原始输入清单，未包含当时尚未跟踪的新浏览器脚本，因此只作为探索性失败和排错证据。后续运行器会在启动前同时散列已跟踪源码及新脚本，并自动捕获完整容器身份及结束清理状态。

`emergency-draft-desktop.png`、`emergency-pending-publication-mobile.png`、`release-emergency-failure.png` 已逐张视觉检查。长标题、模板、节点和人员身份能够换行，桌面及手机布局未发现横向溢出。手机图中“最近审批人／尚无批准记录”对免审批单不适用，已作为本票呈现问题交 Web 实现者修复并新增断言。该轮图片不是修复后最终验收图片。
