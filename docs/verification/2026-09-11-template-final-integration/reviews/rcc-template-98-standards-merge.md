# #98/main 接入 Standards 有界评审

**当前稳定合并增量：成文违规 0，主观发现 0。** Ours `aec58942f01143c1a579aaf2a885141bf5a263c9`，theirs `eb55e841e80345cfef7733b8c560d6037256cd2c`；以已核验 #97 `7334e110…` 实际共同内容对照。未提交、未合入 #104；未重审 #98 原交付实现，未运行测试、未改源码、未读 Spec。

- 四处 Web 人工冲突保留紧凑业务页头、主操作和危险菜单、通知已读及当前权限；同时保留逐表持久实例与缺配置显式保存。API/Header 投影继续区分当前通知与历史原结果，符合 `web/DESIGN.md`、ADR-0013/0026/0027。来源不同的节点、提交人员时间和部分审批断言保留；折叠基本信息后查询姓名符合实际界面，没有删除行为断言来通过测试。
- 后端历史夹具分别验证当前 ApprovalContext 与不可变业务事实、通知位置；撤销并发夹具观察真实不同连接及授权锁，保留先完成已认证发布、再拒绝新请求的断言。表规则调用补原键/版本，显式 STANDARD 关联保留。
- 独立核对正式1–9共18个 SQL/manifest 全部与 theirs 原字节一致；候选SQL仅从9/10顺排到10/11。真实 SHOW CREATE TABLE 生成28/29表累计清单；前序表未丢失，符合 ADR-0024 与 `docs/schema-migrations.md`。

**文档问题已关闭：** 迁移手册原9/10当前路径已更正为10/11，并将旧号明确标为历史；`release-detail-compact.md` 的 AC-07与布局已同步真实整单阶段/逐表实例，保留旧验收时点及 #106 回滚总览退出边界，符合 `docs/agents/design.md`。

**证据边界：** Web02/03真实失败保留，05八文件154 PASS，TS/build通过；五项Web输入hash一致。四Go包检查与manifest生成PASS可对应raw。37个干净语义合并已独立重算，仅正在修改的baseline/readiness测试与原自动结果不同。Schema/cutover完整矩阵、相关夹具最终hash及后续#104相交验证仍待，不能当作最终集成交付通过。

完整主观基线：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest；工具已强制项跳过。

实际输入：`/tmp/rcc-template-98-standards-merge-inputs.json`，SHA256 `bbe644ca021077d930446404d2705b01ace4d00eabcccd09a5b7bb339c8ca0f0`。
