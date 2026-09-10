# #96 Spec 审查

固定点：`836c95da4100d06a102dc6cd0266035cafe18ceb`。提交列表及三点 diff 为空；依 `review-input.md` 审查完整未提交工作树 `git diff <base> --`。50 项源码指纹与 `review-source-manifest.json` 全部匹配。依据完整 #96/#92 正文、评论、标签和 ADR-0026，独立检查实际 diff 及调用方；本轴未运行测试或修改源码。

**发现 1 项（P2，验证缺口）；未发现 AC-013～018 产品实现缺陷。**

- **[P2] 受影响的重新准备竞争回归尚无有效通过证据。** #96「本张完成其验收用例、受影响回归及必要权限/事务/迁移检查」。`affected-regression.txt:409` 中 `TestMultitableReprepareTransfersChangedTargetsAtomically` 失败；源码 `admin/cmd/admin/release_copy_reprepare_multitable_integration_test.go:271` 强制等待竞争者到达 `rcc_release_targets`，但 `admin/internal/infrastructure/mysql/release_orders.go:34` 的既有授权锁会先串行化两次写入。该轮最终为 17 PASS / 1 FAIL（日志 :740～743），不能据此宣称受影响回归全绿。需确认实际阻塞位置、调整夹具以仍证明事务开放期间竞争者无法抢占且提交后返回替代单归属，再完成原完整断言。此项不要求提前交付 #97 的重新准备结果通知，也不据旧夹具失败推断产品回归。

独立核对：AC-013～015 的所有正常资格入口均在原授权事务重算，收件人与审批共用 `approvalContext`；冻结空快照、实时成员及 ADMIN 退出规则一致，仍有可审表时保留整单待办。本人动作排除新提醒，纯待办与旧结果未读分别保留。AC-016 的 header、资格、个人序号共用 RR 快照，主单 document／原请求结果不持久化 receipt；Web 只确认实际展示序号，失败可手动重试，卸载后同账号缓存失效仍执行。AC-017 的隐藏工作区停止查询，API generation 与账号响应头隔离迟到结果。AC-018 的通知写入位于业务保存后、原结果保存前，GET 无补写路径。

九项核心 MySQL、迁移/reset 最终对应 PASS 段、Web 可控计时及最终 5＋7 浏览器路径提供 AC-013～018 证据；保留的失败原轮未计作全绿。未发现额外范围蔓延、临时 flag 或应在本票退出的旧接口；授权的 reset 扩展为必要调用方适配，#97/#98 边界保留。

## 修复后复核：P2 已关闭，剩余 0 项

保留上述初审及原失败日志。新增第 51 项源码变更仅调整竞争夹具；原 50 项 manifest 指纹仍全部匹配，生产锁顺序未改。`release_copy_reprepare_multitable_integration_test.go:273` 通过真实锁等待表和线程连接关联，将阻塞者精确绑定到已进入目标交接 gate 的重新准备连接；不是取消并发断言。`:303` 起的 201 替代草稿、409 指向新单、原键原正文重推、来源/占用转移及重新提交后独立审批断言全部保留。

`reprepare-lock-regression.txt:46` 实际记录竞争连接 13 等待重新准备连接 9 的授权锁；`:47～57` 为释放 gate 后的真实业务响应，`:62～64` 确认该完整用例 PASS。受影响 18 项最终证据为原轮 17 个有效 PASS 加本轮 1 个 PASS；原轮本身仍为失败，未重新标为全绿。测试文件 SHA-256：`96a3445f0adb5fb113cbd1253020c5d8492e66c4efbe20fa689ca4b3e76c93bf`。本次只读复核未运行测试，未发现新 Spec 问题。
