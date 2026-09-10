# 字段交互完整路径与 AC 证据索引

规格 [#71](https://github.com/asherzj/relational-config-center/issues/71)，交付工单 [#80](https://github.com/asherzj/relational-config-center/issues/80)。#80 固定基点 `511b085`，已包含 #74～#79 的独立提交；按用户最终约定汇总为一个合并请求。既定设计依据为 [web/DESIGN.md](../../../web/DESIGN.md) 和 [字段管理设计](../../design-notes/field-policy-management.md)，不重新选择视觉方向。

## 22 条独立验收映射

表内每条用例分别指出主接缝及已有证据。组件、HTTP/MySQL 和系统路径各自证明对应承诺，不将一次浏览器成功等同于全控件矩阵或全部后端边界通过。

| AC | 主接缝／精确测试入口 | 可复核证据 |
| --- | --- | --- |
| AC-001 | `field_policy_integration_test.go` / `TestFieldPolicyAC001RoundTrip`：完整保存、重读、同表字段唯一 | [#74 管理记录](../2026-09-09-field-policy-management.md)，[集成结果](../2026-09-09-field-policy-integration-results.json) |
| AC-002 | 同文件 `TestFieldPolicyAC002InvalidConfigurationIsAtomic`、`TestFieldPolicyDatabaseFailureRollsBackWholeReplacement`：非管理员、非法配置与事务回滚 | [#74 管理记录](../2026-09-09-field-policy-management.md) |
| AC-003 | `ConfiguredRowEditor.test.tsx`、`CombinedQueryForm.test.tsx`、`ManagedRowEditor.test.tsx`：新增列／停用回退 text+exact，原主键／自动填写约束保留 | [#77 恢复记录](../2026-09-09-field-recovery/README.md) |
| AC-004 | `combined-query.cjs`：全部可查询字段直接呈现、折叠保留、两个条件真实 AND 返回 Gamma | [#76 浏览器结果](../2026-09-09-combined-query/result.json)，[说明](../2026-09-09-combined-query/README.md) |
| AC-005 | `CombinedQueryForm.test.tsx`：显式运算符、单一运算符隐藏、单值／集合／区间／无值形状 | [#76 组件验证](../2026-09-09-combined-query/README.md) |
| AC-006 | `CombinedQueryForm.test.tsx`：同形状保留、跨形状清空、未填省略、零／否有效 | [#76 组件验证](../2026-09-09-combined-query/README.md) |
| AC-007 | `FieldValueInput.test.tsx`、`ManagedRowEditor.test.tsx`、`CombinedQueryForm.test.tsx` 与 `field-inputs.cjs`／`combined-query.cjs`：静态实际值、自定义查询／编辑、radio/boolean 限制 | [#75 录入](../2026-09-09-field-inputs/README.md)，[#76 查询](../2026-09-09-combined-query/README.md) |
| AC-008 | `FieldValueInput.test.tsx`、`ManagedRowEditor.test.tsx`：八控件、无损整数／小数／日期时间微秒、CRLF | [#75 模块验证](../2026-09-09-field-inputs/README.md) |
| AC-009 | `ManagedRowEditor.test.tsx` + `TestFieldPolicyAC009AC018KeepDatabaseDefaultsAndExecutionBoundary`：预填、省略、数据库默认、NULL／空字符串 | [#75 Web 与 MySQL 验证](../2026-09-09-field-inputs/README.md) |
| AC-010 | `ManagedRowEditor.test.tsx`：必填空值阻止、0/false有效、新增不可编辑字段不提交、修改只读原值 | [#75 组件验证](../2026-09-09-field-inputs/README.md) |
| AC-011 | `field-inputs.cjs`：修改其他字段并真实保存草稿，旧下拉值保持为自定义值 | [#75 浏览器结果](../2026-09-09-field-inputs/result.json) |
| AC-012 | `ManagedDataPage.test.tsx`／`ManagedDataMutationPage.test.tsx` 与 `field-recovery.cjs`：同账号恢复旧 DOM／输入，重开取新规则，切换身份隔离 | [#77 恢复验证](../2026-09-09-field-recovery/README.md) |
| AC-013 | `ManagedDataPage.test.tsx`、`ChangeSetDialog.test.tsx`、`ReleaseOrdersPage.test.tsx`：列表可见与排序、删除／审阅完整、重复标签保留原名原值 | [#78 显示验证](../2026-09-09-field-display/README.md) |
| AC-014 | `ReleaseOrdersPage.test.tsx` + `field-display.cjs`：真实发布 before/final 在改标签后逐字不变，申请与实际结果实时显示／回退 | [#78 浏览器结果](../2026-09-09-field-display/result.json)，[说明](../2026-09-09-field-display/README.md) |
| AC-015 | `TestFieldPolicyReadDistinguishesDisabledSchemaDriftAndFailure`、`ConfiguredRowEditor.test.tsx`、`CombinedQueryForm.test.tsx`：503与重试、失配警告、真实 Schema 漂移／当前行重读 | [#77 契约与 Web 集成](../2026-09-09-field-recovery/README.md) |
| AC-016 | `TestCombinedQueryAC016RealFieldsCapacityAndAtomicConfiguration`：真实257列能力超限，关闭一字段后256条件／range／256×100 IN通过 | [#76 MySQL 结果与容量依据](../2026-09-09-combined-query/README.md) |
| AC-017 | `CombinedQueryForm.test.tsx` + `TestQueryPolicyEnforcesLimitsAndRequestSortWithoutCorrection`：IN100允许／101拒绝、分页和请求体边界保留 | [#76 契约验证](../2026-09-09-combined-query/README.md) |
| AC-018 | `TestFieldPolicyAC009AC018KeepDatabaseDefaultsAndExecutionBoundary` + 既有 defaults/nullability/Auto Fill HTTP 测试：交互标志不新增业务授权，原执行校验保留 | [#75 HTTP/MySQL 验证](../2026-09-09-field-inputs/README.md) |
| AC-019 | `field-recovery.cjs`、`accounts.mjs`、`TestReleaseDraftCASCancelAndIdempotency`、`TestPublicationCommitUnknownSurvivesExecutableRestart`、`TestRecordVersionCompareAndSwap`：原键／正文、真实 COMMIT ACK 丢失、版本冲突不覆盖 | [#77 回归证据](../2026-09-09-field-recovery/README.md)；本轮发布回归见下文 |
| AC-020 | `TestFieldPolicyAC020MigrationPreservesExistingData`：空库与014升级两次结构等价、旧草稿与业务行保留 | [#74 迁移验证](../2026-09-09-field-policy-management.md) |
| AC-021 | `release_reset_integration_test.go` / 三个 `TestReleaseReset*`：实际维护进程、五表原子清理、中断回滚重跑、业务/版本/游标保留及下一次发布连续 | [#79 隔离重置记录](../2026-09-09-release-reset.md) |
| AC-022 | `field-interactions.cjs`：每引擎桌面1440×1000和390×844真实管理员配置→组合查询→自定义值修改→Change Set→持久草稿审阅 | 本目录最终浏览器结果与截图；执行结果见下文 |

测试源码位于 [Web 录入／查询](../../../web/src/features/managed-data/)、[发布审阅](../../../web/src/features/release-orders/)、[系统路径](../../../web/e2e/)、[Admin HTTP/MySQL](../../../admin/cmd/admin/)。

## 完整路径与工程检查

- 最终字段流程：`RCC_E2E_SUITE=field-interactions RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance` 通过，2026-09-09 12:29:46～12:30:40 UTC。每引擎 6 项检查，分别覆盖 1440×1000、390×844，合计 18 项；页面错误均为零，字段配置还原、业务 fixture 比对、服务/容器清理均通过。详见 [Chromium](chromium-result.json)、[Firefox](firefox-result.json)、[WebKit](webkit-result.json)。
- 最终 Web：`pnpm --dir web test:run --maxWorkers=2`，35 文件 / **375 测试通过**，72.04 秒。生产构建（含类型检查）通过；`pnpm --dir web test:dev` 两项端口与 Origin 检查通过；脚本语法与 `git diff --check` 通过。
- 既有浏览器回归采用分套件验证：首次 all 运行的未保存、规则说明、写入恢复、操作覆盖通过；复杂字段修正旧脚本后 **42 项通过**；三引擎可访问性各 **7 项通过**；草稿、独立审批、混合批量、三引擎发布/回滚/恢复全套通过。每次均采用独立 MySQL，套件内 fixture 与资源清理检查通过；成功完成的独立复验亦通过运行器数据库最终比对。后续 #80 控件修复由最终三引擎字段流程及完整 Web 验证。各轮命令、时间、退出状态、断言与原始产物摘要见 [浏览器回归汇总](browser-regression-summary.json)。
- Go 单元、架构检查与全部模块构建已在 #79 最终代码通过。完整 MySQL 回归 `go -C admin test -json -count=1 -timeout=60m -tags=integration ./...` 在全部 Go 提交 `511b085` 上退出 0，**286 项主测试、279 项子测试通过，零失败、零测试跳过**。其中 `cmd/admin` 236 项主测试，Go 报告耗时 2664.964 秒；4 个包无测试文件。记录起止为 2026-09-09 20:07:57～23:47:40 +08:00，日志时间包含间隔，与 Go 的包运行耗时分别记录。完整包结果、用例和原始日志摘要见 [MySQL 汇总](mysql-summary.json)；#80 未修改 Go 执行逻辑。
- Standards 与 Spec 独立审查的代码发现均为零；Spec 对最终 AC-022 三引擎结果、原始产物及四张截图的定向复核为零项发现。

原始浏览器 profile、Cookie 与服务日志仅保留任务临时目录。本目录保存最终三引擎结果和四张原始精选截图；主代理已逐张目视核对：桌面静态选项与实际编码成组、Change Set 三列完整对比；手机抽屉底部操作可见，对比表焦点环清晰、横向内容由局部滚动查看，无文档横向溢出。

截图：[桌面字段配置](desktop-configuration.png)、[桌面 Change Set](desktop-change-set.png)、[390px 编辑器](mobile-editor.png)、[390px 键盘审阅](mobile-keyboard-review.png)。

## 失败轮与修正

首次字段流程因本机 Docker 访问被拒绝在预检退出，不算产品 red。使用正确隔离环境后，真实浏览器确认必填失败未聚焦字段；修复仅在用户请求审阅失败时定位首错，不在输入修正中跳焦点。WebKit 的原生 focus 滚动在下一帧完成，脚本改为等待真实几何条件；另确认 WebKit 的已聚焦窄屏对比表不能凭原生左右键滚动，共享 Table 增加仅容器自身聚焦且无修饰键时的左右滚动。局部测试先因 scrollLeft 为 0 失败，修后验证容器滚动、表内输入和快捷键保留；三引擎最终完整路径通过。脚本只有完整路径与取消/清理执行结束才记录成功。

首次旧 all 的复杂字段测试有 3 个失败：旧 `.fill()` 向默认单行文本填入多行时被浏览器压平，及旧脚本查找现在已隐藏的两个生成列新增控件。改为真实粘贴触发多行交互，并分别验证生成列无新增输入且非法直接 HTTP 请求仍返回 422；原精确草稿/SQL/readback和拒绝断言保留，定向 42 项复验通过。首次 all 失败不记作全套通过。

旧 PR #89 的可访问性脚本假设焦点先到标题、并把同一滚动节点当成子节点，现按实际键盘滚动区域与标题顺序校验，三个引擎通过。旧 MySQL CI 40 分钟耗尽时没有断言失败，停在第 222 个测试的容器启动阶段；本工单将整套 Go 时限调整到 60 分钟、CI job 65 分钟，单个数据库 fixture 时限保持不变。此前超时不作为最终完整 MySQL 通过依据。

## 设计核对与临时结构

沿用正式 shadcn/ui、Tailwind tokens、现有 Drawer、Change Set、发布审阅与未保存保护。没有新增交互配置快照、执行权限、业务表、旧查询／编辑并行实现、双写或 Feature Flag。旧逐条查询入口已在 #76 删除；原发布执行 Schema、语义快照、记录版本、幂等与回滚仍保留。014 仅新增字段规则表；独立旧发布单重置没有加入迁移或启动流程，具体隔离库与中断恢复由 #79 真实进程验证。
