# 双轴审查输入

固定点 `836c95da4100d06a102dc6cd0266035cafe18ceb`。implement 要求审查通过后才提交，因此 `git log <base>..HEAD --oneline` 为空；规范三点命令 `git diff <base>...HEAD` 也为空。实际待交付对象为本票未提交完整工作树，所有新增源码已显式 intent-to-add：使用 `git diff 836c95da4100d06a102dc6cd0266035cafe18ceb --`；其文件哈希在 review-source-manifest.json，审查修复之后另记复核。

Standards依据：AGENTS.md、docs/agents/{domain,design}.md、CONTEXT-MAP.md、admin/CONTEXT.md、web/DESIGN.md、docs/adr/0001-admin-governs-existing-tables.md、0002-one-managed-database-with-default-deny.md、0012-enforce-admin-ddd-boundaries-with-go-internal.md、0026-authorize-approval-by-table-roles.md，以及 code-review 完整坏味道基线。

Spec依据：完整原始issue-96.json与issue-92.json；本票唯一主要AC013～018，排除#97结果事件接入与#98旧权限切换。README为证据索引，最终共享调用方选择18项回归正在接浏览器后串行运行；测试状态不可将待执行误记为通过。

重点复核：notification是查看者私有读取投影，不能落入跨账号主单document或原请求结果。Get在同一RR快照组装header+approvalContext+receipt；写响应仍按原业务DTO，不能用其缺失/旧值充作最新已读依据。本人动作、资格失效与保留结果未读、已处理历史分别维护。旧快照禁止补造或转换，GET只读。reset新增表是其直接受影响既有调用方，经root明确授权代码+测试扩展，未执行用户环境reset；只有manifest精确验证过的非级联notification→account FK例外。
