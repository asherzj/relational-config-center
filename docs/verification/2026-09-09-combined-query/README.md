# 组合查询 #76 验收

基点 `aaed086`，规格 #71、工单 #76；沿用 `web/DESIGN.md`，前置 #74/#75 已集成。本工单接入组合筛选及统一查询容量，不改变 Query/Mutation Policy 执行权限、发布与回滚安全快照或记录版本。

## 验收接缝与证据

| 用例 | 证据 |
| --- | --- |
| AC-004 | `combined-query.cjs` 真实管理员保存字段配置后打开正式 Web；不可查询 note 不出现；category=notice 与 priority>=20 一次 AND 查询仅返回 Gamma；折叠、执行查询后输入仍保留 |
| AC-005/006 | `CombinedQueryForm.test.tsx` 按显式运算符切换单值/集合/范围/无值；同形状保留、跨形状清空；未填省略、0/false有效、单边范围；显式空字符串、NULL、不筛选分别提交；首次空选项未被误示选中；清空datetime和删除集合前项均不复用旧中间态 |
| AC-007 查询部分 | 组件与浏览器选择中文选项提交实际值、IN自定义digest真实返回Alpha/Epsilon/Gamma；查询前后目录不变；radio不能自由输入，空radio可首次主动选择；完整用例同时引用 #75 [录入验证](../2026-09-09-field-inputs/README.md)及本轮field-inputs真实浏览器回归 |
| AC-016 | `TestCombinedQueryAC016RealFieldsCapacityAndAtomicConfiguration` 创建257列真实MySQL表；GET默认计257并supported=false，清空配置拒绝；仅保存f256关闭查询，剩余256默认字段可一次提交且双边range计一条；256×100 IN亦通过；停用使计数超限，拒绝后原配置不变。Web组件一次提交21字段，能力超限时禁用并说明 |
| AC-017 | 组件100个值提交、101报错且不发送；`TestQueryPolicyEnforcesLimitsAndRequestSortWithoutCorrection` 真实HTTP/MySQL验证256通过/257拒绝、IN100通过/101拒绝、分页和Offset边界保留；已有HTTP安全回归保留1 MiB拒绝 |
| AC-015 基础 | 配置请求失败明确错误与重试，未成功不暴露默认字段；incompatible显示警告、使用effective文本回退。错误/会话/重开进一步联验由 #77 完成 |
| AC-022 查询部分 | 1440×1000和390×844真实Chrome：键盘焦点可达、筛选区局部滚动、无文档横向溢出；查询与清空始终在字段滚动区之外；本轮额外执行 #75 编辑路径和既有accounts路径 |

## 容量决策

上限256，IN/NOT IN每条100，范围一条；HTTP1 MiB、Page Size200、Offset10,000及原查询超时保留。256×100=25,600值参数，加分页仍低于MySQL65,535占位符限制；真实最坏集合请求为113,570字节（约111 KiB）。不是对任意长度值可一次塞入1 MiB的承诺。配置保存、读取和Web取同一effective容量，不仅计算配置行；配置缺失、停用、失配均按回退后的真实字段计数。完整决策与官方依据见[字段契约](../../admin-field-policies.md#组合查询容量)。

## 先红后绿

- HTTP先把旧20边界验收改为256：原实现400 `invalid_query_condition`；更改统一常量后256通过、257拒绝，旧IN/分页边界仍通过。
- 真实257列能力测试先因缺少query_capacity失败；同一effective结果用于读/保存后通过，拒绝保存保持配置不变。
- 直接字段表单测试在组件不存在时失败；第一片直接显示、AND、折叠保留通过；运算符片因无选择器先红，再补形状切换。
- 21字段Web提交先因旧20校验失败，再改用能力返回值转绿。
- 集合删除测试发现日期控件按索引复用导致空白后项显示被删除的日期；改稳定集合项key后转绿。
- 独立评审补查首次空radio/空select以及查询/编辑同时存在时名称冲突；显式未选择状态和查询可访问名前缀修复后复验。

## 验证结果

- 完整 Web：`pnpm --dir web test:run --maxWorkers=2`，33文件、356测试通过（53.23s；包含最终稳定集合项key与中文标签变更）。
- `pnpm --dir web build`：类型检查和生产构建通过。
- `make test`：Admin/Client/Server/Shared及架构检查通过。
- 真实HTTP/MySQL容量/IN/分页测试：`TestCombinedQueryAC016RealFieldsCapacityAndAtomicConfiguration`（19.937s）和`TestQueryPolicyEnforcesLimitsAndRequestSortWithoutCorrection`（12.585s）通过。
- `TestAccountBrowserSystemPath/field-inputs.cjs` + `/combined-query.cjs` 与每轮既有accounts主路径：通过（56.025s），最终视觉调整后的 `/combined-query.cjs` 与既有accounts主路径再次通过（49.844s；子路径3.07s）。
- 桌面/手机截图见本目录，`result.json`仅保留浏览器版本与检查清单。原始浏览器profile、Cookie数据库及重复accounts截图全部移到`/private/tmp/rcc-76-browser-raw-artifacts`，不随仓库提交。
- 独立 Standards/Spec 评审均无剩余阻塞（汇总代理确认）；工单级单commit和合并请求由汇总代理统一收尾，本分支未自行提交或推送。

首次完整Web在sandbox下354/355通过，唯一client-stream本地TCP监听被EPERM拒绝；随后以escalated权限补验该测试并重新完整执行，不能把该失败轮作为全套通过依据。

## 视觉核对

已实际打开桌面和手机渲染图：灰色画布与石墨按钮延续设计；字段卡片桌面三列/手机单列；中文名、真实字段名与类型可核对；字段元数据及单运算符采用轻量文字，避免灰色提示块堆叠；范围边界和集合各项有可见标签；筛选区局部滚动，排序/分页/查询在滚动区外保持可达。首次手机截图捕到侧栏折叠动画中间帧，脚本改为等待侧栏真正隐藏后采集最终图。无新领域概念；增量维护了 `web/DESIGN.md`、字段HTTP契约、技术基线及ADR查询容量条目。

旧逐条添加/删除条件入口、20条件校验与无人使用的旧布局样式已删除。旧operation-coverage/complex-fields系统路径迁移到直接字段查询，编辑器操作保持原作用域。全套三引擎浏览器与全库MySQL回归由 #80 汇总；本记录不将本轮局部Chrome测试描述为三引擎全套验收。
