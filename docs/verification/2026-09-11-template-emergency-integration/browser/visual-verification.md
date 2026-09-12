# 合并后的视觉核对

Root 在实际运行通过后以原分辨率读取以下截图：

- `01-instances/partial-approval-desktop.png`：紧凑页头、各表独立实例与部分审批状态一致；没有把一表通过显示为整单通过。
- `02-emergency/emergency-pending-publication-mobile.png`：390px 长标题及节点名完整换行，应急原因和真实待发布阶段可读，无审批安排或审批事件；原提交按钮保留用于未知请求手动恢复。
- `02-emergency/emergency-reason-mobile.png`：有标签的原因、字符计数、滚动差异及底部确认均可达。
- `03-notification-center/detail-mobile.png`：通知已读与当前审批资格同时显示，返回入口、主动作、逐表节点和长角色身份无页面横向溢出。差异表在局部横向滚动，不把屏外列误认为缺失。

- `07-release-approvals/release-approvals.cjs/release-detail-approved-390.png`：根因修复后的实际审批页面，长标题、完整身份与来源可换行；已批准阶段及对应逐表节点一致，主发布操作、重新准备和更多入口可达，展开差异有可见焦点环。

上述为真实截图观察；几何、键盘、网络结果和持久化事实以各路径原始 JSON 与日志为准。其余截图的独立视觉抽检保存在 Standards 复核报告中。
