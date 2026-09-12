# Standards 复核

状态：通过。成文规范违规 0；独立主观坏味道 0。

本结论绑定 `/private/tmp/rcc-105-review-snapshot-07`。基点及 HEAD：`d31eabd6f7947004d39fe872afee8aaca37bbad2`，新增 commit 为空。45 个文件全部匹配 `source-07.json`，路径集合与固定 diff 一致；diff SHA256：`8b71cdd313012daeb91e1aa98c0a5027150876f6c7d636963cd3bb6756808ced`。沿用初审完整规范及坏味道基线，仅从冻结目录读取源码。

初审 **P2 已关闭**：`web/src/features/release-orders/ReleaseRequestReview.tsx:47–55,94–95` 仅 GET 检查冲突，再进入共享恢复窗口；`QuickRollbackDialog.tsx:48–53,66,81,90–92` 由明确保存按钮处理预览，最终确认才替换原执行，符合 `web/DESIGN.md:152–154`。当前主单、权限、版本和原请求保护保持有效。

保留 source02/source04 通过结论：请求展示共用 `ReleaseRequestIntent.tsx`；空集合归一化位于 HTTP 边界，列表投影保留实例，符合 ADR-0012/0013；身份测试经公开 HTTP 执行，截图等待有限动画，结果按每表游标与版本核对。

source06 四文件增量结论保留：`QuickRollbackDialog.tsx:69–72` 在有可访问关联的顶部描述中说明历史预览及原单终态，符合 `web/DESIGN.md` 的真实状态与历史响应约定；相关断言同步文案。`web/e2e/release-rollbacks.cjs:68–70` 通过公开 GET 取得关联版本后配置测试，遵守版本并发保护。

source07 三文件增量通过：`QuickRollbackDialog.tsx:32–38` 在异步忙碌结束且焦点仍留在窗口容器或 body 时补回取消焦点，不抢走原因等控件的现有焦点；原 `useModalFocus` 的焦点循环、退出及恢复机制保留，符合 `web/DESIGN.md:102`。组件增加取消焦点断言；旧浏览器断言按真实预览事件核对六条历史及唯一 `PREVIEW_QUICK_ROLLBACK`。未发现新规范违规或独立坏味道。

本次未运行 DB、浏览器或测试，未修改源码或提交；不替 Spec 判断验收证据。工具已强制项目不重复报告。
