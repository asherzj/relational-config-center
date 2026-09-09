# #83 草稿目标占用验收记录

固定基点：`950a319669120ac3cd16af0cad68a9a6164672f3`。本票只负责父规格 #81 的 AC-002、AC-003、AC-004、AC-005；完整多表组合由 #84 接续。实现与下一票接口见 [T2 接缝](../design-notes/multitable-release-tickets/t2-targets.md)。

## 验收事实

| 规格 | 已实现行为 | 主要公开接缝证据 |
| --- | --- | --- |
| AC-002 | 零组或一组单字段/组合键；真实字段资格；默认值须显式提供；修改省略值来自真实原值；未结束明细阻止定义变更 | `TestDraftConcurrencyKeyProtectsValuesAndDefinition`、`TestDraftKeyEligibilityDefaultsAndAutoIDReferences`、`TestDraftSaveAndKeyDefinitionRaceCannotBypassReferences` |
| AC-004 | 数值、文本 collation、时间、枚举等数据库等价；NULL 与空值、组合边界和不同表相互区分 | `TestDraftConcurrencyKeyDatabaseEquality`：DECIMAL、INT、FLOAT、DOUBLE、大小写/重音、PAD SPACE、CHAR、NULL、DATETIME、TIME、TIMESTAMP、ENUM；另含组合边界与已存负 TIME |
| AC-003 | 草稿首次保存即占用；新增新值、修改旧新值、删除旧值；空草稿无目标；规则引用独立覆盖无确定主键的自增明细 | `TestDraftReservationsBeginOnSaveAndEndOnCancel`、`TestDraftKeyEligibilityDefaultsAndAutoIDReferences` |
| AC-003 与 AC-007/010 回归 | 共享业务键最后引用释放；取消、拒绝、完结、原单回滚释放主键、业务键和表引用；管理员可取消他人草稿 | `TestDraftSharedKeyReferencesReleaseOnlyAfterLastDetail`、`TestDraftAllTargetTypesReleaseOnEveryTerminalAction`；真实浏览器管理员路径 |
| AC-005 | 稳定 `detail_id`、显式明细表名、分页增量和整单版本 CAS；A/B→A/C 原子替换；失败保留原内容、占用和页面输入 | `TestDraftIncrementalEditsReplaceTargetsAtomically`、`TestDraftTargetReplacementRollsBackOnPersistenceFailure`；两窗口真实浏览器重建及键盘排序 |

没有用 Go 字符串大小写转换近似数据库相等。MySQL 按字段类型与 collation 计算权重，Adapter 对名字、NULL 标志及权重长度做无歧义组合，再使用独立命名空间生成目标键。没有自动推导或占用所有唯一索引。

规则更新、草稿保存与发布之间的互斥也由真实 MySQL 验收覆盖：规则更新先持 Table Policy 排他锁，用当前读检查未结束引用；草稿在首次一致性读前持共享锁；发布/原单回滚在读取执行元数据前取得排他锁并持有至事务结束。`TestDraftBaselineAndPublicationSerializeBeforeSnapshot` 的双向屏障先复现旧实现的两个失败，再证明“发布先，草稿等待并读到提交后的值”和“草稿先，发布等待”；独立表继续可执行。T1 的 `TestIndependentReleaseDetailGrowthDoesNotLockIndexGaps` 同时覆盖明细增长、目标和引用，未引入空范围写锁。

`TestReleaseSubmitRevalidatesBaselineAndRules` 保留外部 SQL 未更新平台版本时仍返回 409 的断言，提交不能静默改写保存的 `before`。`TestDraftSubmitRejectsChangedTargetIdentity` 在真实 `ALTER TABLE` 将管控字段从 `utf8mb4_bin` 改为 `utf8mb4_0900_ai_ci` 后，先复现错误接受提交的 200；修复后校验完整目标身份。测试还检查拒绝后申请与目标完全不变，显式重新保存时遇到新权重占用仍冲突，取消占用单后可重复同一保存请求并成功提交。

## 红绿过程与回归迁移

先从公开 HTTP 接缝获得三个明确 RED：空草稿返回 422；管控键配置字段被拒绝为 400；增量用例找不到稳定明细身份。对应最小实现逐一转绿。Web 原先提交整单 `items` 的分页编辑用例先 RED，改为 `changes` 后转绿。局部筛选运行中未选中的 Vitest 用例不作为通过证据，最终完整 Web 清单另行覆盖。

旧测试中“先创建两张相同目标草稿，再在提交时竞争”的前置条件不再合法。现在在保存时断言冲突，并继续检查整单版本、失败回滚、原目标保留、取消后同逻辑请求重试。审批/发布/完结/回滚的权限、冻结、业务值、历史、执行事实、幂等和容量断言均保留；重新准备成功时目标由新草稿持有，失败时仍属于旧审批单。

新增迁移 015 同时进入启动就绪检查、技术表保护、重复执行及全量新旧结构对比。Policy Catalog 对比使用同一隔离服务器中的独立数据库，仍对比完整结构，减少同时存在的 MySQL 容器数。冻结摘要测试显式固定单内明细身份，使独立订单的对比只变化被测执行元数据；申请中的稳定明细身份仍参与冻结。

## 执行环境与覆盖统计

本机为 macOS arm64、MySQL 8.4。`NO_PROXY` 只作用于本次 loopback 验收进程。最初四个无交集的 cmd/admin 分片共 226 个顶层测试；运行中 Colima 约 3.9 GB 内存实际发生 OOM。所有四片实际退出 1，其中 0、2 由本票主动中止减压，1、3 在中止请求前已结束；这些运行不表述为整体通过。保留其中完成的 175 项 PASS，明确重跑失败及未完成项。既有 `deploy-mysql-1` 同时因 OOM 退出，由主协调者恢复原容器；本票没有重建、删除或修改它。

随后全部数据库验收串行，任一时刻最多一个本票 MySQL 容器，包括 Adapter、HTTP 和浏览器。没有削弱用例内部的 goroutine/双连接竞争，也没有放宽业务超时。首个补齐清单为 55 项，49 PASS、6 FAIL、0 SKIP；六项均保留原失败，再按实际原因修复。最后在完整覆盖基础上针对基线锁和提交变化重验 54 项。精确命名清单、最终通过来源、各次退出码及失败轨迹由旁边的 JSON 记录提供。

最终 [机器记录](2026-09-09-draft-target-reservations.json) 收齐 cmd/admin 的 **228 个唯一顶层测试**，最后状态均为 PASS，无缺失、失败或跳过。54 项受影响批次退出 0（561.499 秒）；DDL 及相关提交/保存定向 5 项退出 0（68.050 秒）。完整 Adapter 包 **15 项**退出 0（92.705 秒），完整 HTTP integration 包 **17 项**退出 0（102.037 秒），包含各自独立的真实数据库测试及既有契约检查。

无标签 `make test` 覆盖全部 Go 模块及既有 HTTP/Application 依赖、会话写能力和路由契约检查；修改后的 Application/HTTP 包再次通过。`make build`、Web 类型检查/生产构建与 `test:dev` 完成。完整 Web 为 28 文件、329 项：本次沙箱运行 328 PASS，唯一 TCP listener 用例因 `EPERM` 失败；同文件在允许本地端口的执行环境重跑 1 PASS，不修改业务代码或跳过断言。

## 真实浏览器

通过浏览器→Vite 同源代理→认证 Admin→隔离 MySQL。完整浏览器运行的账号、未保存输入、规则说明、草稿、审批、回滚以及本票五组场景通过；原批量脚本仍读取旧 `items` 因而失败。它改为检查仅修改项的 `changes.upserts` 和删除的稳定 ID，同时保留未触及明细、1000 项真实发布与桌面/390px 分页断言。聚焦补跑 `release-batches.cjs` 与 `draft-targets.cjs` 整体退出 0，81.410 秒。

本票浏览器还故障注入首次字段/目录读取，手动重试后保留选择和标题；检查必填错误语义、辅助按钮不会提交、64 字符真实字段在 390px 换行；第 21 项只发送本次变化；相等业务键冲突显示表、占用单及申请人；两个窗口重建只覆盖本次修改，保留另一窗口未触及明细；管理员取消他人遗留草稿后释放表引用。

视口截图回读另发现草稿编辑器的 `fieldset` 默认最小内容宽度会让 64 字符字段撑宽内层输入，虽然文档总宽度正常。局部增加可收缩宽度和字段名换行后，浏览器新增实际输入左右边界断言，最终本票路径退出 0（53.572 秒），相关组件文件 52 项及 Web 类型检查/生产构建再次通过。没有以隐藏或裁切内容规避布局要求。

逐图回读了 [390px 管控键配置](2026-09-09-draft-target-reservations/draft-key-config-mobile.png)、[桌面目标冲突](2026-09-09-draft-target-reservations/draft-target-conflict-desktop.png)、[390px 版本冲突](2026-09-09-draft-target-reservations/draft-version-conflict-mobile.png) 与 [管理员取消](2026-09-09-draft-target-reservations/draft-admin-cancel.png)。最终图片展示真实错误和恢复操作，长字段及错误说明在视口内换行，输入框两侧与底部动作可见，没有横向撑出。每张图片的摘要记录在 JSON 中。

## 独立评审与交接

两位原 reviewer 独立重读 live #81/#83、固定基点以来的 tracked/untracked diff。Standards 的默认 submit 按钮、标题必填说明、失败读取手动恢复和长字段移动布局发现均已修复并由原 reviewer 关闭；最终成文规范违规 0、主观坏味道 0。Spec 对锁顺序、提交原值校验、完整目标替换和范围/退出责任再次复核，未解决 0。最后的 DDL 身份增量也已由两位原 reviewer 独立复核，均为 0 未解决、无最高严重级别。

本票没有新增兼容、双写或临时发布路径。`items` 明确保留为支持的原子整单替换接口，`changes` 是增量接口，二者互斥。现有 T1 过渡外观沿用 #84/#86/#88 的退出责任；T3 必须按稳定顺序取得所有相关表的规则/执行锁，之后才建立任何多表基线快照。本票只做一个本地提交，主协调者接入、更新工单与最终项目记录。
