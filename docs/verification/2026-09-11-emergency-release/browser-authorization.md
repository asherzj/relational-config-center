# 浏览器验收明确授权

此前两次自动审批拒绝原文保留在 `web/helper/approval-rejection.txt`，拟运行目标和写入范围保留在 `browser-approval-plan.md`，不改写历史状态。

父任务于本次恢复时转交用户对该计划的明确答复：“同意啊，你继续”。所批准的问题是按该计划，在新建的一次性本地环境中创建并运行认证写入、应急发布和完结测试。

本次沿用原计划：新建 Testcontainers `mysql:8.4` / `rcc_test`，由本工作树启动 Admin 与 Vite；仅本地 HTTP loopback，显式 `RCC_E2E_ISOLATED=1`，新浏览器上下文，仅写测试数据。Go harness 在浏览器启动前写 `runtime-target.json`，记录实际数据库地址、数据库名、Admin/Vite 地址及进程 ID；运行期间另记录匹配数据库端口的 Docker 容器身份，结束检查清理。

此授权允许恢复此前停止的动作，不表示忽略后续自动审批；若发生新的实际拒绝，仍按原规则处理。尚未运行的检查不记为通过。
