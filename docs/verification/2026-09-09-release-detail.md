# 发布单完整详情交互（#64）

状态：本地实现与验收完成；提交、远端核验、Notion 和工单关闭由协调者串行执行。父规格 [#59](https://github.com/asherzj/relational-config-center/issues/59)，本单覆盖 AC-012、013、014、016，沿用 D-001～D-017、灰/石墨主题和真实 HTTP/MySQL 接口。

## 验收结果

| 验收 | 已实现与证据 |
| --- | --- |
| AC-012 | MODIFY 默认隐藏申请未提交及确定同值字段，自动填写/数据库生成说明继续显示；完整视图区分 SQL NULL、JSON null、空串、未提交和不存在。ADD/DELETE 完整可读。20项分页、首项展开、千项定位保持整单操作；真实部分更新验证过滤往返。 |
| AC-013 | 成功初次默认可信数据库发布结果，可切回申请差异；原发布结果与关联恢复结果分开。恢复结果取真实反向单 publication，不从申请内容推测实际值。 |
| AC-014 | 四阶段按八种合法状态显示；取消、拒绝、回滚不伪装为正常四步成功。D-017 准备节点取 SUBMIT，审批/发布/完结显示对应实际事件人员和时间；快速回滚不伪造 APPROVE。真实姓名与永久 ID、32hex 单号复制、中文最近五条历史/全部及关联跳转通过。 |
| AC-016 | 真实原申请人当前 EDITOR+PUBLISHER，APPROVED 同时允许 cancel/execute/reprepare；提交前未知后旧单仍 APPROVED，关闭/刷新/撤权恢复不释放同单冲突写。原 body/key 同时覆盖提交前及提交后丢响应，最终只创建一个新草稿。明确拒绝须审阅最新基线再重建；会话与账号隔离保持。 |
| 键盘与窄屏 | 390px 无整页横向溢出；native summary Enter/Space 可操作，Tab 可到实际字段横滚容器，ArrowRight 展示完整申请值列。局部表格具中文名称与焦点环，共享 Table 其他调用默认行为不变。 |

“仅看变更”表示申请中修改的字段，不承诺触发器或 ON UPDATE 后的最终值。生成值仍依实际 publication 审阅。所有执行、取消和恢复操作始终针对整单，分页不缩小写入范围。

## 最终验证

| 检查 | 最终结果 |
| --- | --- |
| `make test` / `make build` | exit0；23.728s / 2.357s |
| 原完整 `make test-integration` | 533 PASS / 0 FAIL / 0 SKIP；2084.226s；exit0 |
| Web 全量 / typecheck / build / test:dev | 330项 / 28文件，21.60s；全部exit0；test:dev 2项 |
| 正式 `make test-browser` | V7代码，运行v4；142.636s，exit0 |
| 正式 `RCC_E2E_SUITE=all RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance` | 17套件、244条真实命名检查；488.990s，exit0；cleanup、fixture一致、数据库postcheck全部通过 |

所有正式 runner 由协调者持有至最终退出，未调大超时、删减原范围、弱化业务断言或用测试桩替代数据库结果。MySQL 使用原完整入口；148 个 Go/SQL/module/Makefile/Python 输入在合并后冻结，最终 SHA 复核一致。最终产品来自 V5；V6/V7 仅修 E2E 行标题定位、实际按键持续时间与记录，未修改产品。测试与截图版本对应关系见[机器可读摘要](2026-09-09-release-detail-results.json)。

## 独立审查与视觉取证

固定依赖合并与完整功能分别经过 Standards / Spec 两轴独立审查。V7 功能/测试增量无硬性规范违反、无 Spec 阻断；已修复未知重新准备的跨动作互斥、MODIFY 未提交字段默认过滤及验证状态陈述不一致。三项非阻断 P3 已接受保留：请求 scope 字符串拆分、生命周期判断重复、截图取证序列重复；当前约定与行为测试覆盖，未扩大可选重构范围。

独立 impeccable finish reviewer 对以下四张稳定 Chromium 图给出 ship，五个契约部分完整，无视觉修正；正式 fullall v3 新图已再次核对版本，见[完整视觉审查](2026-09-09-release-detail-visual-review.md)；独立 documenter 对现有唯一 web/DESIGN 给出 no-op。该图面审查只证明所见视图，跨引擎键盘与数据库结果由正式门禁独立证明。

截图来自最终正式全量三引擎v3产物，使用禁用动画、复制反馈完成并移除后、全页回到页顶的稳定状态。参考图只约束信息组织；真实界面保留英文字段名、真实单号与仓库灰/石墨主题。

- [已批准详情桌面](2026-09-09-release-detail-approved-desktop.png)
- [已批准详情 390px](2026-09-09-release-detail-approved-390.png)
- [原单已回滚的实际结果 390px](2026-09-09-release-detail-original-result-390.png)
- [快速恢复实际结果 390px](2026-09-09-release-detail-restoration-390.png)

## 依赖整合

固定功能基点 `e3b68383bea425f3258f726d34a967ef6b1aa61c` 合并 #63 `dddf1c3c78f970a656642062ede28c7f41971a8f` 与 #62 `49a08fe31c4771c4727a44f527d11c8f8bd22e49`。7 文件共12处真实冲突保留双方职责：领域文档/契约保留重新准备、快速回滚与人工完结；服务保留各动作状态与反向限制；页面/API/journal 保留两套已交付动作及恢复路径。编译发现 #62 改名 `appendRelatedReleaseEvent` 的非文本冲突，同步 #63 quick rollback 调用，无新后端语义。真实 remerge diff 与逐块决策由协调者冻结归档。

合并定向真实 MySQL 17 个 tests/subtests 通过（122.681s）；合并时 make test/build、Web typecheck 和311项原范围测试通过。最初 Web 沙箱真实 TCP 监听 EPERM 后，在已授权本地网络环境运行原测试通过，无断言修改。

## 红绿与失败保留

- 组件红绿依次覆盖过滤语义、结果切换、八状态/步骤人员时间与中文历史、未知重新准备跨动作、最新基线重建及401会话恢复。401用例发现首个认证错误清除原请求，改为保守保留；另一账号不显示或重放原申请。
- 初始 V1 Web329通过，V2键盘修正后330通过。后续Spec复查发现 omitted 被错误固化为默认可见，纠正测试与实现：原1失败/6通过（1.71s）→7通过（1.25s）；V5全Web330通过（21.60s）。
- 首次全三引擎入口在复制提示处失败（358.593s，exit2）。最小真实浏览器循环重复3次证明下载版Chromium权限prompt拒绝，而本机Chrome默认granted成功；仅给申请人当前origin的clipboard权限后，两浏览器均连续3次通过。正式用例保留真实toast并在浏览器内部断言readText等于真实单号，仅返回布尔，不输出剪贴板原文。
- V3定向 release-workflow / Chromium 通过（97.499s）。自检发现复制toast和未归顶影响截图，后续只修改取证等待/滚动/动画。未使用这些旧图作为最终视觉通过证据。
- V5 GoBrowser新增字段断言失败（142.965s，exit2）：`id`与类型共同构成行标题，getByText精确定位不匹配。最小Chrome回路证明旧定位count0、实际rowheader `id uint64` count1；全部存在/不存在断言改为语义行标题，保留6项未提交、2项生成及过滤往返，避免默认count0假阳性。产品未变，最终正式入口结果见上表。

- V6全三引擎在WebKit横滚处失败（475.161s，exit2）：Tab可达，但零间隔按键被原生动画合并。最终仅将真实方向键按住100ms，保留12键、原15秒最终位置检查并增加整列左右边界。三轮三引擎最小回路全绿；失败的逐键反馈实验不进入产品或正式用例。最终正式结果见上表。

没有新增后端兼容层、假结果、双写或 Feature Flag。
