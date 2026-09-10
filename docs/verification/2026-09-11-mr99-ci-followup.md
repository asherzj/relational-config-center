# MR #99 首轮 CI 跟进

首轮远端 CI 暴露两处验收夹具问题。此次基于已发布的 `dafdde3a4a41a2b8afd03ae57fa51b9908fae08b`，仅调整两个测试文件；不改变生产行为或原测试超时。作为同一 MR 的后续提交交付，不重写原 #88 合并提交，也不以本地复验代替后续远端 CI 状态。

组合查询的容量用例在远端耗时 27,459ms，超过原 20,000ms 上限，完整 Web 为 397/398 通过。该测试在每次添加值后重复对不断增长的 DOM 做角色查询；现在缓存两个稳定的真实按钮，通过精确可访问 label 查找每个新输入。仍逐项添加、填写 100 个值，并将原数量断言加强为完整有序值相等；第 101 个填写值仍须显示明确错误，且只提交一次。原 20 秒上限保持。

WebKit 无障碍脚本等待目标表标题超时，但原 HTTP 查询、创建正文和页面标题实际都是 `field_display_browser_items`，场景要求的是 `stage1_acceptance_items`。原脚本已经调用 `selectOption(table)`，之后只等待新增按钮可见；没有证据确定这次选择为何未生效，不能推断为 React 根因或标题加载缓慢。现在两次进入内容管理页都使用既有 `table_name` URL，等待新增按钮真实可操作后，核对 select 当前值及此次所有表查询的目标路径和 200 状态；首次创建再核对标题与唯一明细的表名。

原键盘、窄屏滚动和焦点断言不变。真实 422 后仍核对 APPROVED、原始 CR、无执行和数据库零写入；后续取消、复制、显式换行转换，以及丢响应后原正文与幂等键恢复断言均保留。

## 本地复验

| 范围 | 最终实际结果 | 证据 |
| --- | --- | --- |
| 完整 CombinedQueryForm 文件 | 13/13，通过且无跳过；容量用例实测 3,074ms | [文件日志](2026-09-11-mr99-ci-followup/web-file.log) |
| 完整 Web test:run | 37 文件、398/398 通过 | [完整日志](2026-09-11-mr99-ci-followup/web-all.log) |
| TypeScript typecheck | 退出 0 | [类型日志](2026-09-11-mr99-ci-followup/web-typecheck.log) |
| Chromium / Firefox / WebKit 正式无障碍范围 | 每引擎 7/7，通过且页面错误为空 | [Chromium](2026-09-11-mr99-ci-followup/browser-chromium.json)、[Firefox](2026-09-11-mr99-ci-followup/browser-firefox.json)、[WebKit](2026-09-11-mr99-ci-followup/browser-webkit.json) |
| 共同结束守卫 | 任务退出 0、容器与卷清理成功、种子五行逐字段前后相同、直接 Admin 和 Web 代理未登录均 401 | [运行](2026-09-11-mr99-ci-followup/run.txt)、[前](2026-09-11-mr99-ci-followup/fixture-before.tsv)/[后](2026-09-11-mr99-ci-followup/fixture-after.tsv)、[认证](2026-09-11-mr99-ci-followup/auth-boundary.txt) |

Web 最终三条命令先串行完成，再用现有正式 shell 入口运行三个引擎的受影响无障碍范围，共享一个隔离 MySQL。入口构建所需 Admin/Web 并执行既有后置守卫；此次没有重复完整 34 范围、Go 集成或 Compose 验收。单次本地耗时是观察值，不是远端性能保证；WebKit 指 Playwright 引擎，不是安装版 Safari。

中间失败仍保留：第一次完整 Web 运行被沙箱拒绝现有流式测试监听 `127.0.0.1`；有界授权后完整运行通过。两个误加到 `getByRole` 的 `exact` 属性曾造成类型错误，移除后才得到上述三条最终绿结果。见[监听拒绝](2026-09-11-mr99-ci-followup/intermediate-loopback-denied.log)及[类型错误](2026-09-11-mr99-ci-followup/intermediate-type-error.log)，不将其改记为首次全绿。

Standards 与 Spec 对固定基点后的两个测试文件独立只读复审，均无残余发现：[Standards](2026-09-11-mr99-ci-followup/standards-review.md)、[Spec](2026-09-11-mr99-ci-followup/spec-review.md)。审查报告形成时浏览器尚在运行，其静态结论与上表后续实际运行结果分开记录。

## 来源与复核

- 原始 [Web CI 日志](2026-09-11-mr99-ci-followup/ci-web.log)、[浏览器 CI 日志](2026-09-11-mr99-ci-followup/ci-browser.log)来自 [MR #99 首轮 Actions run 34510123199](https://github.com/asherzj/relational-config-center/actions/runs/34510123199)。Web job 为 `102981892010`；浏览器 artifact 为 `10165724878`，原 ZIP SHA-256 为 `6c16ccf511a9197325896f4defadc7ae3713270e73caba3eea3a02413145b0ad`。
- 原失败 WebKit 的[真实 HTTP](2026-09-11-mr99-ci-followup/ci-webkit-http.json)、[机器结果](2026-09-11-mr99-ci-followup/ci-webkit-result.json)及[页面正文](2026-09-11-mr99-ci-followup/ci-webkit-body.txt)保留错表事实。
- [两文件源码与 diff 摘要](2026-09-11-mr99-ci-followup/source.json)、[具体差异](2026-09-11-mr99-ci-followup/change.diff)、[Web 命令记录](2026-09-11-mr99-ci-followup/web-results.json)、[浏览器命令记录](2026-09-11-mr99-ci-followup/browser-results.json)绑定本次实际候选。命令记录中的仓外原始路径按 [附件索引](2026-09-11-mr99-ci-followup/evidence-index.json) 的 source 字段映射到随提交归档的原字节附件及 SHA-256；历史 `/private/tmp` 路径不是复核的唯一依赖。

原始日志附件保留捕获到的尾随空白、空行及 CR，逐字节哈希不作格式化。提交前的空白检查覆盖源码和说明文档；仅对这些已核对来源与哈希的原始输出文件作数据保真豁免。
