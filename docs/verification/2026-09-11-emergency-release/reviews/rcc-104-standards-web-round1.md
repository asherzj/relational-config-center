# #104 Standards Web 增量初审

基点 `e2564842e72e96b6ffb1e86708978b11dd8a651b`；仅 Web/API/journal/控件与五份领域文档增量。未读 Spec、未执行测试；后端 P3 已关闭且未重审。以下保留修复前快照，修复中改动待下轮验证。

**成文规范：3 项 P2。** 路径均相对本票工作树。

1. **P2 — 提交冲突无法按最新发布方式恢复。** `web/src/features/release-orders/ReleaseRequestReview.tsx:52–55` 原实现重建只替换版本，沿用原应急原因。另一个窗口把应急改为常规后，原请求确定冲突，用户审阅最新状态再确认仍提交不允许的原因；反向切换则缺必填原因，恢复界面无输入入口。违反 `web/DESIGN.md:98`「审阅最新状态和差异，再明确确认重建新请求」。重建时展示最新方式，常规去除原因，应急允许确认、补填并校验；未知结果期间保留原键/正文。
2. **P2 — 应急复制、重新准备误述审批。** `ReleaseActionDialog.tsx:57,75,78`（同上 features/release-orders 目录）承诺新草稿接受独立审批，危险确认把待发布应急源单称为「已批准」。违反 `web/DESIGN.md:96` 真实说明要求及 `docs/adr/0027-configure-release-workflows-with-stable-templates.md:23` 应急无批准事实、重新提交原因的约定。按源单状态和方式准确说明取消后果及新草稿下一步。
3. **P2 — 新操作成功缺少反馈。** `ReleaseActionDialog.tsx:35`、`NewDraftDialog.tsx:23` 仅关闭/导航；共同 `useReleaseWrite.ts` 也无成功提示。违反 `web/DESIGN.md:97` Sonner 成功反馈规则。确认应急提交、创建成功后显示可关闭的成功提示，未知结果不报成功。

**主观：0 项。** 已应用完整基线：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest；工具已强制项跳过。

**证据与待项：** 原始日志可对应 38 个顶层 HTTP PASS；Web19 原始 414 PASS/12 FAIL 保留，23 的 11 PASS 与24 的1 PASS支持426有效合并口径，非单次全绿。修复将使相交证据需要更新。真实浏览器、最终 DESIGN 视觉同步及最终索引尚待，不据预期放行。

**输入：** `/tmp/rcc-104-standards-web-source-round1.json`（SHA256 `c188399cca1965979b43bde9198f72e398d9d677a1c856f6f64cc2267264c2a9`），包含修复前实际读取哈希、收口时变化及证据哈希。原清单 SHA256 `820b81f0762042322be6d4aa1d1795081050d3f09787ceb2e9f41f8ab814af65`；关键修复前哈希：RequestReview `411f93b6…`、ActionDialog `6eefcb23…`、NewDraftDialog `ed27e8b6…`。当前已有修复写入，不在本轮宣称关闭。
