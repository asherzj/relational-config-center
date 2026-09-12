# 发布流程模板最终验收（#106）

本票实现与验收已完成，等待 root 的独立交付 gate；尚未提交、推送、更新 Notion 或关闭 #106。

固定起点为 `0edb62afde567242e7c734d37d20b834aa0df983`，隔离分支为 `codex/issue-106-release-template-final`。#101–105 的交付均在该起点祖先中。当前代码快照及未经修剪的完整差异见 `source-*.json` 和 `source-*.diff`；每一轮命令、结果与适用快照见 `runs.json`，独立浏览器路径另见 `go-browser-runs.json`。

## 本票变化

- 删除旧固定 `ReleaseProgress` 组件及专用样式。包括已回滚在内的所有状态统一显示整单阶段，逐表流程继续读取已保存实例；取消、拒绝和回滚不能使未执行节点显示为已完成。
- 新增真实取消及拒绝浏览器路径：保留必填取消原因、键盘确认、两表独立节点、真实操作者/时间和桌面/390px 布局断言。回滚页使用真实恢复实例与整单阶段。
- 更新当前使用、升级、迁移和设计文档，说明常规/应急提交、原单恢复、人工完结与正式 Goose 1–11 边界。
- 修复完整入口发现的验收夹具交点。发布流程配置由隔离场景显式调用现有 HTTP 或管理应用接口；没有运行时自动补配置、默认流程执行兜底或 readiness 放宽。

## 结构与旧流程退出

正式前缀是 1–11，历史接管固定为 5。10/11 的 SQL 和累计 manifest 沿用此前真实 MySQL 生成、已纳入起点的原始内容，不为退出候选状态重写 SQL、manifest 或版本账本。相对正式 9，配置只新增 `rcc_release_templates` 和 `rcc_table_release_templates`，节点实例保存在既有发布单文档中。候选退出表示本次交付确认的正式结构，不表示已部署生产。

`inherited-boundary-check.json` 按 Git blob 验证 2183 个既有 verification 和迁移 SQL/JSON 文件原字节不变。历史 Contract＋016 的严格结构比较固定在同一正式前缀 5；当前 11 的新装/升级等价由当前 schema 集成测试证明，不能用历史比较替代。

旧全局 APPROVER 的授权退出、独立回滚接口退出、终态拒绝再回滚、缺常规配置必须显式保存及真实应急待发布等，依据本轮公开 HTTP/真实 MySQL 用例和浏览器路径。字符串搜索仅用于发现残留，不代替行为证据。

## 运行边界与原始失败

环境为 macOS、MySQL 8.4（Colima Docker）、本 worktree 的本地 pnpm 依赖。Docker socket、Go cache 与版本见 `environment.json`、`runs.json`。最多同时运行两个独立随机 MySQL；既有预览服务与用户数据库不在测试范围。所需本地 TCP 和 Docker 命令使用授权的外部沙箱执行。没有声明 Linux CI 或生产部署通过。

完整 Go/Web/MySQL/正式浏览器入口各启动一次。首轮失败保持失败状态，后续只执行失败或未执行范围；不会将被 `&&` 或 shell `set -e` 跳过的模块计为通过。首轮 Go/Web 的本地 TCP 沙箱拒绝，已经通过有本地监听权限的窄复验处理；Web 原 unhandled error 随同对应真实 TCP 用例复验。

完整回归发现的其他交点包括：新外键阻止旧故障夹具 DROP/清理；旧正向发布夹具缺少显式 STANDARD 关联；历史版本与 current11 比较错位；列表字符串断言误认合法节点编码；动态浏览器夹具先于账号授权而使 ready 正确拒绝；已有表关联被版本0重复创建；WebKit 场景尚未进入目标表便打开抽屉，以及持久结果已出现但原包重推仍处理中便提前断言。基础设施身份测试另通过真实注册与授权后的管理应用调用补齐两种关联；HTTP 错误映射测试的内存查询适配器不实现管理事务，因此改为显式提供读取夹具，保留认证查询的 504/503/500 与脱敏断言。该内存场景不替代真实事务验收。每个原日志、截图和后续窄复验均保留，最终对应关系见结果索引。

## 有效结果与适用范围

| 验收范围 | 最终有效结果 | 原始尝试与边界 |
| --- | --- | --- |
| Go 四模块单元与构建 | 全部通过 | 首轮 Admin 本地 TCP 权限导致4项失败；仅复验这些测试并执行首轮未运行的 client/server/shared；`make build` 成功 |
| Web test:run | 480 个唯一测试有效通过 | 原478通过、2失败及1 unhandled error；98项受影响文件窄复验通过且无 unhandled error |
| Web test:dev/typecheck/build | 全部通过 | 各命令独立原始日志；依赖安装在本 worktree 中 |
| MySQL 完整集成 | 11 个包清单，431/431 测试身份有效通过；0 skip、0 未执行 | 原完整入口失败后按 package+test 拼合；详细清单 `integration-results.json` |
| 正式 shell 浏览器 | 34/34 精确 case 有效通过 | 七次 runner 中五次失败、两次成功；原失败没有改记成功 |
| 独立 Go 浏览器 | 12/12 路径有效通过 | 每次全新输出目录；原 Flow、ApprovalFinal、Account 尝试失败保留 |
| 官方 Compose | 6/6 场景通过 | 首次本地路径未共享失败；同一 source12 原字节共享镜像六项通过并清理 |

完整 MySQL 首轮覆盖所有包，cmd/admin 在原定 60 分钟包上限触发超时；当时单项 `TestSchemaReadinessRejectsKnownOldRelease` 只运行13秒，并非该测试运行一小时。29 个后续 cmd/admin 测试未执行，其他包随后完成。首轮共376个 PASS、25个 FAIL、1个中断和29个未执行；25个失败经过定向夹具修复复验，30个中断/未执行测试单独续跑全部通过。完整原日志保留为失败，不声称首轮整套通过。当前 schema 新装、升级、只读 readiness、部分恢复、正式9→10→11 数据保留均在本次真实 MySQL 结果中。

Compose 首次正式迁移已到11，随后 Colima 无法读取未共享的 `/private/tmp` bind fixture，Admin 正确没有启动。第二次将 source12 的666个文件逐项校验后复制到已授权、Docker 共享的 HOME 可写目录，运行未改动的官方脚本和 Compose 配置，记录实际 bind 解析路径；新装、重复启动、未接管阻断、显式接管、未确认阻断和显式恢复六项均通过。镜像运行前后哈希一致，随机 project/volume 与镜像已清理。此环境修正没有改迁移、SQL、部署配置或运行时约束。

最终冻结源码是 `source-14.json`，完整未经修剪差异是 `source-14.diff`。source13/14 仅追加测试夹具修复，生产 Go 与 Compose 输入保持 source12 字节一致；相应 identity 和 HTTP 范围已在新快照窄复验。浏览器、Web、生产构建与其他已通过测试仍适用于其未变化的源码。`source-final-check.json` 验证全部666路径，`resource-cleanup.json` 验证所有自有容器/Compose卷退出及原有两个容器仍运行。原完整超时遗留的一个自有 Testcontainer 按原日志 ID 与 session 标签精确清理，未删除用户数据。

## 浏览器与视觉证据

`browser-matrix.json` 对应正式 shell 的 34 个精确 case，每项保留尝试历史及有效通过来源。12 个独立 Go BrowserSystemPath 每项使用全新 `RCC_E2E_OUTPUT` 和随机真实服务，不复用其他工单的旧证据。真实场景涵盖模板管理、表关联、多表常规/应急、部分与全部审批、手动发布、人工完结或原单恢复、通知、错误、会话恢复和原包重推。

34 项通过是按 case 拼合的有效结果，不将失败的整次 runner 改记成功。`browser-runner-integrity.json` 逐次核对：所有七次 runner 的 `cleanup verified` 为 true，`database-final.txt` 均满足既有全局夹具计数/规则校验；失败 runner 没有执行末尾全行 before/after 比较。只有成功的05和07各自在其选定范围执行了完整行比较，且前后完全一致。不能用后两次的行比较声称前次失败批次执行过同一检查；各业务 case 的实际断言仍按原始结果适用。

`visual-inspection.json` 记录实际打开检查的原图路径、SHA256 与所见事实。桌面和390px取消/拒绝四张原图及原单回滚图显示真实终态；停止节点没有伪造完成或操作者时间。长内容的局部表格滚动与整页无横向溢出分别核对。焦点和会话边界仍以相应真实交互结果共同判断。

## 交付责任

逐 AC 责任、原交付版本和本轮适用证据在 `ac-index.json`，不将旧票记录回写为本轮新结果。Standards 与 Spec 由独立只读审查者分别对固定快照审查，报告在 `reviews/`。最终源码、删除项和原始证据精确清单供 root gate 核验；root 在通过后负责提交、推送及远端核验、Notion 总体记录与 #106 关闭。父 #100、主分支合并和部署不在本次授权收尾动作中。
