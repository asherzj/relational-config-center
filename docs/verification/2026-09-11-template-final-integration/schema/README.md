# 正式 #98 与模板候选迁移集成验收

2026-09-11，工作树 `/private/tmp/rcc-template-notification-integration`。当前验收基于 HEAD `aec58942f01143c1a579aaf2a885141bf5a263c9` 合入已正式发布的 #98/main `eb55e841e80345cfef7733b8c560d6037256cd2c` 后的工作树；尚未合入 #104，不为 #104 后续发布语义或浏览器路径提供证据。

## 实际结果

- `01-generate.log` / `.exit`：root 已执行的真实 MySQL 累计清单生成，exit0；模板候选顺排为 10、11，分别包含 28、29 张控制表。临时生成源已从 `admin/cmd/admin` 删除，原样归档为 `manifest-generator.go.txt`；本轮未重复生成清单。
- `02-matrix.log` / `.exit` / `.sh`：真实有界初始矩阵，552.684s，exit1，42 个顶层测试中 35 PASS、7 FAIL。全部原始失败保留。
- `03-affected.log` / `.exit` / `.sh`：修正夹具后只复验受影响范围并补充正式 v9 的模板种子恢复，215.386s，exit0，12 个顶层测试全部 PASS。
- 两轮去重后 **43 个唯一顶层测试 PASS**；包含子测试共 **99 个唯一 PASS**。其中 31 个顶层结果复用本目录运行 02 的实际通过记录，12 个采用运行 03；没有引用旧版原始日志来替代当前迁移 9/10/11 的执行结果。逐项日志行号、运行输入摘要和覆盖映射见 `effective-passing-coverage.json`、`runs.json`。

所有 MySQL 服务器由测试通过 `startIntegrationMySQL` / Testcontainers 创建，镜像 `mysql:8.4`；各服务器串行启动、停止和删除。DB 名 `rcc_test` 来自该助手的 `WithDatabase`，运行日志保存每次容器身份。没有使用或修改已有 preview、开发持久库或生产库。实际命令与环境见两份 `.sh` 和 `validation-context.json`。

## 7 个原始失败如何替换

| 初始失败测试 | 失败原因与修复 | 最终实际结果 |
| --- | --- | --- |
| `TestLegacyApproverFormalCutover` | 原夹具把 8→11 当成仅有角色收缩，新增模板表触发完整快照差异。先固定 8→9，对 62 种角色／启用状态组合和其他全部事实严格断言，再显式 9→11，保留已收缩账号及其他历史事实。 | 03 PASS |
| `TestLegacyApproverCutoverRecovery` | 第 9 版恢复正确返回 pending（仍有 10/11），旧断言要求 current。第 9 版三个故障边界固定用 v9 构建验收，再单独升当前版。保留 UPDATE 整体回滚、Goose/确认边界、非法旧值拒绝及幂等断言。 | 03 PASS |
| `TestLegacyApproverInvalidDataAndMaintenanceBoundary` | 同样误把第 9 版恢复当成当前完成。明确 recover→pending、up→current；未来故障版本由实际当前版本+1生成（本次 12），继续证明未来恢复不继承 v9 的旧角色例外。 | 03 PASS |
| `TestLegacyApproverCutoverPreservesLiveResponsibilityAndCompletedApproval` | 当前 HTTP 夹具含模板表和 policy.version，直接复制到真实 v8 不兼容。仅排除明确的两张模板配置表，并按目标实际列复制其余历史事实；独立验证正式 8→9，再由正式 10/11 创建新增结构。未手改账本、未手写模板结构。完成审批、现有责任、通知进度和升级后继续审批均保留。 | 03 PASS |
| `TestSchemaBaselineAdoptsCurrentDatabaseWithoutReplayingHistory` | 冻结历史账号包括 12/2 与 31/4，合法 v9 收缩改变角色及版本；同时后续审批增量新增空通知表。baseline 仍只登记 1～5；再分别验证 5→8 旧事实不变、8→9 固定角色映射及其他事实不变、9→11 仅新增模板配置／policy 版本。冻结 SQL/JSON 不变。 | 03 PASS |
| `TestSchemaReadinessRejectsKnownOldRelease` | 旧夹具手工拼回 v1 时先删父模板表，违反新增外键。此测试本无业务数据：仅重建本测试刚创建的独占 Testcontainers `rcc_test` 空库（显式检查名称），通过正式 v1 公开迁移重新安装，保留原 Admin 进程验证 ready503，再公开 up11 后 ready200。原 FK 失败保留；重建边界写入 03 原日志。 | 03 PASS |
| `TestTemplateApprovalSchemaPreservesPublishedRolesAndChecksCompleteReadiness` | 从7直升11时将正式9角色收缩误判为模板改写。先固定升9核验唯一账号映射 31/3→27/4，并完整比较其他事实，再从正式9升11比较 pre-template 事实及当前新装完整物理结构。 | 03 PASS |

新增 `TestTemplateApprovalSchemaRecoversCatalogSeedFromFormalNine` 从真实公开命令安装的正式9开始，拒绝模板 INSERT 以制造第10版 CREATE 已提交／种子未写入的部分执行；验证账本仍为9、普通 up 拒绝、原有账号／策略／非空通知／业务行不变。授权必要 INSERT 后 recover 仅确认10并返回 pending，再显式 up11；与新装逐表比较实际结构，重复 up 后全部数据不变。该测试在 03 实际 PASS。

## 输入适用性与保留项

`02-matrix-inputs.json` 与 `03-affected-inputs.json` 记录各次启动前完整 Admin Go/SQL/JSON/module 输入 SHA-256；`final-admin-inputs.json` 与 03 完全相同。初始测试进程编译后仅编辑了四个测试文件，运行时临时构建迁移命令／Admin 使用的非测试源码、迁移 SQL 与清单全程未变。所有修改的测试函数及 `preTemplateDataSnapshot` 调用方均在 03 重跑；其他初始通过用例及实际助手不受这些变化影响。详细适用性在 `source-applicability.json`。

`preTemplateDataSnapshot` 保留所有旧表和旧策略列的完整比较，只允许模板配置表、表策略新增版本列，以及后续审批迁移新增的**空表标题**；任何非空通知、角色、成员、分配或请求行仍参与比较。正式角色收缩独立通过固定9验证，不从通用快照排除账号。

正式1～9的18份 SQL/manifest 与 `eb55e841e80345cfef7733b8c560d6037256cd2c` 逐字节相同，见 `formal-1-through-9-sha256.json` 与复核 `formal-1-through-9-post-tests.json`。本子任务没有改动生产实现或任何 SQL/manifest，没有放宽 readiness，没有改写其他 verification 目录。

完整验证范围包括新装、正式9→11及结构等价、10/11部分执行恢复、模板选择保留、通知与旧审批事实保留、只读启动／持续就绪完整结构检查、baseline固定5、表审批固定6→7、正式角色固定8→9、未知／超前版本、维护锁、并发、进程中断和当前到下一测试版本。`owned-tests-vs-formal98.diff` 为四个最终测试文件相对于正式98的差异，其中包含接入模板已有测试的差异；它不是本子任务单独新增功能的统计。`final-diff-check` 为这四个测试文件的空白检查结果，exit0。
