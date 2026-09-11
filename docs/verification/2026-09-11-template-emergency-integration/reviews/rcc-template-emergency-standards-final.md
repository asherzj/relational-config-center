Standards：**0 项未解决发现（成文 0、主观 0），本次合并增量放行。**

范围：`/private/tmp/rcc-template-notification-integration`，ours `e7e19b022fc606c137fdf8df1899caf9a5702bb1`，theirs `f545adf24aadd34ee39afb2643a4819aee54c986`；只读复核冲突解决、语义对接、夹具与实际证据，未重跑测试、未读 Spec 报告。

本轮 P2 文档遗漏已关闭：[DESIGN.md:143](/private/tmp/rcc-template-notification-integration/web/DESIGN.md:143) 起三段已逐字恢复 incoming 的发布方式切换、原因校验和冲突重建规则，满足 [设计维护规则:19、22](/private/tmp/rcc-template-notification-integration/docs/agents/design.md:19)。当前 SHA256 `df43e179333e9164005aaa10ecc20a41a123a6154175a7dc0e2fda4d1305e4d9`。

后端保留一致读取、当前审批资格和个人通知，同时完整投影应急原因、类型、流程及缺失表；Web 保留紧凑布局、通知确认、原请求恢复和应急真实状态。历史测试 [release_history_delivery_integration_test.go:220](/private/tmp/rcc-template-notification-integration/admin/cmd/admin/release_history_delivery_integration_test.go:220) 先逐字核对旧请求行，再核验唯一新增停用结果归属，未弱化不可变历史。新增 [account_browser_integration_test.go:110](/private/tmp/rcc-template-notification-integration/admin/cmd/admin/account_browser_integration_test.go:110) 通过正式 API 配置四张历史表关联并断言真实 ready；两份原 SQL、22 个迁移 SQL/manifest 均未改。

证据有效：52 个唯一 HTTP＝原批 51＋历史窄补验 1；154 Web、TS/build 通过。浏览器 **6 条唯一路径**＝01–05＋07；06 失败、06a 诊断均保留，账号父测试/子测试不双计。07 的 480 输入全部匹配；前五条仅未调用的账号夹具文件变化，复用成立。抽查 10 张实际截图未见新增规范问题。incoming 的 68 个 raw 日志/差异文件保持原字节。

完整主观基线均已检查，无可行动新增项：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库规则优先，工具已强制项跳过。

50 份源码/规范逐文件哈希、证据哈希和复用边界见 [actual input](/tmp/rcc-template-emergency-standards-actualinput.json)，SHA256 `a072c5b6cae7654a9378e24178db6f3e335ac0ca7bddaa3f1d610bdbe97ef58e`。总 README 与交付清单仍由 root 封存；本报告不代替该最终归档。
