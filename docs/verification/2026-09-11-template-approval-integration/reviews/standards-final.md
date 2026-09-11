# 依赖合并 Standards 最终增量复核

**Standards 放行：成文规范发现 0，主观坏味道 0。** 范围仅为 #103 前依赖合并；沿用 round1 的 ours `7eb9dd36affe16739d8e8b178afdd9977d4976a3`、theirs `2612bb354ef71d42e193648564b9f30bd9281eb4` 及 30 文件范围，增补一个浏览器脚本。未重跑测试、修改源码或读取 Spec 报告。

增量未削弱契约：

- `admin/cmd/admin/business_auth_integration_test.go:543` 改为并发撤销并观察真实授权锁等待；仍断言在途写入成功、永久发布人及业务值、撤销完成后的旧会话 401。符合 ADR-0026 的当前资格与授权变更串行化、ADR-0018 的永久身份归属。
- `admin/cmd/admin/release_history_delivery_integration_test.go:220` 单独检查实时 ApprovalContext 的新版本、VIEWER 无审批资格和独立 ADMIN 默认；七种状态的保存字段、冻结审批、事件及执行仍完全比较，拒绝篡改前后连实时上下文也完整比较。符合 ADR-0026，并未忽略保存历史。
- `web/e2e/table-release-templates.cjs:12`、`:27` 只增加真实可见导航、四入口断言和截图，原未知结果、会话恢复、冲突与窄屏操作检查全部保留，符合 `web/DESIGN.md`。

证据核验：schema 14 项、HTTP 有效 19 项、Web 有效 188 项及 TS/build、三条浏览器 28 checks 均有对应通过记录。HTTP 明确为原批次 17 项加两次受影响重验；原失败 exit1 保留。224 个后端最终输入、19 个浏览器输入、29 个浏览器产物及 21 个 Web 产物 hash 全匹配。旧 schema 快照仅两测试不同，旧 Web 快照仅新增断言脚本不同，复用合理；诊断通过后的撤销文件未再改变。四份 HTTP 人读日志完整包含原 JSON Output 字节流，仅外加命令/环境及退出码；六份 raw 日志/diff 摘要与长度一致，未以源码空白规则改写。

复核 SHA256：

| 文件 | hash |
| --- | --- |
| business_auth_integration_test.go | `8926ea0e0e90dd27de09d532546a7767413691793fc29ce9f9dba45dcc484dc9` |
| release_history_delivery_integration_test.go | `de6ab79b477da5e01f36a10505738d2e6d06ffcce9549290bb1a7fdb50af3ff2` |
| table-release-templates.cjs | `a60142244c14cef57cc7c194d674b52e89622e2a770383034ab14940df835299` |

[31 文件最终快照](/tmp/rcc-template-approval-integration-standards-final-source-sha256.json)。其余 round1 范围文件未变。已核验材料无证据缺口；root 正在汇总的总 README、全量清单和人工截图复核由 root 收口，不据此宣称尚未提交的合并已交付。

完整主观基线均无新增可行动项：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库规则优先，工具已强制项跳过。
