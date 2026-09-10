# 字段录入 #75 验收

基点 `0ebc913`；规格 #71、工单 #75；设计沿用 `web/DESIGN.md`。本工单仅接入记录新增／修改，不改变业务接口权限、发布执行快照或历史显示。

## 证据分层

| 用例 | 接缝与证据 |
| --- | --- |
| AC-008 | `FieldValueInput.test.tsx`：八控件中的文本／多行／数字／日期／日期时间，以及编辑器测试中的布尔／下拉／单选；保留大整数、高精度小数、MySQL datetime 和 UTC timestamp 微秒；单行输入粘贴 CRLF 不丢字符 |
| AC-009 | 编辑器验证新增预填、未包含字段省略、显式 NULL 与空字符串；真实 MySQL `TestFieldPolicyAC009AC018KeepDatabaseDefaultsAndExecutionBoundary` 经发布后查询验证数据库默认值、NULL 与空字符串 |
| AC-010 | 编辑器验证必填空值阻止提交、0／false 有效、新增不可编辑字段隐藏且不提交预填、修改只读原值不提交；精确范围与步长含 `.5`、`.1` 等合法字符串 |
| AC-011、AC-007 编辑部分 | `field-inputs.cjs` 真实浏览器新增选择自定义值并保存草稿；修改其他字段时保留旧下拉值；组件验证选择中文静态选项后提交实际值、单选旧值可见且只能主动选择替换 |
| AC-018 | 新 HTTP 测试配置隐藏、不可编辑、必填和静态选项后，合法自定义值／空字符串仍可直接发布；类型、生成列、未授权 MODIFY 仍按原错误码拒绝；既有 defaults/nullability/Auto Fill HTTP 回归通过 |
| AC-003、AC-015 编辑部分 | `ConfiguredRowEditor.test.tsx` 验证读取失败明确报错并重试后才展示编辑器；编辑器用 disabled／incompatible effective 配置验证文本回退、警告和主键／生成列／自动填写排除 |
| AC-012、AC-019 配套 | 浏览器验证已打开表单不随管理员保存配置重绘、重新打开采用最新配置；完整 Web 回归和既有 accounts 浏览器路径继续覆盖同账号恢复、记录冲突、原请求重试与审批／发布 |
| AC-022 编辑部分 | 1440×1000 桌面与 390×844 手机，Tab、自定义值、未保存退出、草稿保存、操作可达、无页面横向溢出；见两张截图与 `result.json` |

## 运行结果

- `pnpm --dir web test:run --maxWorkers=2`：32 文件、347 测试通过。
- `pnpm --dir web build`：类型检查与正式构建通过。
- `make test`：Admin、Client、Server、Shared 完整 Go 单元／既有架构检查通过。
- `go -C admin test -count=1 -timeout=8m -tags=integration ./cmd/admin -run '^(TestFieldPolicyAC009AC018KeepDatabaseDefaultsAndExecutionBoundary|TestMutationPolicyUsesDefaultsAndNullabilityAndRejectsInvalidInputFields|TestMutationPolicyAutoFillUsesAccountOperatorAndDatabaseTimeAndRejectsManagedInput)$'`：通过，46.982 秒。
- `go -C admin test -v -count=1 -timeout=8m -tags=integration,browser ./cmd/admin -run '^TestAccountBrowserSystemPath$/^field-inputs.cjs$'`：真实 Admin／Vite／MySQL 与 Chrome 通过，58.097 秒；包含原 accounts 主路径，字段录入子路径 8.55 秒；临时容器已清理。
- 两轴独立评审：Standards、Spec 均无剩余阻塞项。
- `git diff --check` 通过。

本机集成测试使用 Colima Docker socket、localhost NO_PROXY；指定 `RCC_E2E_OUTPUT` 前创建输出目录。浏览器脚本不改原有五行业务 fixture，仅创建隔离测试库中的配置及草稿。完整三引擎 CI 与跨工单联验由 #80 汇总，不把此次局部浏览器结果描述为全套系统验收。

验收先行记录：新增数字控件／预填测试首先因原编辑器无配置控件和预填失败；配置读取失败／重试用例在加载组件不存在时失败；微秒逐字输入测试发现中间态切换控件问题；精确步长、单行 CRLF 粘贴和前导小数点约束分别先红，再经实现转绿。

## 实际视觉核对

已打开最终截图逐项检查：

1. 桌面复用 640px 抽屉，固定标题／底栏、内容独立滚动；手机全宽且长表名换行。
2. 中文字段名、真实名、类型、NULL 与必填标记成组；辅助说明不散落到录入列。
3. 中性灰、石墨按钮与既有边框／圆角一致，彩色仅用于错误状态。
4. 下拉、自定义文本与 NULL 开关清晰分离，手机号宽度下无页面横向溢出。
5. Tab 后焦点环可见，手机取消与 Change Set 操作保持可达。
6. 修正必填输入后旧错误即时清除，最终桌面截图不再误示必填错误。

无新领域概念或执行权限变化，因此无需修改 CONTEXT／ADR；已增量维护 `web/DESIGN.md` 和字段管理设计说明。无临时功能开关、并行编辑器或显示快照。
