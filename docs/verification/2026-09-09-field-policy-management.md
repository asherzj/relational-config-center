# #74 表字段规则管理验证

基点：main `8b5cd85`；分支 `codex/issue-74-field-policy-management`。独立worktree和隔离MySQL8.4；未修改用户原工作区。本记录记录本地验收，提交、PR与集成状态见#74交付记录；未部署。实现规格为#74的AC-001、AC-002、AC-020及父规格#71。设计基准为`web/DESIGN.md`和[字段规则设计](../design-notes/field-policy-management.md)。

## 验收外循环

- AC-001先在真实MySQL、注册并授予ADMIN的HTTP客户端上失败：字段配置GET返回404 `route_not_found`。完成垂直切片后保存、重复保存、重读均成功，真实字段不重复，显示名称/说明/顺序/控件/选项/默认值完整回显。
- AC-002第二个循环先失败：未知控件auto由数据库错误映射为503；随后加入完整预检，非法控件、运算符、选项、默认值、不可完成新增组合、未知字段明确422，原配置不变。VIEWER写入返回403。
- AC-020比较新安装与模拟已存在控制表的数据库经过014两次升级后的SHOW CREATE TABLE，结构完全一致。真实旧发布草稿和3条业务行保持；唯一键与布尔CHECK生效。014本身只创建新字段规则表。

## 已通过的针对性自动化

`admin/cmd/admin/field_policy_integration_test.go`六条真实MySQL/HTTP用例覆盖三个AC，以及：

- default_value属性缺失→SQL NULL、JSON null→JSON NULL、空字符串→JSON STRING，数据库与HTTP均保持区别；显示名称200/201字、空白名、uint32顺序边界；数字范围与步长；upsert保持ID/creator/created_at。
- 先通过全候选校验再写入；隔离库追加的CHECK在第二字段更新时拒绝，第一字段变更也回滚，完整原配置保留。
- Schema从数字漂移为字符串时原number规则变为incompatible；effective回退text/exact；只改变enabled可原样停用。编辑该失配disabled规则、新建非法disabled规则或重新启用均422。
- 配置表不可读取返回503，绝不伪装为未配置。

HTTP适配层测试证明上下文截止返回504 `field_policy_timeout`、依赖故障返回503 `field_policy_unavailable`，不暴露驱动详情，保留Request ID。Web公共协议测试证明原始未知控件/运算符可保留修正、effective严格已知、默认值三态、422可直接修正而503/504需核对。

## 工程检查

- `make test`：全部Go模块、应用/领域测试及架构检查通过。
- `pnpm --dir web test:run --maxWorkers=2`：29文件、334测试全部通过。
- `pnpm --dir web build`：TypeScript与生产构建通过。
- `go test ./internal/interfaces/http`：最新超时错误映射与依赖边界通过。
- `go test -tags=integration ./cmd/admin -run '^TestFieldPolicyReadDistinguishes'`：最终停用例外边界通过。
- `make test-browser`：完整路径通过，162.011秒，全部7个浏览器脚本通过。
- 完整MySQL范围已运行；原单次命令因3个失败和40m超时返回FAIL，不能记为通过。注册清单231条顶层测试；超时堆栈显示所有串行及其子测试已完成（229条，其中3失败），仅两项parallel启动检查运行2秒。其他包全部通过：application 0.942s、domain 0.472s、mysql 69.595s、http 112.768s、config 2.581s。
- 已修旧库升级测试漏执行014，并增加缺014时安全启动诊断；升级测试14.89s、两个未完成启动检查3.09s/3.85s定向补跑通过，总19.424s。其余显式013序列属于刻意不要求完整HTTP Schema的Policy Catalog维护测试，无需追加新表依赖。
- 账户脚本与既有Mutation时间断言定向补测2/2通过（12.71s、14.66s，总28.408s）。账户脚本由本机代理导致回环连接中断；独立回环探针仅关闭代理即可恢复，进程级`NO_PROXY/no_proxy=127.0.0.1,localhost,::1`下原用例通过。时间断言对应Colima在ADD/MODIFY之间校时：16:15:45.308630+08:00日志记录793.539581ms漂移同步，邻近journal明确记录Time jumped backwards；两次隔离复测均通过，无需修改业务取时。
- 完整测试集合已用原范围运行与5项定向补测完成分次覆盖。[全范围清单、原失败及补测索引](2026-09-09-field-policy-integration-results.json)保留231条注册清单、包结果、原日志摘要哈希、覆盖推理和脱敏VM时间证据。原单次40m命令仍为FAIL，未改写为PASS；补证覆盖全部已知失败与未完成项。

正式集成运行设置：`DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`、`TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`。首次testcontainers未自动读取Colima context，显式指定后恢复。Web TCP中断测试需要本机监听权限；在正确权限下控制为2 worker通过。首次浏览器证据目录缺失已补建。

## 真实浏览器

字段管理脚本`web/e2e/field-policies.cjs`通过真实Chromium→Vite同源代理→Admin→MySQL执行；无前端Mock服务。已通过保存与完整回显、字段切换/未保存关闭保护、number范围/步长、390px静态选项、停用保留、真实422错误保留输入、真实PUT成功后丢弃响应并只读核对再恢复编辑。最终完整运行已证明Tab到实际选项值输入框，以及关闭后焦点返回原入口按钮。浏览器为Chrome152.0.7977.83，无pageerror。

已发现并修复原生select以及回显textarea的隐式标签不稳定：正式控件提供明确中文aria-label；没有通过放宽测试定位来掩盖可访问性问题。保存校验失败按error.field_name切到真实字段，聚焦并滚动错误说明；未知结果保持本地输入和完整服务器核对摘要。

已用view_image检查桌面1440×1000与手机390×844截图，核对：

1. 桌面抽屉640px、固定标题/底栏、独立内容滚动，符合已有布局。
2. 中性灰画布、白面板、石墨保存操作，无新增品牌色或装饰阴影。
3. 显示/查询/录入用间距和细分隔线分组，正文与辅助层级清晰。
4. 桌面选项名称、实际值和删除并列对齐；手机改为单列，标签和值保持关联。
5. 手机390px无页面横向溢出，按钮与勾选均可达，中文说明和真实字段名正常换行。
6. 成功toast初次截图遮住底栏，最终已采集通知消失后的viewport截图，固定底栏完整可见；这是采集时点修正，不修改全站通知布局。

证据：[完整浏览器结果](2026-09-09-field-policy-browser/summary.json)、[桌面字段配置](2026-09-09-field-policy-browser/field-policies.cjs/desktop-edit.png)、[桌面选项](2026-09-09-field-policy-browser/field-policies.cjs/desktop-options.png)、[390px选项与底栏](2026-09-09-field-policy-browser/field-policies.cjs/mobile-options.png)。已移除本次运行生成的其他功能重复截图，仅保留本工单必要画面与回归汇总。

## 评审与交付边界

以固定基点8b5cd85进行Standards/Spec独立双轴评审。已修复：未知原始控件/运算符的管理回显、422误判未知结果、Schema漂移规则停用恢复边界、稳定504超时映射、字段错误定位、架构ADR及核对摘要原值/警告。两轴复核均无剩余确认阻塞。数据库和HTTP边界由既有Go架构机器检查继续保护，并把新共享服务纳入禁止持久保存请求身份的检查。

新增accepted ADR0024；更新Admin领域术语、Policy Catalog、HTTP契约、014迁移/升级说明、技术基线和Web设计规范。无临时接口、双写、Feature Flag或显示快照；下游查询/录入/显示消费在后续工单接入，管理页面明确提示。项目Notion记录由主任务在最终验收后更新；提交、PR与集成状态见#74交付记录。
