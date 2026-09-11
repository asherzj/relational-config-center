# #103 Standards 首轮

**成文规范发现 1（P2），主观坏味道 0。** 只读，未运行测试或读取 Spec 报告。

固定基点 `f7b3f7bbdb42665b67a590b1b182a86f5ecf3d32`，工作树 `/private/tmp/rcc-issue-103-release-instances`。采用 `git diff <base>` 并完整读取 `git ls-files --others --exclude-standard` 中新增源码，包含 application/domain/MySQL 的 release_flow、五个新 Go 集成文件及新 Web 组件/脚本；`git log <base>..HEAD --oneline` 为空，符合预提交时点。

**P2：补齐流程成功缺少明确成功反馈。** 新入口 `web/src/features/release-orders/ReleaseOrdersPage.tsx:83` → `ReleaseDraftEditor.tsx:56–58` 保存成功仅调用 `afterSave(onClose)`，共享 `useReleaseWrite` 也只失效缓存，没有 Sonner。若保存已提交而随后详情读取失败，抽屉关闭后仍可能显示旧缺失信息/读取错误，用户无从确认此次保存成功。违反 `web/DESIGN.md:97`：“成功反馈使用 Sonner，默认 3.2 秒，可手动关闭。”修正：收到明确成功结果后，通过共享 Toast 显示草稿保存成功；反馈不依赖后续 refetch，亦不可在未知结果或失败时提前宣称补齐。保留现有关闭保护、原请求与输入恢复。

其他已核对部分未发现违规：配置一致读取及实例/结果持久化留在已有 MySQL 事务边界；HTTP 错误稳定映射，读取失败未冒充缺配置；按表节点人员与时间来自实际决定和事件。最新 create/copy/replay 补强在授权锁内重读账号并校验 EDITOR，先于原请求结果重推，符合 ADR-0012/0013、0018、0026。未要求 #104/#105 或 #106 退出项提前交付。

快照：[30 个审阅输入 SHA256](/tmp/rcc-103-standards-source-round1.json)。关键值：`release_orders.go` `0b67f2f5022d41f38f28266f8da26bad1e994bfa2c9437d4c5b3e3abb481eb74`；`ReleaseDraftEditor.tsx` `c3e8a60206ba54d020184e8e79cd4a13400b75a72b50c4e078f7e044b706e2fa`。后续仅复核变化部分。

待最终核验（不另计缺陷）：上述 UI 修复；failure/authorization 测试最终原始结果及输入匹配（包括授权真实红→绿）；真实浏览器及桌面/390px截图；领域词汇、ADR-0027 与最终证据索引同步。当前不宣称整票验收通过。

完整主观基线均无可行动新增项：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库规则优先，工具已强制项跳过。
