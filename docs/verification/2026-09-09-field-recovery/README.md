# 字段恢复 #77 验收

基点 `7c61d0f`，规格 #71、工单 #77。继承已验收 #74/#75/#76，沿用 `web/DESIGN.md`；每工单一 commit、统一合并请求由汇总代理收尾。本工单复用既有控件、错误反馈、会话边界和原请求恢复流程。

## 验收接缝与证据

| 用例 | 证据 |
| --- | --- |
| AC-003 | `ConfiguredRowEditor.test.tsx` 与 `CombinedQueryForm.test.tsx` 验证比旧查询更新的真实元数据：新列文本／exact、旧列移除、生成列不可写、排序不保留已消失字段；原主键／自动填写边界复用 #75 与 `ManagedDataMutationPage.test.tsx`。缺失／停用的 effective 文本回退复用 [#75](../2026-09-09-field-inputs/README.md) 和 [#76](../2026-09-09-combined-query/README.md) |
| AC-012 | 完整 `AppRoutes` 页面测试保留查询与编辑输入、同一控件节点和配置；同账号恢复期间查询结果列已变化仍不重绘；重新打开编辑器／查询采用最新配置；第二 Account ID 清空旧查询和编辑草稿。`field-recovery.cjs` 实际清除 Cookie、同账号重新登录，管理员已替换配置但旧下拉自定义值和 DOM 不变；第二真实账号隔离 |
| AC-015 | 查询／编辑元数据各自 503 后仅对应错误与重试，成功前不展示默认字段；重试编辑不丢查询输入。真实 MySQL/HTTP `TestFieldPolicyReadDistinguishesDisabledSchemaDriftAndFailure` 覆盖类型失配、停用、ALTER 删除／新增列、目录表临时不可读时 503、恢复后的 GET 与故障前内容一致。修改表单发现 Schema 变化后精确读取当前行，新增列保留真实默认值和 NULL，记录缺失或两次读取 Schema 不一致均明确失败重试 |
| AC-019 | 新增字段的真实原值进入 Change Set，但既有 expected_record_version 不自动前移；模拟当前行版本 1 时请求仍用原版本 0，冲突保留输入且禁用自动覆盖。既有恢复／显式重建后“返回修改 → 再审阅”保留新基线。配置下拉草稿网络未知／后续明确拒绝仍重试原键原正文。浏览器真实 POST 成功后丢响应，手机原请求重放返回同一 version=1 草稿；原有 accounts 路径继续证明冲突、审批、发布未知恢复 |

## 修复与先红后绿

- 已开表单的配置冻结原本已存在，新增页面验收直接通过，不人为制造失败。
- 新 Schema 用例先因编辑器仍展示 `removed`、查询找不到 `new_field` 失败；改以成功读取的字段元数据作为本次控件和排序字段来源后通过，新增字段列信息同时进入 Change Set。
- 评审发现 MODIFY 新字段不能把未知原值当空字符串。测试先看到 `new_default` 为空，再增加精确读取实际行；数据库默认值与 NULL、真实 before 展示和原版本冲突保护转绿。
- 两次读取间再次变更 Schema 的测试先因未显示错误失败；增加字段名、类型、NULL、生成列的完整匹配后转绿，undefined／false 生成列标记视为等价。
- 既有同账号恢复测试发现冻结交互时不能把旧 original 重新盖回已核对基线；修复后保留既有恢复／明确重建的原值，同时保持控件、输入和记录版本规则。
- 将旧成功元数据测试中的空 `fields` 简化响应补成真实字段 fixture：配置缺失仍必须返回每个真实字段。未更改业务权限或发布执行安全快照。

## 验证结果

- 完整 Web：`pnpm --dir web test:run --maxWorkers=2`，33 文件、366 测试通过，48.77 秒。
- `pnpm --dir web build`：TypeScript 和正式生产构建通过。
- `make test`：Admin/Client/Server/Shared 单元与架构检查通过。
- HTTP/MySQL 四条：`TestFieldPolicyReadDistinguishesDisabledSchemaDriftAndFailure`、`TestReleaseDraftCASCancelAndIdempotency`、`TestPublicationCommitUnknownSurvivesExecutableRestart`、`TestRecordVersionCompareAndSwap` 全部通过，36.481 秒。原发布测试使用真实 MySQL COMMIT ACK 丢失与可执行进程重启，不用 Web mock 代替持久幂等证明。
- 真实浏览器：`TestAccountBrowserSystemPath/field-recovery.cjs` 和其必跑 accounts 主路径通过，最终稳定截图轮 39.720 秒（字段子路径 6.14 秒）；结果见 `result.json`。1440×1000 和 390×844；仅本次 Chrome 路径，全三引擎与全库 MySQL 由 #80 汇总。
- Standards / Spec 两轴独立复核无剩余阻塞；`git diff --check` 通过。首次完整 Web 364/365 的旧基线回归失败已修复，不能作为最终通过轮。

集成运行使用隔离 Colima/MySQL、localhost NO_PROXY 和本地 TCP 权限，运行结束销毁容器。原始日志、浏览器 profile、Cookie 与重复 accounts 截图保存在 `/private/tmp/rcc-77-browser-raw`，仅本目录两张精选截图与不含凭据的检查清单随仓库保存；共享 `node_modules` symlink 不提交。

## 视觉与设计维护

稳定渲染图核对：中性灰画布、石墨主按钮与共享颜色一致；桌面抽屉与底栏沿用正式组件；字段中文名、真实名、值和 NULL 开关分离；同账号恢复保留自定义值和查询输入；手机错误状态有明确文字、原请求提示及可达的重试操作；长对比在自身容器滚动，没有文档横向溢出。初次截图碰到恢复／缩放动画，采集增加现有动画完成等待，不修改产品动画。

已增量维护 `web/DESIGN.md` 的打开时真实字段与 Schema 漂移恢复约定；无新增领域概念、执行权限或持久化快照，CONTEXT／ADR 无需变更。没有新增临时恢复通道、功能开关或吞错回退；复用现有错误组件和请求 journal。
