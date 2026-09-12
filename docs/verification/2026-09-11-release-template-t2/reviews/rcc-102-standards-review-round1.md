# #102 Standards 初轮

独立审查者：/root/review_101_standards。协调者从审查者最终消息归档。固定基点8b79faa3b720f7e2a6837aa43ba7952b0ee76028，工作树/private/tmp/rcc-issue-102-table-release-templates；只读，未重跑测试。

2项P2成文规范问题：

1. TableReleaseTemplatesDrawer.tsx:37 保存成功仅设置常驻文本，55行直接显示。违反web/DESIGN.md:94“成功反馈使用Sonner，默认3.2秒，可手动关闭”。应使用useToast().showToast。
2. TablePolicyDrawer.tsx:78只检查latest.data，刷新失败仍有缓存时清除写入错误并把旧版本当最新；启停详情缺少恢复入口，163行继续提交旧版本。违反DESIGN.md:93真实失败说明与可执行下一步。应先确认refetch成功，失败保留输入、版本、错误并显示读取错误及重试，覆盖启停冲突。

已检查全部12项主观基线：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest；无额外主观发现。未发现新增分层依赖或迁移规范违例。

快照SHA256前缀：关联抽屉43a423389bf3，表规则抽屉5e59e9252f01，共享请求事务ff42bef81d6f，readiness22515cb6f2a3，迁移SQL4fc1bf17d62e，manifest01fc39bf49b41。基点已解析，提交列表为空。

待最终核验：修复及恢复证据、当前权限/竞争和受影响旧调用回归、完整浏览器/迁移证据索引、最终文档一致性。
